package bot

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/tionis/gyroskop/internal/database"
)

type Bot struct {
	api             *tgbotapi.BotAPI
	db              *database.DB
	activeGyroskops map[int64]*database.Gyroskop // Cache für aktive Gyroskops
	stopChan        chan bool                    // Channel to stop the background goroutine
}

// New erstellt eine neue Bot-Instanz
func New(token string, db *database.DB) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	return &Bot{
		api:             api,
		db:              db,
		activeGyroskops: make(map[int64]*database.Gyroskop),
		stopChan:        make(chan bool),
	}, nil
}

// Run starts the bot
func (b *Bot) Run() {
	// Load existing open gyroskops on startup
	b.loadActiveGyroskops()

	// Start background goroutine to check for expired gyroskops
	go b.backgroundExpiryChecker()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		select {
		case <-b.stopChan:
			log.Println("Bot stopping...")
			return
		default:
			if update.Message != nil {
				b.handleMessage(update.Message)
			}
			if update.CallbackQuery != nil {
				b.handleCallbackQuery(update.CallbackQuery)
			}
		}
	}
}

// handleMessage verarbeitet eingehende Nachrichten
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	// Nur Gruppennachrichten verarbeiten
	if !message.Chat.IsGroup() && !message.Chat.IsSuperGroup() {
		b.sendMessage(message.Chat.ID, "🥙 Gyroskop funktioniert nur in Gruppen!")
		return
	}

	if message.IsCommand() {
		b.handleCommand(message)
	} else {
		b.handleTextMessage(message)
	}
}

// handleCommand verarbeitet Bot-Befehle
func (b *Bot) handleCommand(message *tgbotapi.Message) {
	command := message.Command()
	args := message.CommandArguments()

	switch command {
	case "start", "help":
		b.handleHelp(message)
	case "gyroskop":
		b.handleNewGyroskop(message, args)
	case "status":
		b.handleStatus(message)
	case "ende":
		b.handleEndGyroskop(message)
	case "stornieren", "cancel":
		b.handleCancelOrder(message)
	}
}

// handleHelp sends the help message
func (b *Bot) handleHelp(message *tgbotapi.Message) {
	helpText := `🥙 *Gyroskop Bot - Gyros Bestellungen koordinieren*

*Befehle:*
/gyroskop - Neues Gyroskop für 15 Minuten öffnen
/gyroskop HH:MM - Neues Gyroskop bis zur angegebenen Uhrzeit öffnen
/gyroskop 30min - Neues Gyroskop für 30 Minuten öffnen
/gyroskop (als Antwort) - Gyroskop wiedereröffnen für 15 Minuten
/gyroskop HH:MM (als Antwort) - Gyroskop mit neuer Deadline wiedereröffnen
/status - Aktuellen Status anzeigen
/ende - Gyroskop beenden (nur als Antwort auf Gyroskop-Nachricht)
/stornieren - Eigene Bestellung stornieren
/help - Diese Hilfe anzeigen

*Bestellen:*
🥩 *Mit Fleisch:* Nutze die Buttons 1️⃣-5️⃣ unter "Mit Fleisch"
🥬 *Vegetarisch:* Nutze die Buttons 1️⃣-5️⃣ unter "Vegetarisch"
❌ *Stornieren:* Schreibe "0" oder nutze den ❌ Stornieren Button

*Beispiele:*
/gyroskop - Öffnet Gyroskop für 15 Minuten
/gyroskop 17:00 - Öffnet Gyroskop bis 17:00 Uhr
/gyroskop 45min - Öffnet Gyroskop für 45 Minuten
🥩 2️⃣ Button - Bestellt 2 Gyros mit Fleisch
🥬 1️⃣ Button - Bestellt 1 vegetarisches Gyros
0 - Storniert die komplette Bestellung`

	b.sendMessage(message.Chat.ID, helpText)
}

