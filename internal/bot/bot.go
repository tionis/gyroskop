package bot


import (
	"fmt"
	"log"
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
	timers          map[int]*time.Timer          // Timer für automatisches Schließen
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
		timers:          make(map[int]*time.Timer),
	}, nil
}

// Run starts the bot
func (b *Bot) Run() {
	// Load existing open gyroskops on startup
	b.loadActiveGyroskops()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		}
		if update.CallbackQuery != nil {
			b.handleCallbackQuery(update.CallbackQuery)
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
/gyroskop HH:MM - Neues Gyroskop bis zur angegebenen Uhrzeit öffnen
/status - Aktuellen Status anzeigen
/ende - Gyroskop beenden (nur als Antwort auf Gyroskop-Nachricht)
/stornieren - Eigene Bestellung stornieren
/help - Diese Hilfe anzeigen

*Bestellen:*
🥩 *Mit Fleisch:* Nutze die Buttons 1️⃣-5️⃣ unter "Mit Fleisch"
🥬 *Vegetarisch:* Nutze die Buttons 1️⃣-5️⃣ unter "Vegetarisch"
❌ *Stornieren:* Schreibe "0" oder nutze den ❌ Stornieren Button

*Beispiele:*
/gyroskop 17:00 - Öffnet Gyroskop bis 17:00 Uhr
🥩 2️⃣ Button - Bestellt 2 Gyros mit Fleisch
🥬 1️⃣ Button - Bestellt 1 vegetarisches Gyros
0 - Storniert die komplette Bestellung`

	b.sendMessage(message.Chat.ID, helpText)
}

// handleNewGyroskop creates a new gyroskop
func (b *Bot) handleNewGyroskop(message *tgbotapi.Message, args string) {
	if args == "" {
		b.sendMessage(message.Chat.ID, "⚠️ Bitte gib eine Uhrzeit an: /gyroskop 17:00")
		return
	}

	// Parse time
	deadline, err := b.parseTime(args)
	if err != nil {
		b.sendMessage(message.Chat.ID, "⚠️ Ungültiges Zeitformat. Verwende HH:MM (z.B. 17:00)")
		return
	}

	// Prüfen ob die Zeit in der Zukunft liegt
	if deadline.Before(time.Now()) {
		b.sendMessage(message.Chat.ID, "⚠️ Die angegebene Zeit liegt in der Vergangenheit!")
		return
	}

	gyroskop, err := b.db.CreateGyroskop(message.Chat.ID, int64(message.From.ID), deadline)
	if err != nil {
		log.Printf("Fehler beim Erstellen des Gyroskops: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Erstellen des Gyroskops")
		return
	}

	userName := b.getUserName(message.From)
	text := fmt.Sprintf("🥙 *Gyroskop geöffnet!*\n\n"+
		"👤 Erstellt von: %s\n"+
		"⏰ Deadline: %s\n\n"+
		"Schreibt eine Zahl um Gyros zu bestellen!\n"+
		"Oder nutzt die Reaction-Buttons: 1️⃣ 2️⃣ 3️⃣ 4️⃣ 5️⃣\n\n"+
		"Zum Beenden: Antwortet auf diese Nachricht mit /ende",
		userName,
		deadline.Format("15:04"),
	)

	// Gyroskop in Cache speichern
	b.activeGyroskops[message.Chat.ID] = gyroskop

	// Start timer for automatic closing
	duration := time.Until(deadline)
	timer := time.AfterFunc(duration, func() {
		b.autoCloseGyroskop(gyroskop)
	})
	b.timers[gyroskop.ID] = timer

	// Nachricht senden mit Reaction-Buttons und Message-ID speichern
	sentMessage := b.sendMessageWithReactions(message.Chat.ID, text)
	if sentMessage != nil {
		// Message-ID in der Datenbank speichern
		err = b.db.UpdateGyroskopMessageID(gyroskop.ID, sentMessage.MessageID)
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

// parseTime parses a time string in HH:MM format
func (b *Bot) parseTime(timeStr string) (time.Time, error) {
	now := time.Now()
	parsedTime, err := time.Parse("15:04", strings.TrimSpace(timeStr))
	if err != nil {
		return time.Time{}, err
	}

	// Kombiniere heutiges Datum mit der angegebenen Zeit
	deadline := time.Date(
		now.Year(), now.Month(), now.Day(),
		parsedTime.Hour(), parsedTime.Minute(), 0, 0,
		now.Location(),
	)

	// Wenn die Zeit heute schon vorbei ist, nimm morgen
	if deadline.Before(now) {
		deadline = deadline.AddDate(0, 0, 1)
	}

	return deadline, nil
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

	// Timer stoppen falls vorhanden
	if timer, exists := b.timers[gyroskop.ID]; exists {
		timer.Stop()
		delete(b.timers, gyroskop.ID)
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
	
	text.WriteString("📊 *Aktueller Status*\n")
	text.WriteString(fmt.Sprintf("⏰ Deadline: %s\n\n", gyroskop.Deadline.Format("15:04")))

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
	
	text.WriteString("📊 *Finale Bestellübersicht*\n")
	text.WriteString(fmt.Sprintf("⏰ Deadline war: %s\n\n", gyroskop.Deadline.Format("15:04")))

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

	// Neue Nachricht zusammenstellen
	text := fmt.Sprintf("🥙 *Gyroskop geöffnet!*\n\n"+
		"👤 Erstellt von: %s\n"+
		"⏰ Deadline: %s\n\n",
		creatorName,
		gyroskop.Deadline.Format("15:04"))

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

		// Start timer for automatic closing
		duration := time.Until(gyroskop.Deadline)
		timer := time.AfterFunc(duration, func() {
			b.autoCloseGyroskop(&gyroskop)
		})
		b.timers[gyroskop.ID] = timer
	}

	log.Printf("Bot started - %d active gyroskops loaded", len(b.activeGyroskops))
}