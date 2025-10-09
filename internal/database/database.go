package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/lithammer/fuzzysearch/fuzzy"
)

type DB struct {
	*sql.DB
}

type Gyroskop struct {
	ID          int       `json:"id"`
	ChatID      int64     `json:"chat_id"`
	CreatedBy   int64     `json:"created_by"`
	MessageID   int       `json:"message_id"`
	Name        string    `json:"name"`
	FoodOptions []string  `json:"food_options"`
	Deadline    time.Time `json:"deadline"`
	IsOpen      bool      `json:"is_open"`
	CreatedAt   time.Time `json:"created_at"`
}

type Order struct {
	ID         int            `json:"id"`
	GyroskopID int            `json:"gyroskop_id"`
	UserID     int64          `json:"user_id"`
	Username   string         `json:"username"`
	FirstName  string         `json:"first_name"`
	LastName   string         `json:"last_name"`
	Quantities map[string]int `json:"quantities"` // Map of food option to quantity
	CreatedAt  time.Time      `json:"created_at"`
}

// Init initializes the PostgreSQL database
func Init() (*DB, error) {
	// Get database connection info from environment variables
	host := getEnvOrDefault("POSTGRES_HOST", "localhost")
	port := getEnvOrDefault("POSTGRES_PORT", "5432")
	user := getEnvOrDefault("POSTGRES_USER", "gyroskop")
	password := getEnvOrDefault("POSTGRES_PASSWORD", "password")
	dbname := getEnvOrDefault("POSTGRES_DB", "gyroskop")
	sslmode := getEnvOrDefault("POSTGRES_SSLMODE", "disable")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	dbWrapper := &DB{db}
	if err := dbWrapper.createTables(); err != nil {
		return nil, err
	}

	return dbWrapper, nil
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// createTables creates the necessary tables
func (db *DB) createTables() error {
	gyroskopTable := `
	CREATE TABLE IF NOT EXISTS gyroskops (
		id SERIAL PRIMARY KEY,
		chat_id BIGINT NOT NULL,
		created_by BIGINT NOT NULL,
		message_id INTEGER DEFAULT 0,
		name TEXT NOT NULL DEFAULT 'Gyros',
		food_options JSONB NOT NULL DEFAULT '["Fleisch", "Vegetarisch"]'::jsonb,
		deadline TIMESTAMP NOT NULL,
		is_open BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	ordersTable := `
	CREATE TABLE IF NOT EXISTS orders (
		id SERIAL PRIMARY KEY,
		gyroskop_id INTEGER NOT NULL,
		user_id BIGINT NOT NULL,
		username TEXT,
		first_name TEXT,
		last_name TEXT,
		quantities JSONB NOT NULL DEFAULT '{}'::jsonb,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (gyroskop_id) REFERENCES gyroskops (id),
		UNIQUE(gyroskop_id, user_id)
	);`

	if _, err := db.Exec(gyroskopTable); err != nil {
		return err
	}

	if _, err := db.Exec(ordersTable); err != nil {
		return err
	}

	return nil
}

// CreateGyroskop creates a new gyroskop
func (db *DB) CreateGyroskop(chatID, createdBy int64, name string, foodOptions []string, deadline time.Time) (*Gyroskop, error) {
	// Default food options if none provided
	if len(foodOptions) == 0 {
		foodOptions = []string{"Fleisch", "Vegetarisch"}
	}

	// Default name if none provided
	if name == "" {
		name = "Gyros"
	}

	foodOptionsJSON, err := json.Marshal(foodOptions)
	if err != nil {
		return nil, err
	}

	var id int
	err = db.QueryRow(`
		INSERT INTO gyroskops (chat_id, created_by, name, food_options, deadline)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		chatID, createdBy, name, foodOptionsJSON, deadline,
	).Scan(&id)
	if err != nil {
		return nil, err
	}

	return &Gyroskop{
		ID:          id,
		ChatID:      chatID,
		CreatedBy:   createdBy,
		Name:        name,
		FoodOptions: foodOptions,
		Deadline:    deadline,
		IsOpen:      true,
		CreatedAt:   time.Now(),
	}, nil
}

// UpdateGyroskopMessageID updates the message ID for a gyroskop
func (db *DB) UpdateGyroskopMessageID(gyroskopID, messageID int) error {
	_, err := db.Exec(`
		UPDATE gyroskops SET message_id = $1 WHERE id = $2`,
		messageID, gyroskopID,
	)
	return err
}

// GetActiveGyroskop gets the active gyroskop for a chat
func (db *DB) GetActiveGyroskop(chatID int64) (*Gyroskop, error) {
	row := db.QueryRow(`
		SELECT id, chat_id, created_by, message_id, name, food_options, deadline, is_open, created_at
		FROM gyroskops WHERE chat_id = $1 AND is_open = true`,
		chatID,
	)

	var g Gyroskop
	var foodOptionsJSON []byte
	err := row.Scan(&g.ID, &g.ChatID, &g.CreatedBy, &g.MessageID, &g.Name, &foodOptionsJSON, &g.Deadline, &g.IsOpen, &g.CreatedAt)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(foodOptionsJSON, &g.FoodOptions); err != nil {
		return nil, err
	}

	return &g, nil
}