// handleNewGyroskop creates a new gyroskop or reopens an existing one
func (b *Bot) handleNewGyroskop(message *tgbotapi.Message, args string) {
	// Check if this is a reply to a gyroskop message (reopen functionality)
	if message.ReplyToMessage != nil {
		b.handleReopenGyroskop(message, args)
		return
	}

	// Check if there's already an active gyroskop
	if existingGyroskop, exists := b.activeGyroskops[message.Chat.ID]; exists {
		berlin, _ := time.LoadLocation("Europe/Berlin")
		deadlineInBerlin := existingGyroskop.Deadline.In(berlin)
		b.sendMessage(message.Chat.ID, fmt.Sprintf("⚠️ Es gibt bereits ein aktives Gyroskop bis %s. Nutze /ende als Antwort auf die Gyroskop-Nachricht um es zu beenden.", deadlineInBerlin.Format("15:04")))
		return
	}

	// Parse deadline from args or use default (15 minutes)
	deadline, err := b.parseDeadline(args)
	if err != nil {
		b.sendMessage(message.Chat.ID, "⚠️ Ungültiges Zeitformat. Verwende HH:MM (z.B. 17:00) oder Dauer (z.B. 30min)")
		return
	}

	// Create new gyroskop
	gyroskop, err := b.db.CreateGyroskop(message.Chat.ID, int64(message.From.ID), deadline)
	if err != nil {
		log.Printf("Fehler beim Erstellen des Gyroskops: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Erstellen des Gyroskops")
		return
	}

	b.sendGyroskopMessage(message.Chat.ID, gyroskop, "🥙 *Gyroskop geöffnet!*", message.From)
}

// handleReopenGyroskop reopens a closed gyroskop
func (b *Bot) handleReopenGyroskop(message *tgbotapi.Message, args string) {
	// Check if user is the creator by checking the replied message
	replyMessage := message.ReplyToMessage

	// Try to find the gyroskop by message ID
	gyroskop, err := b.db.GetGyroskopByMessageID(message.Chat.ID, replyMessage.MessageID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ Das ist keine gültige Gyroskop-Nachricht")
		return
	}

	// Check if user is the creator
	if gyroskop.CreatedBy != int64(message.From.ID) {
		b.sendMessage(message.Chat.ID, "⚠️ Nur der Ersteller kann das Gyroskop wiedereröffnen!")
		return
	}

	// Check if there's already an active gyroskop
	if existingGyroskop, exists := b.activeGyroskops[message.Chat.ID]; exists {
		berlin, _ := time.LoadLocation("Europe/Berlin")
		deadlineInBerlin := existingGyroskop.Deadline.In(berlin)
		b.sendMessage(message.Chat.ID, fmt.Sprintf("⚠️ Es gibt bereits ein aktives Gyroskop bis %s. Beende es zuerst.", deadlineInBerlin.Format("15:04")))
		return
	}

	// Parse new deadline or use default (15 minutes)
	deadline, err := b.parseDeadline(args)
	if err != nil {
		b.sendMessage(message.Chat.ID, "⚠️ Ungültiges Zeitformat. Verwende HH:MM (z.B. 17:00) oder Dauer (z.B. 30min)")
		return
	}

	// Reopen the gyroskop
	err = b.db.ReopenGyroskop(gyroskop.ID, deadline)
	if err != nil {
		log.Printf("Fehler beim Wiedereröffnen des Gyroskops: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Wiedereröffnen des Gyroskops")
		return
	}

	// Update gyroskop data
	gyroskop.Deadline = deadline
	gyroskop.IsOpen = true

	b.sendGyroskopMessage(message.Chat.ID, gyroskop, "🔄 *Gyroskop wiedereröffnet!*", message.From)
}

// sendGyroskopMessage sends the gyroskop message with proper formatting
func (b *Bot) sendGyroskopMessage(chatID int64, gyroskop *database.Gyroskop, title string, user *tgbotapi.User) {
	userName := b.getUserName(user)

	// Convert deadline to Berlin timezone for display
	berlin, _ := time.LoadLocation("Europe/Berlin")
	deadlineInBerlin := gyroskop.Deadline.In(berlin)

	text := fmt.Sprintf("%s\n\n"+
		"👤 Erstellt von: %s\n"+
		"⏰ Deadline: %s Uhr\n\n"+
		"Schreibt eine Zahl um Gyros zu bestellen!\n"+
		"Oder nutzt die Reaction-Buttons: 1️⃣ 2️⃣ 3️⃣ 4️⃣ 5️⃣\n\n"+
		"Zum Beenden: Antwortet auf diese Nachricht mit /ende",
		title,
		userName,
		deadlineInBerlin.Format("15:04"),
	)

	// Add gyroskop to cache
	b.activeGyroskops[chatID] = gyroskop

	// Send message with reaction buttons and save message ID
	sentMessage := b.sendMessageWithReactions(chatID, text)
	if sentMessage != nil {
		// Update message ID in database
		err := b.db.UpdateGyroskopMessageID(gyroskop.ID, sentMessage.MessageID)
		if err != nil {
			log.Printf("Error saving message ID: %v", err)
		}
		gyroskop.MessageID = sentMessage.MessageID
	}
}

