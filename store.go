package main

// CHUNK_START: imports-and-package-v1-uuid-p5q9r2s8
// BUSINESS_PURPOSE: Declares the package and lists all required imports for the data access layer (Store). Single source of truth for database, file path, and time handling dependencies. Keep minimal; add new imports here during data-layer refactors.
// SPEC_LINK: specbook-chapter-1 (Data Model & Persistence) + non-negotiables on minimal dependencies
import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)
// CHUNK_END: imports-and-package-v1-uuid-p5q9r2s8

// CHUNK_START: store-struct-and-newstore-v1-uuid-t3u7v4w1
// BUSINESS_PURPOSE: Defines the Store struct (db connection holder) and initializes the SQLite connection, creates directories, enables WAL mode, runs migrations, sets schema version, and verifies integrity per specbook Chapter 1 persistence requirements
// SPEC_LINK: specbook-chapter-1 + chapter-1.1
type Store struct {
	db     *sql.DB
	dbPath string
}

func NewStore(dbPath string) (*Store, error) {
	fmt.Println("--- DEBUG: NEWSTORE START ---")

	// 1. Plumbing
	absPath, _ := filepath.Abs(dbPath)
	fmt.Printf("DEBUG: Target DB Path: %s\n", absPath)

	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", absPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db, dbPath: absPath}

	// 2. WAL Mode
	fmt.Println("DEBUG: Enabling WAL Mode...")
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}

	// 3. MIGRATION
	fmt.Println("DEBUG: Starting Migration...")
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	// Force a disk write by setting a version number
	fmt.Println("DEBUG: Recording schema version...")
	if _, err := db.Exec("PRAGMA user_version = 1;"); err != nil {
		return nil, err
	}

	// 4. INTEGRITY CHECK
	fmt.Println("DEBUG: Running Integrity Check...")
	if err := s.VerifyIntegrity(); err != nil {
		return nil, fmt.Errorf("integrity check failed: %w", err)
	}

	fmt.Println("--- DEBUG: NEWSTORE SUCCESS (DB SHOULD BE > 0 BYTES) ---")
	return s, nil
}
// CHUNK_END: store-struct-and-newstore-v1-uuid-t3u7v4w1

// CHUNK_START: migrate-method-v1-uuid-v8w2x5y9
// BUSINESS_PURPOSE: Executes initial schema creation for transactions, balance_anchors, and accounts tables if they do not exist per specbook Chapter 1 data model
// SPEC_LINK: specbook-chapter-1
func (s *Store) migrate() error {
	// 1. Transactions Table
	_, err := s.db.Exec(`
        CREATE TABLE IF NOT EXISTS transactions (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            date TEXT NOT NULL,
            check_number TEXT,
            description TEXT NOT NULL,
            amount INTEGER NOT NULL,
            type TEXT NOT NULL,
            account TEXT NOT NULL,
            source TEXT DEFAULT 'manual',
            cleared INTEGER DEFAULT 0,
			voided INTEGER DEFAULT 0,
            fitid TEXT UNIQUE,
			original_description TEXT
        );
    `)
	if err != nil {
		return err
	}

	// 2. Balance Anchors Table (The Temporal Guardrail)
	_, err = s.db.Exec(`
        CREATE TABLE IF NOT EXISTS balance_anchors (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            anchor_date TEXT NOT NULL,
            amount INTEGER NOT NULL,
            account TEXT NOT NULL
        );
    `)
	if err != nil {
		return err
	}

	// 3. Account Mappings Table
	_, err = s.db.Exec(`
        CREATE TABLE IF NOT EXISTS accounts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL UNIQUE,
            ext_id TEXT NOT NULL UNIQUE
        );
    `)
	return err
}
// CHUNK_END: migrate-method-v1-uuid-v8w2x5y9

// CHUNK_START: verify-integrity-v1-uuid-x1y6z3a7
// BUSINESS_PURPOSE: Runs SQLite PRAGMA integrity_check to verify database consistency after initialization or backup per specbook Chapter 1.1 durability non-negotiables
// SPEC_LINK: specbook-chapter-1.1
func (s *Store) VerifyIntegrity() error {
	var result string
	err := s.db.QueryRow("PRAGMA integrity_check;").Scan(&result)
	if err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}
	return nil
}
// CHUNK_END: verify-integrity-v1-uuid-x1y6z3a7

