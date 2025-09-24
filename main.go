package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

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

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start bot in a goroutine
	go bot.Run()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutdown signal received, stopping bot...")
	bot.Stop()
}