// handleStatus shows the current status
func (b *Bot) handleStatus(message *tgbotapi.Message) {
	gyroskop, exists := b.activeGyroskops[message.Chat.ID]
	if !exists {
		b.sendMessage(message.Chat.ID, "❌ Kein aktives Gyroskop in dieser Gruppe")
		return
	}

	orders, err := b.db.GetOrdersByGyroskop(gyroskop.ID)
	if err != nil {
		log.Printf("Fehler beim Laden der Bestellungen: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Laden der Bestellungen")
		return
	}

	text := b.formatCurrentStatus(gyroskop, orders)
	b.sendMessage(message.Chat.ID, text)
}

// handleCloseGyroskop schließt das aktive Gyroskop
func (b *Bot) handleCloseGyroskop(message *tgbotapi.Message) {
	gyroskop, err := b.db.GetActiveGyroskop(message.Chat.ID)
	if err != nil {
		b.sendMessage(message.Chat.ID, "❌ Kein aktives Gyroskop in dieser Gruppe")
		return
	}

	// Check if the user is the creator
	if gyroskop.CreatedBy != int64(message.From.ID) {
		b.sendMessage(message.Chat.ID, "⚠️ Nur der Ersteller kann das Gyroskop schließen!")
		return
	}

	orders, err := b.db.GetOrdersByGyroskop(gyroskop.ID)
	if err != nil {
		log.Printf("Fehler beim Laden der Bestellungen: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Laden der Bestellungen")
		return
	}

	err = b.db.CloseGyroskop(gyroskop.ID)
	if err != nil {
		log.Printf("Fehler beim Schließen des Gyroskops: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Schließen des Gyroskops")
		return
	}

	text := "🔒 *Gyroskop geschlossen!*\n\n" + b.formatOrderSummary(gyroskop, orders)
	b.sendMessage(message.Chat.ID, text)
}

// handleCancelOrder storniert eine Bestellung
func (b *Bot) handleCancelOrder(message *tgbotapi.Message) {
	gyroskop, exists := b.activeGyroskops[message.Chat.ID]
	if !exists {
		b.sendMessage(message.Chat.ID, "❌ Kein aktives Gyroskop in dieser Gruppe")
		return
	}

	err := b.db.RemoveOrder(gyroskop.ID, int64(message.From.ID))
	if err != nil {
		log.Printf("Error canceling order: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Error canceling order")
		return
	}

	userName := b.getUserName(message.From)
	b.sendMessage(message.Chat.ID, fmt.Sprintf("✅ Bestellung von %s wurde storniert", userName))
}

// handleTextMessage processes text messages (Bestellungen)
func (b *Bot) handleTextMessage(message *tgbotapi.Message) {
	// Für das neue System mit zwei Gyros-Arten nutzen wir hauptsächlich die Buttons
	// Texteingabe ist nur noch für einfache Stornierung (0) gedacht

	quantity, err := strconv.Atoi(strings.TrimSpace(message.Text))
	if err != nil {
		return // Ignorieren wenn es keine Zahl ist
	}

	// Nur 0 für Stornierung akzeptieren
	if quantity != 0 {
		_, exists := b.activeGyroskops[message.Chat.ID]
		if exists {
			b.sendMessage(message.Chat.ID, "💡 Nutze die Buttons um zwischen 🥩 Fleisch und 🥬 vegetarischen Gyros zu wählen!")
		}
		return
	}

	gyroskop, exists := b.activeGyroskops[message.Chat.ID]
	if !exists {
		return // Ignorieren wenn kein aktives Gyroskop
	}

	// Check if the gyroskop is still open
	if time.Now().After(gyroskop.Deadline) {
		b.sendMessage(message.Chat.ID, "⏰ Das Gyroskop ist bereits abgelaufen!")
		return
	}

	userName := b.getUserName(message.From)

	// Bestellung stornieren (0 eingegeben)
	err = b.db.RemoveOrder(gyroskop.ID, int64(message.From.ID))
	if err != nil {
		log.Printf("Error canceling order: %v", err)
		return
	}
	b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ %s hat die Bestellung storniert", userName))
}

