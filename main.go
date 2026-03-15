package main

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

	// 2. Initialize Persistence (Chapter 1)
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

	// 4. Start Listeners
	// Loopback listener for the "Fortress"
	go func() {
		fmt.Println("Routes registered on pairMux: /pair, /pair/approve")

		// FIX: Remove 127.0.0.1 so the phone can hit this port via LAN IP
		pairAddr := fmt.Sprintf(":%d", *pairPort)

		fmt.Printf("Pairing Fortress active on %s (LAN access enabled for scans)\n", pairAddr)
		log.Fatal(http.ListenAndServe(pairAddr, pairMux))
	}()

	// 2. The Main UI (The "Ledger")
	uiAddr := fmt.Sprintf(":%d", *uiPort)
	fmt.Printf("Crystal Clear Checking active on %s\n", uiAddr)

	// Ensure you use the mux that has your JWT middleware!
	log.Fatal(http.ListenAndServe(uiAddr, mux))
}
