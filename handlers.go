package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Global instance of the store (initialized in main.go)
var store *Store

// DashboardHandler renders the main "Honest Truth" view.
func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	const accountName = "Joint Checking"

	// 1. Get the Big Number
	balance, err := store.GetHonestBalance(accountName)
	if err != nil {
		log.Printf("Balance error: %v", err)
		http.Error(w, "Failed to calculate balance", http.StatusInternalServerError)
		return
	}

	// 2. Get the "Abstract Art" (Uncleared Transactions)
	rows, err := store.db.Query(`
		SELECT date, check_number, description, amount 
		FROM transactions 
		WHERE account = ? AND cleared = 0 AND voided = 0
		ORDER BY date DESC`, accountName)
	if err != nil {
		http.Error(w, "Failed to load transactions", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type txn struct {
		Date    string
		CheckNo string
		Desc    string
		Amount  float64
	}
	var txns []txn

	for rows.Next() {
		var t txn
		var cents int64
		if err := rows.Scan(&t.Date, &t.CheckNo, &t.Desc, &cents); err != nil {
			continue
		}
		t.Amount = float64(cents) / 100.0
		txns = append(txns, t)
	}

	// 3. Package it for the Template
	data := struct {
		Account      string
		Balance      float64
		Transactions []txn
	}{
		Account:      accountName,
		Balance:      float64(balance) / 100.0,
		Transactions: txns,
	}

	templates.ExecuteTemplate(w, "dashboard.html", data)
}

func AddCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		if err := templates.ExecuteTemplate(w, "add-check.html", nil); err != nil {
			log.Printf("Template error: %v", err)
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}

	if r.Method == http.MethodPost {
		checkNum := r.FormValue("check_number")
		date := r.FormValue("date")
		desc := r.FormValue("description")
		amountStr := r.FormValue("amount") // e.g., "12.50"

		// --- Border Control: String-to-Int Manual Parsing ---
		var cents int64
		parts := strings.Split(amountStr, ".")

		// 1. Process Dollars
		dollars, _ := strconv.ParseInt(parts[0], 10, 64)
		cents = dollars * 100

		// 2. Process Fractions (Fixed-Point Parsing)
		if len(parts) > 1 {
			fraction := parts[1]
			// Pad or truncate to exactly 2 digits
			if len(fraction) == 1 {
				fraction += "0" // "12.5" -> "12.50"
			} else if len(fraction) > 2 {
				fraction = fraction[:2] // "12.555" -> "12.55"
			}
			pennies, _ := strconv.ParseInt(fraction, 10, 64)
			cents += pennies
		}

		// Ensure financial integrity: Check is always an outflow (negative)
		if cents > 0 {
			cents = -cents
		}

		// 3. Save to DB
		_, err := store.db.Exec(`
			INSERT INTO transactions (date, check_number, description, amount, type, account, source, cleared)
			VALUES (?, ?, ?, ?, 'Check', 'Joint Checking', 'manual', 0)`,
			date, checkNum, desc, cents)

		if err != nil {
			log.Printf("DB Error: %v", err)
			http.Error(w, "Failed to save transaction", http.StatusInternalServerError)
			return
		}

		// 4. Redirect
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// UploadHandler handles the OFX file drop.
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	file, _, err := r.FormFile("ofx_file")
	if err != nil {
		http.Error(w, "Failed to upload file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	count, err := store.IngestOFX(file)
	if err != nil {
		log.Printf("Ingest failed: %v", err)
		http.Error(w, fmt.Sprintf("Ingest failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Instead of fmt.Fprintf(w, "Success..."), use this:
	log.Printf("Successfully ingested %d transactions", count)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// PairApproveHandler turns a valid nonce into a 90-day login cookie.
func PairApproveHandler(w http.ResponseWriter, r *http.Request) {
	nonce := r.URL.Query().Get("nonce")

	pairingMutex.Lock()
	p, exists := pairingStore[nonce]
	if exists {
		delete(pairingStore, nonce) // One-time use!
	}
	pairingMutex.Unlock()

	if !exists || time.Now().After(p.ExpiresAt) {
		http.Error(w, "Invalid or expired pairing token", http.StatusForbidden)
		return
	}

	// Create the JWT for this device
	token, err := CreateToken("Stuart-Primary")
	if err != nil {
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	// Set the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "ccc_auth",
		Value:    token,
		Expires:  time.Now().Add(90 * 24 * time.Hour),
		HttpOnly: true,
		Path:     "/",
	})

	// Redirect from the Pairing Port (55888) to the UI Port (8080)
	// Using the same Hostname/IP the user used to get here
	host, _, _ := net.SplitHostPort(r.Host)
	dashboardURL := fmt.Sprintf("http://%s:8080/", host)
	http.Redirect(w, r, dashboardURL, http.StatusSeeOther)
}