// parseDeadline parses a deadline from various formats or returns default (15 minutes from now)
func (b *Bot) parseDeadline(input string) (time.Time, error) {
	input = strings.TrimSpace(input)

	// If no input, default to 15 minutes from now
	if input == "" {
		return time.Now().Add(15 * time.Minute), nil
	}

	// Load Berlin timezone
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.Time{}, err
	}

	now := time.Now().In(berlin)

	// Check for duration format (e.g., "15min", "30min", "1h")
	durationRegex := regexp.MustCompile(`^(\d+)(min|m|h|hour|hours)$`)
	if matches := durationRegex.FindStringSubmatch(input); len(matches) == 3 {
		value, err := strconv.Atoi(matches[1])
		if err != nil {
			return time.Time{}, err
		}

		var duration time.Duration
		unit := strings.ToLower(matches[2])
		switch unit {
		case "min", "m":
			duration = time.Duration(value) * time.Minute
		case "h", "hour", "hours":
			duration = time.Duration(value) * time.Hour
		}

		if duration == 0 {
			return time.Time{}, fmt.Errorf("invalid duration")
		}

		return now.Add(duration).UTC(), nil
	}

	// Check for time format (HH:MM)
	timeRegex := regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	if timeRegex.MatchString(input) {
		// Parse as time today in Berlin timezone
		parsedTime, err := time.ParseInLocation("15:04", input, berlin)
		if err != nil {
			return time.Time{}, err
		}

		// Combine today's date with the parsed time
		deadline := time.Date(
			now.Year(), now.Month(), now.Day(),
			parsedTime.Hour(), parsedTime.Minute(), 0, 0,
			berlin,
		)

		// If the time is in the past, use tomorrow
		if deadline.Before(now) {
			deadline = deadline.AddDate(0, 0, 1)
		}

		return deadline.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid format")
}

// handleEndGyroskop ends the gyroskop (reply only)
func (b *Bot) handleEndGyroskop(message *tgbotapi.Message) {
	// Check if it is a reply to a message
	if message.ReplyToMessage == nil {
		b.sendMessage(message.Chat.ID, "⚠️ /ende muss als Antwort auf die Gyroskop-Nachricht verwendet werden!")
		return
	}

	gyroskop, exists := b.activeGyroskops[message.Chat.ID]
	if !exists {
		b.sendMessage(message.Chat.ID, "❌ Kein aktives Gyroskop in dieser Gruppe")
		return
	}

	// Check if the user is the creator
	if gyroskop.CreatedBy != int64(message.From.ID) {
		b.sendMessage(message.Chat.ID, "⚠️ Nur der Ersteller kann das Gyroskop beenden!")
		return
	}

	b.closeGyroskop(gyroskop)
}

// autoCloseGyroskop automatically closes an expired gyroskop
func (b *Bot) autoCloseGyroskop(gyroskop *database.Gyroskop) {
	log.Printf("Auto-closing gyroskop %d for chat %d", gyroskop.ID, gyroskop.ChatID)
	b.closeGyroskop(gyroskop)
}

// closeGyroskop schließt ein Gyroskop und sendet Übersicht
func (b *Bot) closeGyroskop(gyroskop *database.Gyroskop) {
	orders, err := b.db.GetOrdersByGyroskop(gyroskop.ID)
	if err != nil {
		log.Printf("Fehler beim Laden der Bestellungen: %v", err)
		b.sendMessage(gyroskop.ChatID, "❌ Fehler beim Laden der Bestellungen")
		return
	}

	err = b.db.CloseGyroskop(gyroskop.ID)
	if err != nil {
		log.Printf("Fehler beim Schließen des Gyroskops: %v", err)
		b.sendMessage(gyroskop.ChatID, "❌ Fehler beim Schließen des Gyroskops")
		return
	}

	// Aus Cache entfernen
	delete(b.activeGyroskops, gyroskop.ChatID)

	text := "🔒 *Gyroskop beendet!*\n\n" + b.formatOrderSummary(gyroskop, orders)
	b.sendMessage(gyroskop.ChatID, text)
}

