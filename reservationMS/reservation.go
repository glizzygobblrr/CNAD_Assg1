package reservationMS

import (
	"encoding/json"
	"bytes"
	"fmt"
	"log"
	"net/http"
	"time"
	"database/sql"

	"github.com/gorilla/mux"
	_ "github.com/go-sql-driver/mysql"
)

type Reservation struct {
	ID        int       `json:"id,omitempty"`
	UserID    int       `json:"user_id"`
	VehicleID int       `json:"vehicle_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

type Vehicle struct {
	ID          int    `json:"id"`
	Model       string `json:"model"`
	IsAvailable string `json:"is_available"`
}

var db *sql.DB

// Initialize DB connection
func init() {
	var err error
	db, err = sql.Open("mysql", "user:password@tcp(127.0.0.1:3306)/car_sharing_system")
	if err != nil {
		log.Fatalf("Error opening database connection: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Error pinging database: %v", err)
	}
}

// Get available vehicles
func GetAvailableVehicles(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	rows, err := db.Query("SELECT id, model FROM vehicles WHERE is_available = 'available'") // Use 'available' string
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to fetch vehicles: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var vehicles []Vehicle
	for rows.Next() {
		var vehicle Vehicle
		if err := rows.Scan(&vehicle.ID, &vehicle.Model); err != nil {
			http.Error(w, fmt.Sprintf("Error reading data: %v", err), http.StatusInternalServerError)
			return
		}
		vehicles = append(vehicles, vehicle)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(vehicles)
}

// Create a reservation
const paymentMSURL = "http://localhost:5000/payments"

// Create a reservation and notify PaymentMS
func CreateReservation(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var reservation Reservation
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reservation); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Check if vehicle is available
	var isAvailable string
	err := db.QueryRow("SELECT is_available FROM vehicles WHERE id = ?", reservation.VehicleID).Scan(&isAvailable)
	if err != nil || isAvailable != "available" {
		http.Error(w, "Vehicle not available", http.StatusBadRequest)
		return
	}

	// Create the reservation
	result, err := db.Exec("INSERT INTO reservations (user_id, vehicle_id, start_time, end_time) VALUES (?, ?, ?, ?)",
		reservation.UserID, reservation.VehicleID, reservation.StartTime, reservation.EndTime)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to create reservation: %v", err), http.StatusInternalServerError)
		return
	}

	// Retrieve the reservation ID
	reservationID, err := result.LastInsertId()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to retrieve reservation ID: %v", err), http.StatusInternalServerError)
		return
	}
	reservation.ID = int(reservationID)

	// Update vehicle availability
	_, err = db.Exec("UPDATE vehicles SET is_available = 'unavailable' WHERE id = ?", reservation.VehicleID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to update vehicle availability: %v", err), http.StatusInternalServerError)
		return
	}

	// Notify PaymentMS
	go notifyPaymentMS(reservation) // Non-blocking call to avoid delaying response

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode("Reservation created successfully")
}

// Notify PaymentMS of the new reservation
func notifyPaymentMS(reservation Reservation) {
	// Create payment request payload
	paymentRequest := map[string]interface{}{
		"reservation_id": reservation.ID,
		"user_id":        reservation.UserID,
		"vehicle_id":     reservation.VehicleID,
		"start_time":     reservation.StartTime,
		"end_time":       reservation.EndTime,
	}

	jsonData, err := json.Marshal(paymentRequest)
	if err != nil {
		log.Printf("Error marshalling payment request: %v", err)
		return
	}

	// Make HTTP POST request to PaymentMS
	resp, err := http.Post(fmt.Sprintf("%s/process", paymentMSURL), "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		log.Printf("Error sending payment notification to PaymentMS: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("PaymentMS responded with status code %d", resp.StatusCode)
	}
}


// Modify a reservation
func ModifyReservation(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	var reservation Reservation
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&reservation); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Modify the reservation
	_, err := db.Exec("UPDATE reservations SET start_time = ?, end_time = ? WHERE id = ?",
		reservation.StartTime, reservation.EndTime, reservation.ID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to modify reservation: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode("Reservation updated successfully")
}

// Cancel a reservation
func CancelReservation(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	params := mux.Vars(r)
	reservationID := params["id"]

	// Ensure the reservation ID is valid
	if reservationID == "" {
		http.Error(w, "Reservation ID is required", http.StatusBadRequest)
		return
	}

	// Free the vehicle associated with the reservation
	_, err := db.Exec("UPDATE vehicles v JOIN reservations r ON v.id = r.vehicle_id SET v.is_available = 'available' WHERE r.id = ?",
		reservationID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to cancel reservation: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete the reservation
	_, err = db.Exec("DELETE FROM reservations WHERE id = ?", reservationID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Unable to cancel reservation: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode("Reservation canceled successfully")
}
