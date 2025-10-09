package database

import (
	"os"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) *DB {
	// Set test database environment variables
	os.Setenv("DB_HOST", "localhost")
	os.Setenv("DB_PORT", "5432")
	os.Setenv("DB_NAME", "gyroskop_test")
	os.Setenv("DB_USER", "gyroskop")
	os.Setenv("DB_PASSWORD", "gyroskop123")
	os.Setenv("DB_SSLMODE", "disable")

	db, err := Init()
	if err != nil {
		t.Skipf("Skipping test - PostgreSQL not available: %v", err)
		return nil
	}

	// Clean up any existing test data
	db.Exec("DELETE FROM orders")
	db.Exec("DELETE FROM gyroskops")

	return db
}

func TestDatabaseInit(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	// Check if tables exist
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('gyroskops', 'orders')").Scan(&count)
	if err != nil {
		t.Fatalf("Error checking tables: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 tables, found: %d", count)
	}
}

func TestCreateGyroskop(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	deadline := time.Now().Add(2 * time.Hour)
	chatID := int64(12345)
	createdBy := int64(67890)
	name := "Test Gyros"
	foodOptions := []string{"Fleisch", "Vegetarisch"}

	gyroskop, err := db.CreateGyroskop(chatID, createdBy, name, foodOptions, deadline)
	if err != nil {
		t.Fatalf("Error creating gyroskop: %v", err)
	}

	if gyroskop.ID == 0 {
		t.Error("Gyroskop ID should not be 0")
	}

	if gyroskop.ChatID != chatID {
		t.Errorf("Expected ChatID %d, got: %d", chatID, gyroskop.ChatID)
	}

	if gyroskop.CreatedBy != createdBy {
		t.Errorf("Expected CreatedBy %d, got: %d", createdBy, gyroskop.CreatedBy)
	}

	if gyroskop.Name != name {
		t.Errorf("Expected Name %s, got: %s", name, gyroskop.Name)
	}

	if len(gyroskop.FoodOptions) != len(foodOptions) {
		t.Errorf("Expected %d food options, got: %d", len(foodOptions), len(gyroskop.FoodOptions))
	}

	if !gyroskop.IsOpen {
		t.Error("Gyroskop should be open")
	}
}

func TestAddOrder(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	// Create gyroskop
	chatID := int64(12345)
	createdBy := int64(67890)
	deadline := time.Now().Add(2 * time.Hour)
	name := "Test Gyros"
	foodOptions := []string{"Fleisch", "Vegetarisch"}

	gyroskop, err := db.CreateGyroskop(chatID, createdBy, name, foodOptions, deadline)
	if err != nil {
		t.Fatalf("Error creating gyroskop: %v", err)
	}

	// Add order
	userID := int64(11111)
	username := "testuser"
	firstName := "Test"
	lastName := "User"
	quantities := map[string]int{"Fleisch": 3, "Vegetarisch": 1}

	err = db.AddOrUpdateOrder(gyroskop.ID, userID, username, firstName, lastName, quantities)
	if err != nil {
		t.Fatalf("Error adding order: %v", err)
	}

	// Get orders
	orders, err := db.GetOrdersByGyroskop(gyroskop.ID)
	if err != nil {
		t.Fatalf("Error getting orders: %v", err)
	}

	if len(orders) != 1 {
		t.Errorf("Expected 1 order, got: %d", len(orders))
	}

	order := orders[0]
	if order.UserID != userID {
		t.Errorf("Expected UserID %d, got: %d", userID, order.UserID)
	}

	if order.Quantities["Fleisch"] != 3 {
		t.Errorf("Expected Fleisch quantity 3, got: %d", order.Quantities["Fleisch"])
	}

	if order.Quantities["Vegetarisch"] != 1 {
		t.Errorf("Expected Vegetarisch quantity 1, got: %d", order.Quantities["Vegetarisch"])
	}
}

func TestCloseGyroskop(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	// Create gyroskop
	chatID := int64(12345)
	createdBy := int64(67890)
	deadline := time.Now().Add(2 * time.Hour)
	name := "Test Gyros"
	foodOptions := []string{"Fleisch", "Vegetarisch"}

	gyroskop, err := db.CreateGyroskop(chatID, createdBy, name, foodOptions, deadline)
	if err != nil {
		t.Fatalf("Error creating gyroskop: %v", err)
	}

	// Close gyroskop
	err = db.CloseGyroskop(gyroskop.ID)
	if err != nil {
		t.Fatalf("Error closing gyroskop: %v", err)
	}

	// Check that there is no active gyroskop
	_, err = db.GetActiveGyroskop(chatID)
	if err == nil {
		t.Error("Es sollte kein aktives Gyroskop mehr geben")
	}
}