// handleCallbackQuery verarbeitet Reactions/Inline-Button Klicks
func (b *Bot) handleCallbackQuery(query *tgbotapi.CallbackQuery) {
	// Parse Callback Data
	data := query.Data
	if len(data) < 2 || data[0] != 'g' {
		return
	}

	// Format: gXY wobei X=Art (m=meat, v=veggie) und Y=Anzahl
	// oder g0 für stornieren
	if data == "g0" {
		b.handleCancelOrderCallback(query)
		return
	}

	if len(data) < 3 {
		return
	}

	gyrosType := data[1] // 'm' für Fleisch, 'v' für vegetarisch
	quantity, err := strconv.Atoi(data[2:])
	if err != nil {
		return
	}

	gyroskop, exists := b.activeGyroskops[query.Message.Chat.ID]
	if !exists {
		b.answerCallbackQuery(query.ID, "❌ Kein aktives Gyroskop")
		return
	}

	// Check if the gyroskop is still open
	if time.Now().After(gyroskop.Deadline) {
		b.answerCallbackQuery(query.ID, "⏰ Das Gyroskop ist bereits abgelaufen!")
		return
	}

	// Aktuelle Bestellung des Users laden
	currentOrders, err := b.db.GetOrdersByGyroskop(gyroskop.ID)
	if err != nil {
		log.Printf("Fehler beim Laden der aktuellen Bestellungen: %v", err)
		b.answerCallbackQuery(query.ID, "❌ Fehler beim Laden der Bestellungen")
		return
	}

	var currentMeat, currentVeggie int
	for _, order := range currentOrders {
		if order.UserID == int64(query.From.ID) {
			currentMeat = order.QuantityMeat
			currentVeggie = order.QuantityVeggie
			break
		}
	}

	// Neue Werte basierend auf Gyros-Typ setzen
	var newMeat, newVeggie int
	var responseText string

	if gyrosType == 'm' {
		// Fleisch-Gyros
		newMeat = quantity
		newVeggie = currentVeggie
		if quantity == 1 {
			responseText = "✅ 1 Gyros mit Fleisch"
		} else {
			responseText = fmt.Sprintf("✅ %d Gyros mit Fleisch", quantity)
		}
	} else if gyrosType == 'v' {
		// Vegetarische Gyros
		newMeat = currentMeat
		newVeggie = quantity
		if quantity == 1 {
			responseText = "✅ 1 vegetarisches Gyros"
		} else {
			responseText = fmt.Sprintf("✅ %d vegetarische Gyros", quantity)
		}
	}

	// Bestellung hinzufügen/aktualisieren
	err = b.db.AddOrUpdateOrder(
		gyroskop.ID,
		int64(query.From.ID),
		query.From.UserName,
		query.From.FirstName,
		query.From.LastName,
		newMeat,
		newVeggie,
	)
	if err != nil {
		log.Printf("Error adding order: %v", err)
		b.answerCallbackQuery(query.ID, "❌ Fehler beim Bestellen")
		return
	}

	b.answerCallbackQuery(query.ID, responseText)

	// Nach jeder Änderung die Gyroskop-Nachricht mit aktuellem Status aktualisieren
	b.updateGyroskopMessage(gyroskop, query.Message)
}

// handleCancelOrderCallback behandelt das Stornieren einer Bestellung über Callback
func (b *Bot) handleCancelOrderCallback(query *tgbotapi.CallbackQuery) {
	gyroskop, exists := b.activeGyroskops[query.Message.Chat.ID]
	if !exists {
		b.answerCallbackQuery(query.ID, "❌ Kein aktives Gyroskop")
		return
	}

	// Check if the gyroskop is still open
	if time.Now().After(gyroskop.Deadline) {
		b.answerCallbackQuery(query.ID, "⏰ Das Gyroskop ist bereits abgelaufen!")
		return
	}

	// Bestellung stornieren
	err := b.db.RemoveOrder(gyroskop.ID, int64(query.From.ID))
	if err != nil {
		log.Printf("Error canceling order: %v", err)
		b.answerCallbackQuery(query.ID, "❌ Fehler beim Stornieren")
		return
	}

	b.answerCallbackQuery(query.ID, "❌ Bestellung storniert")

	// Nach jeder Änderung die Gyroskop-Nachricht mit aktuellem Status aktualisieren
	b.updateGyroskopMessage(gyroskop, query.Message)
}

