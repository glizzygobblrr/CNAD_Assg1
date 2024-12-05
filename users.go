package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"

)

// structs
type User struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Phone    string `json:"phone,omitempty"`
	Membership  string `json:"membership_level"`
}

type Reservation struct {
	VehicleID int    `json:"vehicle_id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

type Vehicle struct {
	ID    int    `json:"id"`
	Model string `json:"model"`
}

// validate whether membership level
func ValidateMembership(membership string) bool {
	normalizedMembership := strings.ToLower(membership)
	return normalizedMembership == "basic" || normalizedMembership == "premium"
}

// hash password
func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hashedPassword), err
}

// register a new user with hashed password
func registerUser(w http.ResponseWriter, r *http.Request) {
	var user User
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// validate membership level
	if !ValidateMembership(user.Membership) {
		http.Error(w, "Invalid membership level. It must be 'Basic' or 'Premium'.", http.StatusBadRequest)
		return
	}

	// hash the registered password
	hashedPassword, err := HashPassword(user.Password)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}
	user.Password = hashedPassword

	//store in db
	db, err := sql.Open("mysql", "user:password@tcp(127.0.0.1:3306)/car_sharing_system")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	query := `INSERT INTO users (email, password, phone, membership_level) VALUES (?, ?, ?, ?)`
	_, err = db.Exec(query, user.Email, user.Password, user.Phone, user.Membership)
	if err != nil {
		http.Error(w, "Unable to register user", http.StatusInternalServerError)
		return
	}

	fmt.Printf("User %s registered successfully\n", user.Email)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode("User successfully registered")
}

// validate whether password matches the hashed password
func CheckPassword(hashedPassword, plainPassword string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(plainPassword))
}

// validate login User with password
func loginUser(w http.ResponseWriter, r *http.Request) {
	var user User
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&user); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	db, err := sql.Open("mysql", "user:password@tcp(127.0.0.1:3306)/car_sharing_system")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		log.Fatal(err)
		return
	}
	defer db.Close()

	// retrieve the hashed password from db
	var hashedPassword string
	query := `SELECT password FROM users WHERE email = ?`
	err = db.QueryRow(query, user.Email).Scan(&hashedPassword)
	if err != nil {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	// compare hashed password with input password
	err = CheckPassword(hashedPassword, user.Password)
	if err != nil {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	fmt.Printf("User %s logged in successfully\n", user.Email)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode("User successfully logged in")
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/register", registerUser).Methods("POST")
	router.HandleFunc("/login", loginUser).Methods("POST")

	fmt.Println("Server running on http://localhost:5000")
	log.Fatal(http.ListenAndServe(":5000", router))
}
