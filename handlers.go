package main

// CHUNK_START: imports-and-package-v1-uuid-h2k9m3p7
// BUSINESS_PURPOSE: Declares the package and lists all required imports for HTTP handlers. This is the single source of truth for dependencies used in request handling, response formatting, and time/date logic. Keep minimal; add new imports here first during refactors or feature additions.
// SPEC_LINK: specbook-chapter-5 (UI & Operational Flows) + non-negotiables on minimal dependencies
import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)
// CHUNK_END: imports-and-package-v1-uuid-h2k9m3p7

// Global instance of the store (initialized in main.go)
var store *Store

// CHUNK_START: dashboard-handler-v1-uuid-a3c7d2f8
// BUSINESS_PURPOSE: Renders the main dashboard ("Honest Truth" view) with honest balance, paginated cleared history, pending user entries, and unmatched bank records per specbook Chapter 5 (UI & Operational Flows)
// SPEC_LINK: specbook-chapter-5
func DashboardHandler(w http.ResponseWriter, r *http.Request) {
	const accountName = "Joint Checking" // In the future, this comes from a session/user
	const pageSize = 20

	// 1. Get Page Number from URL (e.g., /?page=1)
	pageStr := r.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 0 {
		page = 0
	}
	offset := page * pageSize

	// 2. Calculate the "Honest Balance" (Always needed for the header)
	balanceCents, err := store.GetHonestBalance(accountName)
	if err != nil {
		log.Printf("Balance error: %v", err)
		http.Error(w, "Failed to calculate balance", http.StatusInternalServerError)
		return
	}
	balanceDisplay := float64(balanceCents) / 100.0

	type txn struct {
		ID      int64   `json:"id"`
		Date    string  `json:"date"`
		CheckNo string  `json:"check_no"`
		Desc    string  `json:"desc"`
		Amount  float64 `json:"amount"`
		Source  string  `json:"source"`
	}

	// Helper to fetch data
	fetch := func(query string, args ...interface{}) []txn {
		rows, err := store.db.Query(query, args...)
		if err != nil {
			log.Printf("Query error: %v", err)
			return nil
		}
		defer rows.Close()
		var list []txn
		for rows.Next() {
			var t txn
			var cents int64
			// Note: History query and Matchmaker query must select columns in this exact order
			if err := rows.Scan(&t.ID, &t.Date, &t.CheckNo, &t.Desc, &cents, &t.Source); err != nil {
				log.Printf("Scan error: %v", err)
				continue
			}
			t.Amount = float64(cents) / 100.0
			list = append(list, t)
		}
		return list
	}

	// 3. Fetch History (with Pagination)
	history := fetch(`
        SELECT id, date, check_number, description, amount, source
        FROM transactions
        WHERE account = ? AND cleared = 1
        ORDER BY date DESC, id DESC
        LIMIT ? OFFSET ?`, accountName, pageSize, offset)

	// 4. Handle AJAX/Lazy-Loading Request
	// If the client asks for JSON, we stop here and just send the history slice
	if r.URL.Query().Get("format") == "json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(history)
		return
	}

	// 5. Fetch Matchmaker Data (Only needed for the initial full page load)
	userEntries := fetch(`
        SELECT id, date, check_number, description, amount, source
        FROM transactions
        WHERE account = ? AND source = 'manual' AND cleared = 0 AND voided = 0
        ORDER BY date ASC`, accountName)
	bankRecords := fetch(`
        SELECT id, date, check_number, description, amount, source
        FROM transactions
        WHERE account = ? AND source = 'ofx' AND cleared = 0
        ORDER BY date ASC`, accountName)

	// 6. Render Full HTML Page
	data := struct {
		Balance     float64
		Account     string
		UserEntries []txn
		BankRecords []txn
		History     []txn
	}{
		Balance:     balanceDisplay,
		Account:     accountName,
		UserEntries: userEntries,
		BankRecords: bankRecords,
		History:     history,
	}
	templates.ExecuteTemplate(w, "dashboard.html", data)
}
// CHUNK_END: dashboard-handler-v1-uuid-a3c7d2f8

// CHUNK_START: add-check-handler-v1-uuid-b8e4f1c9
// BUSINESS_PURPOSE: Handles GET (show form) and POST (manual paper check entry) with strict financial parsing (cents conversion, always negative outflow) per specbook Chapter 5 (transaction entry flow)
// SPEC_LINK: specbook-chapter-5
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
// CHUNK_END: add-check-handler-v1-uuid-b8e4f1c9

// CHUNK_START: upload-ofx-handler-v1-uuid-c1a9e6b2
// BUSINESS_PURPOSE: Handles GET (upload form) and POST (OFX file ingestion & reconciliation trigger) per specbook Chapter 2 (File Ingestion & Reconciliation)
// SPEC_LINK: specbook-chapter-2
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	// 1. If it's a GET, show the upload page
	if r.Method == http.MethodGet {
		if err := templates.ExecuteTemplate(w, "upload.html", nil); err != nil {
			log.Printf("Template error: %v", err)
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}

	// 2. If it's a POST, process the file
	if r.Method == http.MethodPost {
		file, _, err := r.FormFile("ofx_file")
		if err != nil {
			http.Error(w, "Failed to upload file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Use the store instance to ingest
		count, err := store.IngestOFX(file)
		if err != nil {
			log.Printf("Ingest failed: %v", err)
			// Show the user the error so they know if the OFX was "crusty"
			http.Error(w, fmt.Sprintf("Ingest failed: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("Successfully ingested %d transactions", count)

		// Redirect home to see the new balance and transactions
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}
// CHUNK_END: upload-ofx-handler-v1-uuid-c1a9e6b2

// CHUNK_START: pair-approve-handler-v1-uuid-d5f2c7a4
// BUSINESS_PURPOSE: Consumes a valid pairing nonce (one-time use) and issues a 90-day JWT cookie, redirecting to the main UI per specbook Chapter 4 (pairing completion)
// SPEC_LINK: specbook-chapter-4
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
// CHUNK_END: pair-approve-handler-v1-uuid-d5f2c7a4

// CHUNK_START: reconcile-handler-v1-uuid-e9b3d8f1
// BUSINESS_PURPOSE: Handles POST requests to reconcile a manual user entry with a bank/ofx record per specbook Chapter 2 (reconciliation logic)
// SPEC_LINK: specbook-chapter-2
func ReconcileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ManualID int64 `json:"manual_id"`
		BankID   int64 `json:"bank_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Call the database logic to merge them
	err := store.Reconcile(req.ManualID, req.BankID)
	if err != nil {
		log.Printf("Reconciliation failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
// CHUNK_END: reconcile-handler-v1-uuid-e9b3d8f1

// CHUNK_START: void-handler-v1-uuid-f2c4e9a6
// BUSINESS_PURPOSE: Handles POST to void/mark a transaction as voided per specbook Chapter 5 (transaction management)
// SPEC_LINK: specbook-chapter-5
func VoidHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	// Update the DB to mark it voided
	err := store.VoidTransaction(id)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
// CHUNK_END: void-handler-v1-uuid-f2c4e9a6