// formatCurrentStatus formatiert den aktuellen Status (während Gyroskop läuft)
func (b *Bot) formatCurrentStatus(gyroskop *database.Gyroskop, orders []database.Order) string {
	var text strings.Builder

	// Convert deadline to Berlin timezone for display
	berlin, _ := time.LoadLocation("Europe/Berlin")
	deadlineInBerlin := gyroskop.Deadline.In(berlin)

	text.WriteString("📊 *Aktueller Status*\n")
	text.WriteString(fmt.Sprintf("⏰ Deadline: %s Uhr\n\n", deadlineInBerlin.Format("15:04")))

	if len(orders) == 0 {
		text.WriteString("Noch keine Bestellungen 😢")
		return text.String()
	}

	totalMeat := 0
	totalVeggie := 0

	for _, order := range orders {
		name := b.formatUserName(&order)
		var orderText string

		if order.QuantityMeat > 0 && order.QuantityVeggie > 0 {
			orderText = fmt.Sprintf("🥩 %d mit Fleisch, 🥬 %d vegetarisch", order.QuantityMeat, order.QuantityVeggie)
		} else if order.QuantityMeat > 0 {
			if order.QuantityMeat == 1 {
				orderText = "🥩 1 mit Fleisch"
			} else {
				orderText = fmt.Sprintf("🥩 %d mit Fleisch", order.QuantityMeat)
			}
		} else if order.QuantityVeggie > 0 {
			if order.QuantityVeggie == 1 {
				orderText = "🥬 1 vegetarisch"
			} else {
				orderText = fmt.Sprintf("🥬 %d vegetarisch", order.QuantityVeggie)
			}
		}

		text.WriteString(fmt.Sprintf("• %s: %s\n", name, orderText))
		totalMeat += order.QuantityMeat
		totalVeggie += order.QuantityVeggie
	}

	totalGyros := totalMeat + totalVeggie
	text.WriteString(fmt.Sprintf("\n🥙 *Gesamt: %d Gyros* (🥩 %d mit Fleisch, 🥬 %d vegetarisch)", totalGyros, totalMeat, totalVeggie))
	return text.String()
}

// formatOrderSummary formatiert die finale Bestellübersicht
func (b *Bot) formatOrderSummary(gyroskop *database.Gyroskop, orders []database.Order) string {
	var text strings.Builder

	// Convert deadline to Berlin timezone for display
	berlin, _ := time.LoadLocation("Europe/Berlin")
	deadlineInBerlin := gyroskop.Deadline.In(berlin)

	text.WriteString("📊 *Finale Bestellübersicht*\n")
	text.WriteString(fmt.Sprintf("⏰ Deadline war: %s Uhr\n\n", deadlineInBerlin.Format("15:04")))

	if len(orders) == 0 {
		text.WriteString("Keine Bestellungen eingegangen 😢")
		return text.String()
	}

	totalMeat := 0
	totalVeggie := 0

	for _, order := range orders {
		name := b.formatUserName(&order)
		var orderText string

		if order.QuantityMeat > 0 && order.QuantityVeggie > 0 {
			orderText = fmt.Sprintf("🥩 %d mit Fleisch, 🥬 %d vegetarisch", order.QuantityMeat, order.QuantityVeggie)
		} else if order.QuantityMeat > 0 {
			if order.QuantityMeat == 1 {
				orderText = "🥩 1 mit Fleisch"
			} else {
				orderText = fmt.Sprintf("🥩 %d mit Fleisch", order.QuantityMeat)
			}
		} else if order.QuantityVeggie > 0 {
			if order.QuantityVeggie == 1 {
				orderText = "🥬 1 vegetarisch"
			} else {
				orderText = fmt.Sprintf("🥬 %d vegetarisch", order.QuantityVeggie)
			}
		}

		text.WriteString(fmt.Sprintf("• %s: %s\n", name, orderText))
		totalMeat += order.QuantityMeat
		totalVeggie += order.QuantityVeggie
	}

	totalGyros := totalMeat + totalVeggie
	text.WriteString(fmt.Sprintf("\n🥙 *Gesamt: %d Gyros* (🥩 %d mit Fleisch, 🥬 %d vegetarisch)", totalGyros, totalMeat, totalVeggie))
	return text.String()
}

// getUserName holt den Anzeigenamen eines Users
func (b *Bot) getUserName(user *tgbotapi.User) string {
	if user.FirstName != "" {
		if user.LastName != "" {
			return fmt.Sprintf("%s %s", user.FirstName, user.LastName)
		}
		return user.FirstName
	}
	if user.UserName != "" {
		return "@" + user.UserName
	}
	return fmt.Sprintf("User %d", user.ID)
}

// formatUserName formatiert den Namen aus einer Bestellung
func (b *Bot) formatUserName(order *database.Order) string {
	if order.FirstName != "" {
		if order.LastName != "" {
			return fmt.Sprintf("%s %s", order.FirstName, order.LastName)
		}
		return order.FirstName
	}
	if order.Username != "" {
		return "@" + order.Username
	}
	return fmt.Sprintf("User %d", order.UserID)
}

// sendMessage sendet eine Nachricht
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Fehler beim Senden der Nachricht: %v", err)
	}
}

