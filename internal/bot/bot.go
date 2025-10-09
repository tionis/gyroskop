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
	helpText := `🥙 *Gyroskop Bot - Essensbestellungen koordinieren*

*Befehle:*
/gyroskop - Neues Gyroskop für 15 Minuten öffnen (Standard: Gyros mit Fleisch und Vegetarisch)
/gyroskop HH:MM - Neues Gyroskop bis zur angegebenen Uhrzeit öffnen
/gyroskop 30min - Neues Gyroskop für 30 Minuten öffnen
/gyroskop Pizza, Margherita, Salami, Hawaii - Pizza-Gyroskop mit eigenen Optionen
/gyroskop 17:00, Burger, Beef, Chicken, Veggie - Burger-Gyroskop bis 17:00 Uhr
/gyroskop 10min, Döner, Fleisch, Vegetarisch, Dürüm - Döner-Gyroskop für 10min mit 3 Optionen
/gyroskop (als Antwort) - Gyroskop wiedereröffnen oder Optionen ändern
/status - Aktuellen Status anzeigen
/ende - Gyroskop beenden (nur Ersteller)
/stornieren - Eigene Bestellung stornieren
/help - Diese Hilfe anzeigen

*Format:* /gyroskop [Zeit], [Name], Option1, Option2, ...
  ⚠️ Wichtig: Komma-getrennt! Zeit und Name müssen durch Komma getrennt sein.

*Bestellen:*
📱 *Buttons:* Nutze die Buttons 1️⃣-5️⃣ unter jeder Option
💬 *Text:* Schreibe einfach die Anzahl und Option (z.B. "2 fleisch" oder "3 veggie")
   - Eine Zeile pro Option, oder alles in einer Zeile
   - Fuzzy Matching: "fleisch", "meat", "fl" funktionieren alle
❌ *Stornieren:* Schreibe "0" oder nutze den ❌ Stornieren Button

*Beispiele:*
/gyroskop - Standard Gyros für 15min
/gyroskop 30min - Gyros für 30min
/gyroskop Pizza, Margherita, Salami - Pizza-Gyroskop mit 3 Optionen
/gyroskop 10min, Döner, Fleisch, Vegetarisch - Döner für 10min mit 2 Optionen
2 fleisch - Bestellt 2x Fleisch
3 veggie - Bestellt 3x Vegetarisch
2 meat, 3 veggie - Bestellt 2x Fleisch und 3x Vegetarisch (mehrere in einer Zeile)
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

	// Parse deadline and food options from args
	deadline, name, foodOptions, err := b.parseGyroskopArgs(args)
	if err != nil {
		b.sendMessage(message.Chat.ID, "⚠️ Ungültiges Format. Verwende: /gyroskop [Zeit], Name, Option1, Option2, ...")
		return
	}

	// Create new gyroskop
	gyroskop, err := b.db.CreateGyroskop(message.Chat.ID, int64(message.From.ID), name, foodOptions, deadline)
	if err != nil {
		log.Printf("Fehler beim Erstellen des Gyroskops: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Erstellen des Gyroskops")
		return
	}

	b.sendGyroskopMessage(message.Chat.ID, gyroskop, "🥙 *Gyroskop geöffnet!*", message.From)
}

// handleReopenGyroskop reopens a closed gyroskop or updates deadline/options of an active one
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
		b.sendMessage(message.Chat.ID, "⚠️ Nur der Ersteller kann das Gyroskop bearbeiten!")
		return
	}

	// Parse new deadline and options
	deadline, name, foodOptions, err := b.parseGyroskopArgs(args)
	if err != nil {
		b.sendMessage(message.Chat.ID, "⚠️ Ungültiges Format. Verwende: /gyroskop [Zeit], Name, Option1, Option2, ...")
		return
	}

	// Check if this is the currently active gyroskop
	if existingGyroskop, exists := b.activeGyroskops[message.Chat.ID]; exists && existingGyroskop.ID == gyroskop.ID {
		// Update deadline of active gyroskop
		err = b.db.UpdateGyroskopDeadline(gyroskop.ID, deadline)
		if err != nil {
			log.Printf("Fehler beim Aktualisieren der Deadline: %v", err)
			b.sendMessage(message.Chat.ID, "❌ Fehler beim Aktualisieren der Deadline")
			return
		}

		// Update name and options if provided
		if args != "" {
			err = b.db.UpdateGyroskopOptions(gyroskop.ID, name, foodOptions)
			if err != nil {
				log.Printf("Fehler beim Aktualisieren der Optionen: %v", err)
				b.sendMessage(message.Chat.ID, "❌ Fehler beim Aktualisieren der Optionen")
				return
			}
		}

		// Update cache
		existingGyroskop.Deadline = deadline
		existingGyroskop.Name = name
		existingGyroskop.FoodOptions = foodOptions

		berlin, _ := time.LoadLocation("Europe/Berlin")
		deadlineInBerlin := deadline.In(berlin)

		b.sendMessage(message.Chat.ID, fmt.Sprintf("⏰ *Aktualisiert!*\n\nName: %s\nDeadline: %s Uhr\nOptionen: %s", name, deadlineInBerlin.Format("15:04"), strings.Join(foodOptions, ", ")))

		// Update the gyroskop message with new deadline
		b.updateGyroskopMessage(existingGyroskop, replyMessage)
		return
	}

	// Check if there's a different active gyroskop
	if existingGyroskop, exists := b.activeGyroskops[message.Chat.ID]; exists && existingGyroskop.ID != gyroskop.ID {
		berlin, _ := time.LoadLocation("Europe/Berlin")
		deadlineInBerlin := existingGyroskop.Deadline.In(berlin)
		b.sendMessage(message.Chat.ID, fmt.Sprintf("⚠️ Es gibt bereits ein anderes aktives Gyroskop bis %s. Beende es zuerst.", deadlineInBerlin.Format("15:04")))
		return
	}

	// This is a closed gyroskop, reopen it
	err = b.db.ReopenGyroskop(gyroskop.ID, deadline)
	if err != nil {
		log.Printf("Fehler beim Wiedereröffnen des Gyroskops: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Wiedereröffnen des Gyroskops")
		return
	}

	// Update gyroskop data
	gyroskop.Deadline = deadline
	gyroskop.Name = name
	gyroskop.FoodOptions = foodOptions
	gyroskop.IsOpen = true

	b.sendGyroskopMessage(message.Chat.ID, gyroskop, "🔄 *Gyroskop wiedereröffnet!*", message.From)
}

// sendGyroskopMessage sends the gyroskop message with proper formatting
func (b *Bot) sendGyroskopMessage(chatID int64, gyroskop *database.Gyroskop, title string, user *tgbotapi.User) {
	userName := b.getUserName(user)

	// Convert deadline to Berlin timezone for display
	berlin, _ := time.LoadLocation("Europe/Berlin")
	deadlineInBerlin := gyroskop.Deadline.In(berlin)

	// Generate example orders
	var examples []string
	for _, option := range gyroskop.FoodOptions {
		examples = append(examples, fmt.Sprintf("'2 %s'", strings.ToLower(option)))
	}

	text := fmt.Sprintf("%s\n\n"+
		"👤 Erstellt von: %s\n"+
		"⏰ Deadline: %s Uhr\n\n"+
		"Zum Bestellen schreibt %s oder nutzt die Buttons unten.\n\n"+
		"Zum Beenden: /ende",
		title,
		userName,
		deadlineInBerlin.Format("15:04"),
		strings.Join(examples, ", "),
	)

	// Add gyroskop to cache
	b.activeGyroskops[chatID] = gyroskop

	// Send message with reaction buttons and save message ID
	sentMessage := b.sendMessageWithReactions(chatID, text, gyroskop.FoodOptions)
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
	text := strings.TrimSpace(strings.ToLower(message.Text))

	// Check if there's an active gyroskop
	gyroskop, exists := b.activeGyroskops[message.Chat.ID]
	if !exists {
		return // Ignore if no active gyroskop
	}

	// Check if the gyroskop is still open
	if time.Now().After(gyroskop.Deadline) {
		b.sendMessage(message.Chat.ID, "⏰ Das Gyroskop ist bereits abgelaufen!")
		return
	}

	userName := b.getUserName(message.From)

	// Handle cancellation (0)
	if text == "0" {
		err := b.db.RemoveOrder(gyroskop.ID, int64(message.From.ID))
		if err != nil {
			log.Printf("Error canceling order: %v", err)
			return
		}
		b.sendMessage(message.Chat.ID, fmt.Sprintf("❌ %s hat die Bestellung storniert", userName))
		// Update the gyroskop message with current orders
		b.updateGyroskopMessage(gyroskop, message)
		return
	}

	// Parse order syntax using shortcodes generated from food options
	quantities := b.parseOrderText(text, gyroskop.FoodOptions)
	if quantities == nil {
		return // Ignore invalid formats
	}

	// Check if all quantities are 0
	hasQuantity := false
	for _, qty := range quantities {
		if qty > 0 {
			hasQuantity = true
			break
		}
	}
	if !hasQuantity {
		return
	}

	// Add or update order
	err := b.db.AddOrUpdateOrder(
		gyroskop.ID,
		int64(message.From.ID),
		message.From.UserName,
		message.From.FirstName,
		message.From.LastName,
		quantities,
	)
	if err != nil {
		log.Printf("Error adding order: %v", err)
		b.sendMessage(message.Chat.ID, "❌ Fehler beim Bestellen")
		return
	}

	// Format response message
	orderText := b.formatOrderQuantities(quantities, gyroskop.FoodOptions)
	b.sendMessage(message.Chat.ID, fmt.Sprintf("✅ %s: %s", userName, orderText))

	// Update the gyroskop message with current orders
	b.updateGyroskopMessage(gyroskop, message)
}

// parseOrderText parses order text using fuzzy matching
// Supports formats like:
//
//	"2 fleisch" - single order
//	"2 fleisch, 3 veggie" - multiple orders in one line (comma separated)
//	"2 meat\n3 veggie" - multiple orders on separate lines
//
// Returns map of food option to quantity, or nil if invalid format
func (b *Bot) parseOrderText(text string, foodOptions []string) map[string]int {
	quantities := make(map[string]int)

	// Split by newlines and commas to handle both formats
	lines := strings.Split(text, "\n")
	var parts []string
	for _, line := range lines {
		lineParts := strings.Split(line, ",")
		parts = append(parts, lineParts...)
	}

	// Pattern to match: number followed by text
	// Examples: "2 fleisch", "3meat", "1 veggie"
	orderRegex := regexp.MustCompile(`^\s*(\d+)\s*(.+?)\s*$`)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		matches := orderRegex.FindStringSubmatch(part)
		if len(matches) != 3 {
			continue // Invalid format for this part, skip it
		}

		quantity, err := strconv.Atoi(matches[1])
		if err != nil || quantity < 0 || quantity > 10 {
			continue // Invalid quantity, skip it
		}

		optionText := strings.TrimSpace(matches[2])

		// Use fuzzy matching to find the option
		matchedOption, found := database.FuzzyMatchOption(optionText, foodOptions)
		if !found {
			continue // No match found, skip it
		}

		// Add to quantities (if already exists, overwrite)
		quantities[matchedOption] = quantity
	}

	// If we didn't parse any valid orders, return nil
	if len(quantities) == 0 {
		return nil
	}

	return quantities
}

// formatOrderQuantities formats quantities map into a readable string
func (b *Bot) formatOrderQuantities(quantities map[string]int, foodOptions []string) string {
	var parts []string

	for _, option := range foodOptions {
		if qty, ok := quantities[option]; ok && qty > 0 {
			if qty == 1 {
				parts = append(parts, fmt.Sprintf("1 %s", option))
			} else {
				parts = append(parts, fmt.Sprintf("%d %s", qty, option))
			}
		}
	}

	return strings.Join(parts, ", ")
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

// parseGyroskopArgs parses the gyroskop command arguments
// Format (comma-separated): [time], [name], option1, option2, ...
// Examples:
//
//	/gyroskop -> default (15min, "Gyros", ["Fleisch", "Vegetarisch"])
//	/gyroskop 17:00 -> until 17:00, default name and options
//	/gyroskop Pizza, Margherita, Salami, Hawaii -> default time (15min), Pizza with 3 options
//	/gyroskop 30min, Burger, Beef, Chicken, Veggie -> 30min, Burger with 3 options
//	/gyroskop 10min, Döner, Fleisch, Vegetarisch, Dürüm -> 10min, Döner with 3 options
func (b *Bot) parseGyroskopArgs(args string) (time.Time, string, []string, error) {
	args = strings.TrimSpace(args)

	// Default values
	name := "Gyros"
	foodOptions := []string{"Fleisch", "Vegetarisch"}
	deadline := time.Now().Add(15 * time.Minute)

	// If no args, return defaults
	if args == "" {
		return deadline, name, foodOptions, nil
	}

	// Split by comma
	parts := strings.Split(args, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	// Try to parse first part as deadline
	firstPartDeadline, err := b.parseDeadline(parts[0])
	startIdx := 0
	if err == nil {
		// First part is a deadline
		deadline = firstPartDeadline
		startIdx = 1
	}

	// If we have remaining parts, parse them as name and food options
	if len(parts) > startIdx {
		// Next part is the name
		if len(parts) > startIdx && parts[startIdx] != "" {
			name = parts[startIdx]
			startIdx++
		}

		// Remaining parts are food options
		if len(parts) > startIdx {
			foodOptions = parts[startIdx:]
			// Filter out empty strings
			filtered := make([]string, 0, len(foodOptions))
			for _, opt := range foodOptions {
				if opt != "" {
					filtered = append(filtered, opt)
				}
			}
			if len(filtered) > 0 {
				foodOptions = filtered
			}
		}
	}

	return deadline, name, foodOptions, nil
}

// handleEndGyroskop ends the gyroskop created by the user
func (b *Bot) handleEndGyroskop(message *tgbotapi.Message) {
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
	if !strings.HasPrefix(data, "g") {
		b.answerCallbackQuery(query.ID, "❌ Ungültige Callback-Daten")
		return
	}

	// Format: g<optionIndex>_<quantity>
	// Example: g0_2 means option 0, quantity 2
	parts := strings.TrimPrefix(data, "g")

	// Special case: g0 means cancel
	if parts == "0" {
		b.handleCancelOrderCallback(query)
		return
	}

	// Parse format: <index>_<quantity>
	splitParts := strings.Split(parts, "_")
	if len(splitParts) != 2 {
		b.answerCallbackQuery(query.ID, "❌ Ungültiges Format")
		return
	}

	optionIndex, err := strconv.Atoi(splitParts[0])
	if err != nil {
		b.answerCallbackQuery(query.ID, "❌ Ungültiger Index")
		return
	}

	quantity, err := strconv.Atoi(splitParts[1])
	if err != nil || quantity < 0 || quantity > 10 {
		b.answerCallbackQuery(query.ID, "❌ Ungültige Anzahl")
		return
	}

	gyroskop, exists := b.activeGyroskops[query.Message.Chat.ID]
	if !exists {
		b.answerCallbackQuery(query.ID, "❌ Kein aktives Gyroskop")
		return
	}

	// Check if the gyroskop is still open
	if !gyroskop.IsOpen {
		b.answerCallbackQuery(query.ID, "❌ Gyroskop ist bereits geschlossen")
		return
	}

	// Validate option index
	if optionIndex < 0 || optionIndex >= len(gyroskop.FoodOptions) {
		b.answerCallbackQuery(query.ID, "❌ Ungültige Option")
		return
	}

	selectedOption := gyroskop.FoodOptions[optionIndex]

	// Load current order
	currentQuantities := make(map[string]int)
	existingOrder, err := b.db.GetOrder(gyroskop.ID, int64(query.From.ID))
	if err == nil {
		currentQuantities = existingOrder.Quantities
	}

	// Update quantity for selected option
	currentQuantities[selectedOption] = quantity

	// Add or update order
	err = b.db.AddOrUpdateOrder(
		gyroskop.ID,
		int64(query.From.ID),
		query.From.UserName,
		query.From.FirstName,
		query.From.LastName,
		currentQuantities,
	)
	if err != nil {
		log.Printf("Error adding order: %v", err)
		b.answerCallbackQuery(query.ID, "❌ Fehler beim Bestellen")
		return
	}

	var responseText string
	if quantity == 1 {
		responseText = fmt.Sprintf("✅ 1 %s", selectedOption)
	} else {
		responseText = fmt.Sprintf("✅ %d %s", quantity, selectedOption)
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

	totals := make(map[string]int)

	for _, order := range orders {
		name := b.formatUserName(&order)
		orderText := b.formatOrderQuantities(order.Quantities, gyroskop.FoodOptions)

		text.WriteString(fmt.Sprintf("• %s: %s\n", name, orderText))

		// Add to totals
		for option, qty := range order.Quantities {
			totals[option] += qty
		}
	}

	// Format totals
	var totalItems int
	var totalParts []string
	for _, option := range gyroskop.FoodOptions {
		if qty, ok := totals[option]; ok && qty > 0 {
			totalItems += qty
			totalParts = append(totalParts, fmt.Sprintf("%d %s", qty, option))
		}
	}

	text.WriteString(fmt.Sprintf("\n🥙 *Gesamt: %d* (%s)", totalItems, strings.Join(totalParts, ", ")))
	return text.String()
}

// formatOrderSummary formatiert die finale Bestellübersicht
func (b *Bot) formatOrderSummary(gyroskop *database.Gyroskop, orders []database.Order) string {
	var text strings.Builder

	// Convert deadline to Berlin timezone for display
	berlin, _ := time.LoadLocation("Europe/Berlin")
	deadlineInBerlin := gyroskop.Deadline.In(berlin)

	text.WriteString(fmt.Sprintf("📊 *%s - Finale Bestellübersicht*\n", gyroskop.Name))
	text.WriteString(fmt.Sprintf("⏰ Deadline war: %s Uhr\n\n", deadlineInBerlin.Format("15:04")))

	if len(orders) == 0 {
		text.WriteString("Keine Bestellungen eingegangen 😢")
		return text.String()
	}

	totals := make(map[string]int)

	for _, order := range orders {
		name := b.formatUserName(&order)
		orderText := b.formatOrderQuantities(order.Quantities, gyroskop.FoodOptions)

		text.WriteString(fmt.Sprintf("• %s: %s\n", name, orderText))

		// Add to totals
		for option, qty := range order.Quantities {
			totals[option] += qty
		}
	}

	// Format totals
	var totalItems int
	var totalParts []string
	for _, option := range gyroskop.FoodOptions {
		if qty, ok := totals[option]; ok && qty > 0 {
			totalItems += qty
			totalParts = append(totalParts, fmt.Sprintf("%d %s", qty, option))
		}
	}

	text.WriteString(fmt.Sprintf("\n🥙 *Gesamt: %d* (%s)", totalItems, strings.Join(totalParts, ", ")))
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

// createFoodOptionsKeyboard creates an inline keyboard based on food options
func (b *Bot) createFoodOptionsKeyboard(foodOptions []string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	// Create rows for each food option
	for i, option := range foodOptions {
		// Add header row
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(option+":", "noop"),
		))

		// Add button row for quantities 1-5
		// Format: g<index>_<quantity>
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("1️⃣", fmt.Sprintf("g%d_1", i)),
			tgbotapi.NewInlineKeyboardButtonData("2️⃣", fmt.Sprintf("g%d_2", i)),
			tgbotapi.NewInlineKeyboardButtonData("3️⃣", fmt.Sprintf("g%d_3", i)),
			tgbotapi.NewInlineKeyboardButtonData("4️⃣", fmt.Sprintf("g%d_4", i)),
			tgbotapi.NewInlineKeyboardButtonData("5️⃣", fmt.Sprintf("g%d_5", i)),
		))
	}

	// Add cancel button
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("❌ Stornieren", "g0"),
	))

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// sendMessageWithReactions sendet eine Nachricht mit Reaction-Buttons
func (b *Bot) sendMessageWithReactions(chatID int64, text string, foodOptions []string) *tgbotapi.Message {
	keyboard := b.createFoodOptionsKeyboard(foodOptions)

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
	text := fmt.Sprintf("🥙 *%s geöffnet!*\n\n"+
		"👤 Erstellt von: %s\n"+
		"⏰ Deadline: %s Uhr\n\n",
		gyroskop.Name,
		creatorName,
		deadlineInBerlin.Format("15:04"))

	// Aktuelle Bestellungen hinzufügen
	if len(orders) > 0 {
		text += "📋 *Aktuelle Bestellungen:*\n"
		totals := make(map[string]int)

		for _, order := range orders {
			name := b.formatUserName(&order)
			orderText := b.formatOrderQuantities(order.Quantities, gyroskop.FoodOptions)

			text += fmt.Sprintf("• %s: %s\n", name, orderText)

			// Add to totals
			for option, qty := range order.Quantities {
				totals[option] += qty
			}
		}

		// Format totals
		var totalItems int
		var totalParts []string
		for _, option := range gyroskop.FoodOptions {
			if qty, ok := totals[option]; ok && qty > 0 {
				totalItems += qty
				totalParts = append(totalParts, fmt.Sprintf("%d %s", qty, option))
			}
		}

		text += fmt.Sprintf("\n🥙 *Aktuell: %d* (%s)\n\n", totalItems, strings.Join(totalParts, ", "))
	} else {
		text += "📋 *Noch keine Bestellungen*\n\n"
	}

	// Generate example orders
	var examples []string
	for _, option := range gyroskop.FoodOptions {
		examples = append(examples, fmt.Sprintf("'2 %s'", strings.ToLower(option)))
	}

	text += fmt.Sprintf("Zum Bestellen schreibt %s oder nutzt die Buttons unten.\n\n", strings.Join(examples, ", "))
	text += "Zum Beenden: /ende"

	// Inline Keyboard mit Reaction-Buttons dynamisch erstellen
	keyboard := b.createFoodOptionsKeyboard(gyroskop.FoodOptions)

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
