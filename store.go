package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

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

// GetHonestBalance uses the NEW balance_anchors table
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

// HotBackup remains the same (Modern VACUUM INTO approach)
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

// getAccountByExtID finds the internal account name for a bank's external ID.
func (s *Store) getAccountByExtID(extID string) (string, error) {
	var name string
	err := s.db.QueryRow("SELECT name FROM accounts WHERE ext_id = ?", extID).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil
}

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

func (s *Store) VoidTransaction(id int64) error {
	_, err := s.db.Exec("UPDATE transactions SET voided = 1 WHERE id = ? AND cleared = 0", id)
	return err
}