// sendMessageWithReactions sendet eine Nachricht mit Reaction-Buttons
func (b *Bot) sendMessageWithReactions(chatID int64, text string) *tgbotapi.Message {
	// Inline Keyboard mit Reaction-Buttons für beide Gyros-Arten erstellen
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🥩 Mit Fleisch:", "noop"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1️⃣", "gm1"),
			tgbotapi.NewInlineKeyboardButtonData("2️⃣", "gm2"),
			tgbotapi.NewInlineKeyboardButtonData("3️⃣", "gm3"),
			tgbotapi.NewInlineKeyboardButtonData("4️⃣", "gm4"),
			tgbotapi.NewInlineKeyboardButtonData("5️⃣", "gm5"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🥬 Vegetarisch:", "noop"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1️⃣", "gv1"),
			tgbotapi.NewInlineKeyboardButtonData("2️⃣", "gv2"),
			tgbotapi.NewInlineKeyboardButtonData("3️⃣", "gv3"),
			tgbotapi.NewInlineKeyboardButtonData("4️⃣", "gv4"),
			tgbotapi.NewInlineKeyboardButtonData("5️⃣", "gv5"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Stornieren", "g0"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyMarkup = keyboard

	sentMessage, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Fehler beim Senden der Nachricht mit Reactions: %v", err)
		return nil
	}

	return &sentMessage
}

// answerCallbackQuery antwortet auf eine Callback Query
func (b *Bot) answerCallbackQuery(callbackQueryID, text string) {
	callback := tgbotapi.NewCallback(callbackQueryID, text)
	_, err := b.api.Request(callback)
	if err != nil {
		log.Printf("Fehler beim Antworten auf Callback Query: %v", err)
	}
}

// updateGyroskopMessage aktualisiert die Gyroskop-Nachricht mit aktuellen Bestellungen
func (b *Bot) updateGyroskopMessage(gyroskop *database.Gyroskop, originalMessage *tgbotapi.Message) {
	// MessageID verwenden - falls nicht gesetzt, die von der Callback-Query nehmen
	messageID := gyroskop.MessageID
	if messageID == 0 {
		messageID = originalMessage.MessageID
	}

	// Aktuelle Bestellungen laden
	orders, err := b.db.GetOrdersByGyroskop(gyroskop.ID)
	if err != nil {
		log.Printf("Fehler beim Laden der Bestellungen für Update: %v", err)
		return
	}

	// Ersteller-Name laden
	var creatorName string
	for _, order := range orders {
		if order.UserID == gyroskop.CreatedBy {
			creatorName = b.formatUserName(&order)
			break
		}
	}
	if creatorName == "" {
		creatorName = fmt.Sprintf("User %d", gyroskop.CreatedBy)
	}

	// Convert deadline to Berlin timezone for display
	berlin, _ := time.LoadLocation("Europe/Berlin")
	deadlineInBerlin := gyroskop.Deadline.In(berlin)

	// Neue Nachricht zusammenstellen
	text := fmt.Sprintf("🥙 *Gyroskop geöffnet!*\n\n"+
		"👤 Erstellt von: %s\n"+
		"⏰ Deadline: %s Uhr\n\n",
		creatorName,
		deadlineInBerlin.Format("15:04"))

	// Aktuelle Bestellungen hinzufügen
	if len(orders) > 0 {
		text += "📋 *Aktuelle Bestellungen:*\n"
		totalMeat := 0
		totalVeggie := 0

		for _, order := range orders {
			name := b.formatUserName(&order)
			var orderText string

			if order.QuantityMeat > 0 && order.QuantityVeggie > 0 {
				orderText = fmt.Sprintf("🥩 %d mit Fleisch, 🥬 %d vegetarisch", order.QuantityMeat, order.QuantityVeggie)
			} else if order.QuantityMeat > 0 {
				if order.QuantityMeat == 1 {
					orderText = "🥩 1 mit Fleisch"
				} else {
					orderText = fmt.Sprintf("🥩 %d mit Fleisch", order.QuantityMeat)
				}
			} else if order.QuantityVeggie > 0 {
				if order.QuantityVeggie == 1 {
					orderText = "🥬 1 vegetarisch"
				} else {
					orderText = fmt.Sprintf("🥬 %d vegetarisch", order.QuantityVeggie)
				}
			}

			text += fmt.Sprintf("• %s: %s\n", name, orderText)
			totalMeat += order.QuantityMeat
			totalVeggie += order.QuantityVeggie
		}

		totalGyros := totalMeat + totalVeggie
		text += fmt.Sprintf("\n🥙 *Aktuell: %d Gyros* (🥩 %d mit Fleisch, 🥬 %d vegetarisch)\n\n", totalGyros, totalMeat, totalVeggie)
	} else {
		text += "📋 *Noch keine Bestellungen*\n\n"
	}

	text += "Schreibt eine Zahl um Gyros zu bestellen!\n"
	text += "Oder nutzt die Buttons für 🥩 Fleisch oder 🥬 vegetarisch\n\n"
	text += "Zum Beenden: Antwortet auf diese Nachricht mit /ende"

	// Inline Keyboard mit Reaction-Buttons für beide Gyros-Arten erstellen
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🥩 Mit Fleisch:", "noop"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1️⃣", "gm1"),
			tgbotapi.NewInlineKeyboardButtonData("2️⃣", "gm2"),
			tgbotapi.NewInlineKeyboardButtonData("3️⃣", "gm3"),
			tgbotapi.NewInlineKeyboardButtonData("4️⃣", "gm4"),
			tgbotapi.NewInlineKeyboardButtonData("5️⃣", "gm5"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🥬 Vegetarisch:", "noop"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1️⃣", "gv1"),
			tgbotapi.NewInlineKeyboardButtonData("2️⃣", "gv2"),
			tgbotapi.NewInlineKeyboardButtonData("3️⃣", "gv3"),
			tgbotapi.NewInlineKeyboardButtonData("4️⃣", "gv4"),
			tgbotapi.NewInlineKeyboardButtonData("5️⃣", "gv5"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❌ Stornieren", "g0"),
		),
	)

	// Nachricht editieren
	edit := tgbotapi.NewEditMessageText(originalMessage.Chat.ID, messageID, text)
	edit.ParseMode = tgbotapi.ModeMarkdown
	edit.ReplyMarkup = &keyboard

	_, err = b.api.Send(edit)
	if err != nil {
		log.Printf("Fehler beim Editieren der Gyroskop-Nachricht: %v", err)
	}
}

