package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	// 1. CLI Flags
	dbPath := flag.String("db", filepath.Join(os.Getenv("HOME"), ".crystalclearchecking", "ledger.db"), "Path to SQLite database")
	uiPort := flag.Int("port", 8080, "Port for the main web interface")
	pairPort := flag.Int("pair-port", 55888, "Port for local SSH pairing (loopback only)")
	
	// Subcommand logic for Chapter 1.1: Backup
	if len(os.Args) > 1 && os.Args[1] == "backup" {
		backupCmd := flag.NewFlagSet("backup", flag.ExitOnError)
		dest := backupCmd.String("path", "", "Destination path for backup")
		backupCmd.Parse(os.Args[2:])
		
		s, err := NewStore(*dbPath)
		if err != nil {
			log.Fatalf("Failed to open DB for backup: %v", err)
		}
		if err := s.HotBackup(*dest); err != nil {
			log.Fatalf("Backup failed: %v", err)
		}
		fmt.Println("Safe Hot Backup completed successfully. [OK]")
		return
	}

	flag.Parse()

	// 2. Initialize Persistence (Chapter 1)
	// This triggers the integrity check and WAL mode automatically.
	var err error
	store, err = NewStore(*dbPath)
	if err != nil {
		log.Fatalf("Could not initialize store: %v", err)
	}

	// 3. Define Routes (Chapter 5)
	// Operational Routes (Protected by JWT)
	mux := http.NewServeMux()
	mux.Handle("/", ValidateJWT(http.HandlerFunc(DashboardHandler)))
	mux.Handle("/add-check", ValidateJWT(http.HandlerFunc(AddCheckHandler)))
	mux.Handle("/upload", ValidateJWT(http.HandlerFunc(UploadHandler)))

	// Public/Pairing Routes
	pairMux := http.NewServeMux()
	pairMux.Handle("/pair", RequireLoopback(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, _ := GeneratePairingNonce()
		fmt.Fprintf(w, "<h1>Pairing Initiated</h1><p>Scan this nonce on your phone: <strong>%s</strong></p>", nonce)
		// In the full version, this renders the QR code from templates.go
	})))

	// 4. Start Listeners
	// Loopback listener for the "Fortress"
	go func() {
		pairAddr := fmt.Sprintf("127.0.0.1:%d", *pairPort)
		fmt.Printf("Pairing Fortress active on %s (SSH tunnel required)\n", pairAddr)
		log.Fatal(http.ListenAndServe(pairAddr, pairMux))
	}()

	// Main UI listener for the LAN
	uiAddr := fmt.Sprintf(":%d", *uiPort)
	fmt.Printf("Crystal Clear Checking active on %s\n", uiAddr)
	log.Fatal(http.ListenAndServe(uiAddr, mux))
}
