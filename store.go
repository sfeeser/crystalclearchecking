package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the database connection and the file path for maintenance tasks.
type Store struct {
	db     *sql.DB
	dbPath string
}

// NewStore initializes the database, enables WAL mode, and runs integrity checks.
func NewStore(dbPath string) (*Store, error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Store{db: db, dbPath: dbPath}

	// Chapter 1.1.2: Startup Integrity Verification
	if err := s.VerifyIntegrity(); err != nil {
		log.Fatalf("DATABASE CORRUPTED: %v. Binary exiting to prevent data loss.", err)
	}
	fmt.Println("Database integrity verified. [OK]")

	// Enable WAL mode for concurrency
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}

	// Chapter 1: Persistence - Create tables
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	return s, nil
}

// VerifyIntegrity runs the PRAGMA check. Returns error if result is not "ok".
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

// GetHonestBalance calculates: starting_balance + SUM(amount)
// Filtered by account, starting date, and avoiding voided transactions.
func (s *Store) GetHonestBalance(accountName string) (int64, error) {
	var startingBalance int64
	var startingDate string

	// 1. Get account starting truth
	err := s.db.QueryRow(`
		SELECT starting_balance, starting_date 
		FROM accounts WHERE name = ?`, accountName).Scan(&startingBalance, &startingDate)
	if err != nil {
		return 0, fmt.Errorf("could not find account: %w", err)
	}

	// 2. Sum all non-voided transactions since starting date
	var sum int64
	err = s.db.QueryRow(`
		SELECT COALESCE(SUM(amount), 0) 
		FROM transactions 
		WHERE account = ? AND date >= ? AND voided = 0`, 
		accountName, startingDate).Scan(&sum)
	
	if err != nil {
		return 0, err
	}

	return startingBalance + sum, nil
}

// HotBackup Chapter 1.1.1: Safely clones the DB while the server is running.
func (s *Store) HotBackup(destPath string) error {
	if destPath == "" {
		timestamp := time.Now().Format("20060102_150405")
		destPath = filepath.Join(filepath.Dir(s.dbPath), "backups", fmt.Sprintf("ccc_backup_%s.db", timestamp))
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	// Using VACUUM INTO is a modern, safe way to create a hot backup in SQLite
	// It handles the WAL/journal state and produces a clean, single-file database.
	_, err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s';", destPath))
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Chapter 1.1.1 Safety Check: Verify the new backup immediately
	backupDB, err := sql.Open("sqlite", destPath)
	if err != nil {
		return err
	}
	defer backupDB.Close()

	var result string
	if err := backupDB.QueryRow("PRAGMA integrity_check;").Scan(&result); err != nil || result != "ok" {
		return fmt.Errorf("backup integrity check failed: %s", result)
	}

	return nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(DBSchema) // DBSchema is the SQL string defined in schema.go
	return err
}
