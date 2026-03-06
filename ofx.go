package main

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/aclindsa/ofxgo"
)

// IngestOFX parses an OFX reader and reconciles transactions into the store.
func (s *Store) IngestOFX(r io.Reader) (int, error) {
	parsed, err := ofxgo.ParseResponse(r)
	if err != nil {
		return 0, fmt.Errorf("failed to parse OFX: %w", err)
	}

	count := 0
	// OFX files can contain multiple statements (Checking, Savings, etc.)
	for _, snt := range parsed.Bank {
		statement, ok := snt.(*ofxgo.BankStatementResponse)
		if !ok {
			continue
		}

		// Chapter 2: Account Validation
		// Match the bank's AcctID to our accounts.ext_id
		accountName, err := s.getAccountByExtID(string(statement.BankAcctFrom.AcctID))
		if err != nil {
			// Skip unmatched accounts as per SpecBook
			continue
		}

		for _, tran := range statement.BankTranList.Transactions {
			if err := s.reconcileOFXTransaction(accountName, tran); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}

// reconcileOFXTransaction implements the deduplication and merge logic.
func (s *Store) reconcileOFXTransaction(accountName string, t ofxgo.Transaction) error {
	// Convert float-like ofxgo.Amount to integer cents
	// ofxgo.Amount is a Dec, we multiply by 100 and round to be safe.
	amountFloat, _ := t.Amount.Float64()
	cents := int64(math.Round(amountFloat * 100))

	fitid := string(t.FITID)
	date := t.DtPosted.Time.Format("2006-01-02")
	
	var checkNum *string
	if t.CheckNum != "" {
		s := string(t.CheckNum)
		checkNum = &s
	}

	// 1. Primary Deduplication: Check if FITID already exists
	// We use "INSERT ... ON CONFLICT DO NOTHING" to ensure idempotency.
	// If it exists, we don't touch it.
	
	// 2. The Merge: If it's a check, try to find a manual entry to "Clear"
	if checkNum != nil {
		var manualID int64
		err := s.db.QueryRow(`
			SELECT id FROM transactions 
			WHERE account = ? 
			AND check_number = ? 
			AND cleared = 0 
			AND voided = 0 
			LIMIT 1`, accountName, *checkNum).Scan(&manualID)

		if err == nil {
			// Match found! Update the manual entry with bank truth.
			_, err = s.db.Exec(`
				UPDATE transactions SET 
					bank_fitid = ?, 
					cleared = 1, 
					original_description = ?,
					imported_at = CURRENT_TIMESTAMP
				WHERE id = ?`, fitid, t.Name, manualID)
			return err
		}
	}

	// 3. Normal Insert: New bank transaction (or no manual check match)
	_, err := s.db.Exec(`
		INSERT INTO transactions (
			date, check_number, description, original_description, 
			amount, type, account, bank_fitid, source, cleared
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'ofx', 1)
		ON CONFLICT(bank_fitid) DO NOTHING`,
		date, checkNum, t.Name, t.Name, cents, t.Type.String(), accountName, fitid)

	return err
}

func (s *Store) getAccountByExtID(extID string) (string, error) {
	var name string
	err := s.db.QueryRow("SELECT name FROM accounts WHERE ext_id = ?", extID).Scan(&name)
	return name, err
}
