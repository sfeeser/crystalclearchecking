package main

// CHUNK_START: imports-and-package-v1-uuid-1d4e7b9a
// BUSINESS_PURPOSE: Declares the package and lists all required imports for the application. This is the single source of truth for dependencies to prevent missing/extra imports during refactors or feature additions. Keep minimal and aligned with used stdlib + external packages (qrcode only for pairing QR).
// SPEC_LINK: specbook-chapter-0 (core intent) + non-negotiables on minimal dependencies
import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/skip2/go-qrcode"
)
// CHUNK_END: imports-and-package-v1-uuid-1d4e7b9a

// CHUNK_START: cli-flags-and-subcommands-v1-uuid-8f3a2b1c
// BUSINESS_PURPOSE: Parses command-line flags and handles subcommands like 'backup' per specbook Chapter 1.1 (Data Durability & Hot Backup)
// SPEC_LINK: specbook-chapter-1.1
func main() {
	// 1. CLI Flags
	dbPath := flag.String("db", filepath.Join(".", "data", "ledger.db"), "Path to SQLite database")
	uiPort := flag.Int("port", 8080, "Port for the main web interface")
	pairPort := flag.Int("pair-port", 55888, "Port for local SSH pairing (loopback only)")

	flag.Parse()

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
}
// CHUNK_END: cli-flags-and-subcommands-v1-uuid-8f3a2b1c

// CHUNK_START: persistence-init-v1-uuid-4e9d7f2a
// BUSINESS_PURPOSE: Initializes the SQLite store/persistence layer at startup per specbook Chapter 1 (Data Model & Persistence)
// SPEC_LINK: specbook-chapter-1
var store *Store // global for simplicity in this POC; consider passing in production

func main() { // continued
	// 2. Initialize Persistence (Chapter 1)
	var err error
	store, err = NewStore(*dbPath)
	if err != nil {
		log.Fatalf("Could not initialize store: %v", err)
	}
}
// CHUNK_END: persistence-init-v1-uuid-4e9d7f2a

// CHUNK_START: main-router-setup-v1-uuid-c6b8e3f9
// BUSINESS_PURPOSE: Sets up the protected HTTP multiplexer for core application routes (dashboard, entry, upload, etc.) per specbook Chapter 5 (UI & Operational Flows)
// SPEC_LINK: specbook-chapter-5
func main() { // continued
	// 3. Define Routes (Chapter 5)
	// Operational Routes (Protected by JWT)
	mux := http.NewServeMux()
	mux.Handle("/", ValidateJWT(http.HandlerFunc(DashboardHandler)))
	mux.Handle("/add-check", ValidateJWT(http.HandlerFunc(AddCheckHandler)))
	mux.Handle("/upload", ValidateJWT(http.HandlerFunc(UploadHandler)))
	mux.Handle("/reconcile", ValidateJWT(http.HandlerFunc(ReconcileHandler)))
	mux.Handle("/void", ValidateJWT(http.HandlerFunc(VoidHandler)))
}
// CHUNK_END: main-router-setup-v1-uuid-c6b8e3f9

// CHUNK_START: pairing-router-setup-v1-uuid-2a1d9e5b
// BUSINESS_PURPOSE: Sets up the public pairing multiplexer (non-JWT) for device proximity authorization per specbook Chapter 4 (Pairing & Security)
// SPEC_LINK: specbook-chapter-4
func main() { // continued
	// Public/Pairing Routes (Chapter 4)
	pairMux := http.NewServeMux()

	pairMux.Handle("/pair/approve", (http.HandlerFunc(PairApproveHandler)))

	pairMux.Handle("/pair", RequireLoopback(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, _ := GeneratePairingNonce()

		// 1. Ask the kernel for the current local IP (Zero Hard-coding)
		conn, err := net.Dial("udp", "1.1.1.1:80")
		if err != nil {
			log.Printf("Could not determine local IP: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer conn.Close()
		localIP := conn.LocalAddr().(*net.UDPAddr).IP.String()

		// 2. Build the Approval URL (Works for both Laptop and Phone)
		approvalURL := fmt.Sprintf("http://%s:%d/pair/approve?nonce=%s", localIP, *pairPort, nonce)

		// 3. Generate QR Code in the Terminal for the Phone
		// This requires: go get github.com/skip2/go-qrcode
		qr, err := qrcode.New(approvalURL, qrcode.Medium)
		if err == nil {
			fmt.Println("\n--- CRYSTAL CLEAR CHECKING: PHONE PAIRING ---")
			fmt.Println(qr.ToSmallString(false))
			fmt.Println("URL:", approvalURL)
			fmt.Println("----------------------------------------------")
		} else {
			log.Printf("QR Generation failed: %v", err)
		}

		// 4. Send the "Pairing Fortress" HTML to the Laptop Browser
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `
            <!DOCTYPE html>
            <html>
            <head><title>Pairing Fortress</title></head>
            <body style="font-family: sans-serif; text-align: center; padding-top: 50px;">
                <h1>Pairing Fortress</h1>
                <p>A new pairing token has been generated.</p>
                <p><strong>Nonce:</strong> %s</p>
                <p><a href="%s" style="display: inline-block; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 5px;">Authorize THIS Device</a></p>
                <hr style="width: 50%%; margin: 30px auto;">
                <p>To authorize your <strong>Phone</strong>, scan the QR code displayed in your laptop's terminal.</p>
                <p><small>This link expires in 10 minutes.</small></p>
            </body>
            </html>
        `, nonce, approvalURL)
	})))
}
// CHUNK_END: pairing-router-setup-v1-uuid-2a1d9e5b

// CHUNK_START: pairing-server-start-v1-uuid-7f4c1a8d
// BUSINESS_PURPOSE: Starts the background pairing listener on a dedicated port (LAN-enabled for phone scanning) per specbook Chapter 4 (proximity pairing)
// SPEC_LINK: specbook-chapter-4
func main() { // continued
	// 4. Start Listeners
	// Loopback listener for the "Fortress"
	go func() {
		fmt.Println("Routes registered on pairMux: /pair, /pair/approve")

		// FIX: Remove 127.0.0.1 so the phone can hit this port via LAN IP
		pairAddr := fmt.Sprintf(":%d", *pairPort)

		fmt.Printf("Pairing Fortress active on %s (LAN access enabled for scans)\n", pairAddr)
		log.Fatal(http.ListenAndServe(pairAddr, pairMux))
	}()
}
// CHUNK_END: pairing-server-start-v1-uuid-7f4c1a8d

// CHUNK_START: main-ui-server-start-v1-uuid-9e2b5f6d
// BUSINESS_PURPOSE: Starts the primary web server for the ledger UI on the configured port per specbook Chapter 5 (main application access)
// SPEC_LINK: specbook-chapter-5
func main() { // continued
	// 2. The Main UI (The "Ledger")
	uiAddr := fmt.Sprintf(":%d", *uiPort)
	fmt.Printf("Crystal Clear Checking active on %s\n", uiAddr)

	// Ensure you use the mux that has your JWT middleware!
	log.Fatal(http.ListenAndServe(uiAddr, mux))
}
// CHUNK_END: main-ui-server-start-v1-uuid-9e2b5f6d