// loadActiveGyroskops lädt alle aktiven Gyroskops beim Bot-Start
func (b *Bot) loadActiveGyroskops() {
	gyroskops, err := b.db.GetAllActiveGyroskops()
	if err != nil {
		log.Printf("Error loading active gyroskops: %v", err)
		return
	}

	now := time.Now()
	for _, gyroskop := range gyroskops {
		// Prüfen ob das Gyroskop bereits abgelaufen ist
		if gyroskop.Deadline.Before(now) {
			// Automatisch schließen
			go b.autoCloseGyroskop(&gyroskop)
			continue
		}

		// In Cache laden
		b.activeGyroskops[gyroskop.ChatID] = &gyroskop
	}

	log.Printf("Bot started - %d active gyroskops loaded", len(b.activeGyroskops))
}

// backgroundExpiryChecker runs in a background goroutine and checks for expired gyroskops every minute
func (b *Bot) backgroundExpiryChecker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	log.Println("Background expiry checker started")

	for {
		select {
		case <-b.stopChan:
			log.Println("Background expiry checker stopping...")
			return
		case <-ticker.C:
			b.checkExpiredGyroskops()
		}
	}
}

// checkExpiredGyroskops checks for and closes any expired gyroskops
func (b *Bot) checkExpiredGyroskops() {
	now := time.Now()

	// Check all active gyroskops in cache
	for chatID, gyroskop := range b.activeGyroskops {
		if gyroskop.Deadline.Before(now) {
			log.Printf("Found expired gyroskop %d in chat %d, closing...", gyroskop.ID, chatID)
			go b.autoCloseGyroskop(gyroskop)
		}
	}

	// Also check database directly in case cache is out of sync
	gyroskops, err := b.db.GetAllActiveGyroskops()
	if err != nil {
		log.Printf("Error checking active gyroskops from database: %v", err)
		return
	}

	for _, gyroskop := range gyroskops {
		if gyroskop.Deadline.Before(now) {
			// Check if we already have this in cache (to avoid double processing)
			if cachedGyroskop, exists := b.activeGyroskops[gyroskop.ChatID]; exists && cachedGyroskop.ID == gyroskop.ID {
				// Already handled above
				continue
			}

			log.Printf("Found expired gyroskop %d in database (not in cache), closing...", gyroskop.ID)
			go b.autoCloseGyroskop(&gyroskop)
		}
	}
}

// Stop gracefully stops the bot
func (b *Bot) Stop() {
	close(b.stopChan)
}