// GetAllActiveGyroskops gets all active gyroskops
func (db *DB) GetAllActiveGyroskops() ([]Gyroskop, error) {
	rows, err := db.Query(`
		SELECT id, chat_id, created_by, message_id, name, food_options, deadline, is_open, created_at
		FROM gyroskops WHERE is_open = true`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var gyroskops []Gyroskop
	for rows.Next() {
		var g Gyroskop
		var foodOptionsJSON []byte
		err := rows.Scan(&g.ID, &g.ChatID, &g.CreatedBy, &g.MessageID, &g.Name, &foodOptionsJSON, &g.Deadline, &g.IsOpen, &g.CreatedAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(foodOptionsJSON, &g.FoodOptions); err != nil {
			return nil, err
		}

		gyroskops = append(gyroskops, g)
	}

	return gyroskops, rows.Err()
}

// CloseGyroskop closes a gyroskop
func (db *DB) CloseGyroskop(gyroskopID int) error {
	_, err := db.Exec(`
		UPDATE gyroskops SET is_open = false WHERE id = $1`,
		gyroskopID,
	)
	return err
}

// AddOrUpdateOrder adds or updates an order
func (db *DB) AddOrUpdateOrder(gyroskopID int, userID int64, username, firstName, lastName string, quantities map[string]int) error {
	quantitiesJSON, err := json.Marshal(quantities)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO orders (gyroskop_id, user_id, username, first_name, last_name, quantities, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, CURRENT_TIMESTAMP)
		ON CONFLICT (gyroskop_id, user_id) 
		DO UPDATE SET 
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			quantities = EXCLUDED.quantities,
			created_at = CURRENT_TIMESTAMP`,
		gyroskopID, userID, username, firstName, lastName, quantitiesJSON,
	)
	return err
}

// GetOrdersByGyroskop gets all orders for a gyroskop
func (db *DB) GetOrdersByGyroskop(gyroskopID int) ([]Order, error) {
	rows, err := db.Query(`
		SELECT id, gyroskop_id, user_id, username, first_name, last_name, quantities, created_at
		FROM orders WHERE gyroskop_id = $1 ORDER BY created_at`,
		gyroskopID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		var quantitiesJSON []byte
		err := rows.Scan(&o.ID, &o.GyroskopID, &o.UserID, &o.Username, &o.FirstName, &o.LastName, &quantitiesJSON, &o.CreatedAt)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(quantitiesJSON, &o.Quantities); err != nil {
			return nil, err
		}

		// Ensure quantities map is initialized
		if o.Quantities == nil {
			o.Quantities = make(map[string]int)
		}

		// Filter out orders with no quantities
		hasQuantity := false
		for _, qty := range o.Quantities {
			if qty > 0 {
				hasQuantity = true
				break
			}
		}

		if hasQuantity {
			orders = append(orders, o)
		}
	}

	return orders, rows.Err()
}

// RemoveOrder removes an order (sets quantities to empty)
func (db *DB) RemoveOrder(gyroskopID int, userID int64) error {
	_, err := db.Exec(`
		UPDATE orders SET quantities = '{}'::jsonb WHERE gyroskop_id = $1 AND user_id = $2`,
		gyroskopID, userID,
	)
	return err
}

// GetOrder gets a specific order
func (db *DB) GetOrder(gyroskopID int, userID int64) (*Order, error) {
	row := db.QueryRow(`
		SELECT id, gyroskop_id, user_id, username, first_name, last_name, quantities, created_at
		FROM orders WHERE gyroskop_id = $1 AND user_id = $2`,
		gyroskopID, userID,
	)

	var o Order
	var quantitiesJSON []byte
	err := row.Scan(&o.ID, &o.GyroskopID, &o.UserID, &o.Username, &o.FirstName, &o.LastName, &quantitiesJSON, &o.CreatedAt)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(quantitiesJSON, &o.Quantities); err != nil {
		return nil, err
	}

	return &o, nil
}

// GetGyroskopByMessageID gets a gyroskop by its message ID
func (db *DB) GetGyroskopByMessageID(chatID int64, messageID int) (*Gyroskop, error) {
	row := db.QueryRow(`
		SELECT id, chat_id, created_by, message_id, name, food_options, deadline, is_open, created_at
		FROM gyroskops WHERE chat_id = $1 AND message_id = $2`,
		chatID, messageID,
	)

	var g Gyroskop
	var foodOptionsJSON []byte
	err := row.Scan(&g.ID, &g.ChatID, &g.CreatedBy, &g.MessageID, &g.Name, &foodOptionsJSON, &g.Deadline, &g.IsOpen, &g.CreatedAt)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(foodOptionsJSON, &g.FoodOptions); err != nil {
		return nil, err
	}

	return &g, nil
}

// ReopenGyroskop reopens a closed gyroskop with a new deadline
func (db *DB) ReopenGyroskop(gyroskopID int, deadline time.Time) error {
	_, err := db.Exec(`
		UPDATE gyroskops SET is_open = true, deadline = $1 WHERE id = $2`,
		deadline, gyroskopID,
	)
	return err
}

// UpdateGyroskopDeadline updates the deadline of an active gyroskop
func (db *DB) UpdateGyroskopDeadline(gyroskopID int, deadline time.Time) error {
	_, err := db.Exec(`
		UPDATE gyroskops SET deadline = $1 WHERE id = $2`,
		deadline, gyroskopID,
	)
	return err
}

// UpdateGyroskopOptions updates the name and food options of a gyroskop
func (db *DB) UpdateGyroskopOptions(gyroskopID int, name string, foodOptions []string) error {
	foodOptionsJSON, err := json.Marshal(foodOptions)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		UPDATE gyroskops SET name = $1, food_options = $2 WHERE id = $3`,
		name, foodOptionsJSON, gyroskopID,
	)
	return err
}

// FuzzyMatchOption finds the best matching food option using fuzzy matching
// Returns the matched option and true if a match is found, empty string and false otherwise
func FuzzyMatchOption(input string, options []string) (string, bool) {
	cleanedInput := strings.TrimSpace(input)
	if len(cleanedInput) == 0 {
		return "", false
	}
	results := fuzzy.FindFold(cleanedInput, options)
	if len(results) >= 1 {
		return results[0], true
	} else {
		return "", false
	}
}
