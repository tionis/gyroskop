package main

import (
	"log"
	"os"

	"github.com/tionis/gyroskop/internal/bot"
	"github.com/tionis/gyroskop/internal/database"
)

func main() {
	// Read bot token from environment variable
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Initialize database
	db, err := database.Init()
	if err != nil {
		log.Fatalf("Error initializing database: %v", err)
	}
	defer db.Close()

	// Start bot
	bot, err := bot.New(token, db)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	bot.Run()
}