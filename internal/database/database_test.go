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

	gyroskop, err := db.CreateGyroskop(chatID, createdBy, deadline)
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

	gyroskop, err := db.CreateGyroskop(chatID, createdBy, deadline)
	if err != nil {
		t.Fatalf("Error creating gyroskop: %v", err)
	}

	// Add order
	userID := int64(11111)
	username := "testuser"
	firstName := "Test"
	lastName := "User"
	quantityMeat := 3
	quantityVeggie := 1

	err = db.AddOrUpdateOrder(gyroskop.ID, userID, username, firstName, lastName, quantityMeat, quantityVeggie)
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

	if order.QuantityMeat != quantityMeat {
		t.Errorf("Expected QuantityMeat %d, got: %d", quantityMeat, order.QuantityMeat)
	}

	if order.QuantityVeggie != quantityVeggie {
		t.Errorf("Expected QuantityVeggie %d, got: %d", quantityVeggie, order.QuantityVeggie)
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

	gyroskop, err := db.CreateGyroskop(chatID, createdBy, deadline)
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

	_, err := db.CreateGyroskop(chatID1, createdBy, deadline)
	if err != nil {
		t.Fatalf("Fehler beim Erstellen des ersten Gyroskops: %v", err)
	}

	_, err = db.CreateGyroskop(chatID2, createdBy, deadline)
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