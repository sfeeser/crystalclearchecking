package main

import (
	"fmt"
	"net/http"
	"strconv"
)

// Global instance of the store (initialized in main.go)
var store *Store

// DashboardHandler renders the main "Honest Truth" view.
func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	// For MVP, we assume a single account named "Joint Checking"
	// In a full build, this would loop through all accounts in store.
	balance, err := store.GetHonestBalance("Joint Checking")
	if err != nil {
		http.Error(w, "Failed to calculate balance", http.StatusInternalServerError)
		return
	}

	data := struct {
		Balance float64
		Account string
	}{
		Balance: float64(balance) / 100.0,
		Account: "Joint Checking",
	}

	RenderTemplate(w, "dashboard", data)
}

// AddCheckHandler processes the manual "12-second" check entry.
func AddCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		RenderTemplate(w, "add-check", nil)
		return
	}

	// Parse Form
	checkNum := r.FormValue("check_number")
	date := r.FormValue("date") // YYYY-MM-DD from HTML5 date picker
	amountStr := r.FormValue("amount")
	desc := r.FormValue("description")

	// Convert dollars to integer cents
	amountFloat, _ := strconv.ParseFloat(amountStr, 64)
	cents := int64(amountFloat * 100)
	if cents > 0 {
		cents = -cents // Checks are always debits (negative)
	}

	_, err := store.db.Exec(`
		INSERT INTO transactions (date, check_number, description, amount, type, account, source, cleared)
		VALUES (?, ?, ?, ?, 'Check', 'Joint Checking', 'manual', 0)`,
		date, checkNum, desc, cents)

	if err != nil {
		http.Error(w, "Entry failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// UploadHandler handles the OFX file drop.
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("ofx_file")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	count, err := store.IngestOFX(file)
	if err != nil {
		http.Error(w, "Ingest failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "Successfully imported %d transactions!", count)
}