func TestGetAllActiveGyroskops(t *testing.T) {
	db := setupTestDB(t)
	if db == nil {
		return
	}
	defer db.Close()

	// Mehrere Gyroskops erstellen
	chatID1 := int64(12345)
	chatID2 := int64(54321)
	createdBy := int64(67890)
	deadline := time.Now().Add(2 * time.Hour)
	name := "Test Gyros"
	foodOptions := []string{"Fleisch", "Vegetarisch"}

	_, err := db.CreateGyroskop(chatID1, createdBy, name, foodOptions, deadline)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des ersten Gyroskops: %v", err)
	}

	_, err = db.CreateGyroskop(chatID2, createdBy, name, foodOptions, deadline)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des zweiten Gyroskops: %v", err)
	}

	// Get all active gyroskops
	gyroskops, err := db.GetAllActiveGyroskops()
	if err != nil {
		t.Fatalf("Error getting all active gyroskops: %v", err)
	}

	if len(gyroskops) != 2 {
		t.Errorf("Expected 2 aktive Gyroskops, got: %d", len(gyroskops))
	}
}

func TestFuzzyMatchOption(t *testing.T) {
	options := []string{"Fleisch", "Vegetarisch", "Margherita", "Hawaiian Pizza", "Mit Käse"}

	tests := []struct {
		name      string
		input     string
		options   []string
		want      string
		wantFound bool
	}{
		// Exact matches (case insensitive)
		{
			name:      "exact match lowercase",
			input:     "fleisch",
			options:   options,
			want:      "Fleisch",
			wantFound: true,
		},
		{
			name:      "exact match uppercase",
			input:     "FLEISCH",
			options:   options,
			want:      "Fleisch",
			wantFound: true,
		},
		{
			name:      "exact match mixed case",
			input:     "VeGeTaRiScH",
			options:   options,
			want:      "Vegetarisch",
			wantFound: true,
		},
		// Prefix matches
		{
			name:      "prefix match short",
			input:     "fl",
			options:   options,
			want:      "Fleisch",
			wantFound: true,
		},
		{
			name:      "prefix match longer",
			input:     "veg",
			options:   options,
			want:      "Vegetarisch",
			wantFound: true,
		},
		{
			name:      "prefix match multi-word",
			input:     "haw",
			options:   options,
			want:      "Hawaiian Pizza",
			wantFound: true,
		},
		// Contains matches
		{
			name:      "contains match pizza",
			input:     "pizza",
			options:   options,
			want:      "Hawaiian Pizza",
			wantFound: true,
		},
		{
			name:      "contains match rita",
			input:     "rita",
			options:   options,
			want:      "Margherita",
			wantFound: true,
		},
		{
			name:      "contains match cheese",
			input:     "käse",
			options:   options,
			want:      "Mit Käse",
			wantFound: true,
		},
		// Reverse contains (input contains option)
		// {
		// 	name:      "reverse contains",
		// 	input:     "i want fleisch please",
		// 	options:   options,
		// 	want:      "Fleisch",
		// 	wantFound: true,
		// },
		// No match
		{
			name:      "no match",
			input:     "xyz",
			options:   options,
			want:      "",
			wantFound: false,
		},
		{
			name:      "empty input",
			input:     "",
			options:   options,
			want:      "",
			wantFound: false,
		},
		{
			name:      "whitespace only",
			input:     "   ",
			options:   options,
			want:      "",
			wantFound: false,
		},
		// Edge cases
		{
			name:      "single character match",
			input:     "f",
			options:   options,
			want:      "Fleisch",
			wantFound: true,
		},
		{
			name:      "match with extra spaces",
			input:     "  fleisch  ",
			options:   options,
			want:      "Fleisch",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := FuzzyMatchOption(tt.input, tt.options)
			if found != tt.wantFound {
				t.Errorf("FuzzyMatchOption() found = %v, want %v", found, tt.wantFound)
			}
			if got != tt.want {
				t.Errorf("FuzzyMatchOption() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFuzzyMatchOption_Priority(t *testing.T) {
	// Test that exact match has priority over prefix/contains
	options := []string{"Fleisch", "Fleischkäse"}

	got, found := FuzzyMatchOption("fleisch", options)
	if !found {
		t.Fatal("Expected to find a match")
	}
	if got != "Fleisch" {
		t.Errorf("Expected exact match 'Fleisch', got %v", got)
	}
}

func TestFuzzyMatchOption_EmptyOptions(t *testing.T) {
	options := []string{}

	got, found := FuzzyMatchOption("fleisch", options)
	if found {
		t.Error("Expected no match with empty options")
	}
	if got != "" {
		t.Errorf("Expected empty string, got %v", got)
	}
}