// CHUNK_START: get-honest-balance-v1-uuid-z4a9b2c8
// BUSINESS_PURPOSE: Calculates the "honest" current balance by applying post-anchor transactions to the latest balance anchor, handling missing anchors gracefully per specbook Chapter 1 data model
// SPEC_LINK: specbook-chapter-1
func (s *Store) GetHonestBalance(accountName string) (int64, error) {
	var anchorAmount int64
	var anchorDate string

	// 1. Get the latest anchor for this account
	err := s.db.QueryRow(`
        SELECT amount, anchor_date 
        FROM balance_anchors 
        WHERE account = ? 
        ORDER BY anchor_date DESC LIMIT 1`, accountName).Scan(&anchorAmount, &anchorDate)

	// If no anchor exists yet, we assume 0
	if err == sql.ErrNoRows {
		anchorAmount = 0
		anchorDate = "1970-01-01"
	} else if err != nil {
		return 0, err
	}

	// 2. Sum all transactions occurring AFTER the anchor date
	var sum int64
	err = s.db.QueryRow(`
        SELECT COALESCE(SUM(amount), 0) 
        FROM transactions 
        WHERE account = ? AND date >= ? AND cleared = 0 AND voided = 0`,
		accountName, anchorDate).Scan(&sum)

	if err != nil {
		return 0, err
	}

	return anchorAmount + sum, nil
}
// CHUNK_END: get-honest-balance-v1-uuid-z4a9b2c8

// CHUNK_START: hot-backup-v1-uuid-b7c3d6e1
// BUSINESS_PURPOSE: Performs a hot backup using VACUUM INTO to create a consistent snapshot without locking the live database per specbook Chapter 1.1 durability & hot backup requirements
// SPEC_LINK: specbook-chapter-1.1
func (s *Store) HotBackup(destPath string) error {
	if destPath == "" {
		timestamp := time.Now().Format("20060102_150405")
		destPath = filepath.Join(filepath.Dir(s.dbPath), "backups", fmt.Sprintf("ccc_backup_%s.db", timestamp))
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}
	_, err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s';", destPath))
	return err
}
// CHUNK_END: hot-backup-v1-uuid-b7c3d6e1

// CHUNK_START: get-account-by-extid-v1-uuid-d9e4f2g5
// BUSINESS_PURPOSE: Maps a bank's external account ID (from OFX) to the internal account name for reconciliation per specbook Chapter 2 ingestion flow
// SPEC_LINK: specbook-chapter-2
func (s *Store) getAccountByExtID(extID string) (string, error) {
	var name string
	err := s.db.QueryRow("SELECT name FROM accounts WHERE ext_id = ?", extID).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}
// CHUNK_END: get-account-by-extid-v1-uuid-d9e4f2g5

// CHUNK_START: reconcile-transaction-v1-uuid-f2g8h4j9
// BUSINESS_PURPOSE: Reconciles a manual entry with a bank transaction by deleting the bank record and updating the manual one with bank's fitid/description/amount in a transaction per specbook Chapter 2 reconciliation rules
// SPEC_LINK: specbook-chapter-2
func (s *Store) Reconcile(manualID, bankID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Capture the bank's truth metadata
	var fitid, bankDesc string
	var bankCents int64
	err = tx.QueryRow("SELECT fitid, description, amount FROM transactions WHERE id = ?", bankID).
		Scan(&fitid, &bankDesc, &bankCents)
	if err != nil {
		return err
	}

	// 2. DELETE the bank record FIRST
	// This frees up the 'fitid' so it can be reused by the manual entry
	_, err = tx.Exec("DELETE FROM transactions WHERE id = ?", bankID)
	if err != nil {
		return err
	}

	// 3. NOW update Stuart's record with the bank's data
	_, err = tx.Exec(`
        UPDATE transactions 
        SET amount = ?, cleared = 1, fitid = ?, original_description = ? 
        WHERE id = ?`, bankCents, fitid, bankDesc, manualID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
// CHUNK_END: reconcile-transaction-v1-uuid-f2g8h4j9

// CHUNK_START: void-transaction-v1-uuid-h5j1k7m3
// BUSINESS_PURPOSE: Marks a transaction as voided (only if not already cleared) per specbook Chapter 5 transaction management
// SPEC_LINK: specbook-chapter-5
func (s *Store) VoidTransaction(id int64) error {
	_, err := s.db.Exec("UPDATE transactions SET voided = 1 WHERE id = ? AND cleared = 0", id)
	return err
}
// CHUNK_END: void-transaction-v1-uuid-h5j1k7m3
