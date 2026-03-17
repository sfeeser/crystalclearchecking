package main

// CHUNK_START: imports-and-package-v1-uuid-g4j8n5q2
// BUSINESS_PURPOSE: Declares the package and lists all required imports for OFX ingestion, parsing, and reconciliation. Single source of truth for dependencies related to file parsing, database interaction, and amount handling. Keep minimal; add new imports here during OFX-related refactors.
// SPEC_LINK: specbook-chapter-2 (File Ingestion & Reconciliation) + non-negotiables on minimal dependencies
import (
	"database/sql"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/aclindsa/ofxgo"
)
// CHUNK_END: imports-and-package-v1-uuid-g4j8n5q2

// CHUNK_START: ingest-ofx-entrypoint-v1-uuid-k7m9p3r6
// BUSINESS_PURPOSE: Main entry point for ingesting an OFX file: parses the response, sets the balance anchor from the bank's ledger balance, and processes each transaction for reconciliation per specbook Chapter 2
// SPEC_LINK: specbook-chapter-2
func (s *Store) IngestOFX(r io.Reader) (int, error) {
	parsed, err := ofxgo.ParseResponse(r)
	if err != nil {
		return 0, fmt.Errorf("failed to parse OFX: %w", err)
	}

	count := 0
	for _, snt := range parsed.Bank {
		statement, ok := snt.(*ofxgo.StatementResponse)
		if !ok {
			continue
		}

		// 1. Map the Bank's Account ID to our internal account name
		extID := string(statement.BankAcctFrom.AcctID)
		accountName, err := s.getAccountByExtID(extID)
		if err != nil {
			// If account isn't mapped, we use the ExtID as a fallback name
			accountName = extID
		}

		// 2. SET THE ANCHOR: The Bank's "Ledger Balance" is our Point of Truth
		balAmt, _ := parseOFXAmount(statement.BalAmt.String())
		asOfDate := statement.DtAsOf.Time.Format("2006-01-02")

		_, err = s.db.Exec(`
			INSERT INTO balance_anchors (anchor_date, amount, account) 
			VALUES (?, ?, ?)`,
			asOfDate, balAmt, accountName)
		if err != nil {
			return count, fmt.Errorf("failed to set balance anchor: %w", err)
		}

		// 3. PROCESS TRANSACTIONS
		for _, tran := range statement.BankTranList.Transactions {
			if err := s.reconcileOFXTransaction(accountName, tran); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}
// CHUNK_END: ingest-ofx-entrypoint-v1-uuid-k7m9p3r6

// CHUNK_START: reconcile-ofx-transaction-v1-uuid-l2n6q8t4
// BUSINESS_PURPOSE: Reconciles a single OFX transaction: attempts perfect match by check number, falls back to fuzzy amount match for rogue flagging, inserts or skips on fitid conflict, and sets cleared status per specbook Chapter 2 reconciliation rules
// SPEC_LINK: specbook-chapter-2
func (s *Store) reconcileOFXTransaction(accountName string, t ofxgo.Transaction) error {
	cents, err := parseOFXAmount(t.TrnAmt.String())
	if err != nil {
		return err
	}

	fitid := string(t.FiTID)
	date := t.DtPosted.Time.Format("2006-01-02")
	description := strings.TrimSpace(string(t.Name))

	// Default to Cleared for boring stuff (ACH/Debit)
	clearedStatus := 1

	var checkNum sql.NullString
	if t.CheckNum != "" {
		checkNum = sql.NullString{String: string(t.CheckNum), Valid: true}
		// It's a check! If we don't find a match, it MUST be a Rogue.
		clearedStatus = 0
	}

	// 1. ATTEMPT PERFECT MATCH (Check Number)
	if checkNum.Valid {
		var manualID int64
		err := s.db.QueryRow(`
            SELECT id FROM transactions 
            WHERE account = ? AND check_number = ? AND cleared = 0 
            LIMIT 1`, accountName, checkNum.String).Scan(&manualID)

		if err == nil {
			_, err = s.db.Exec(`UPDATE transactions SET fitid = ?, cleared = 1 WHERE id = ?`, fitid, manualID)
			return err
		}
	}

	// 2. ATTEMPT FUZZY MATCH (Amount Only)
	// If the amount matches a manual entry, send it to the Matchmaker as a Rogue!
	var hasSimilarManual bool
	s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM transactions WHERE account = ? AND amount = ? AND cleared = 0)`,
		accountName, cents).Scan(&hasSimilarManual)

	if hasSimilarManual {
		clearedStatus = 0
	}

	// 3. INSERT (Either as a Settled item or a Rogue)
	_, err = s.db.Exec(`
        INSERT INTO transactions (date, check_number, description, amount, type, account, fitid, source, cleared)
        VALUES (?, ?, ?, ?, ?, ?, ?, 'ofx', ?)
        ON CONFLICT(fitid) DO NOTHING`,
		date, checkNum, description, cents, t.TrnType.String(), accountName, fitid, clearedStatus)

	return err
}
// CHUNK_END: reconcile-ofx-transaction-v1-uuid-l2n6q8t4

// CHUNK_START: parse-ofx-amount-v1-uuid-m9p4r1u5
// BUSINESS_PURPOSE: Converts OFX amount strings (e.g. "123.45" or "-45.67") to integer cents without floating-point precision issues; handles negatives, padding/truncation, and empty values per specbook financial integrity rules
// SPEC_LINK: specbook-chapter-1 (Data Model) + Chapter 2 (amount handling)
// CHUNK_VERSION_COMMENT: Core helper; used in ingest and balance anchor
func parseOFXAmount(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	// Split into dollars and cents
	parts := strings.Split(s, ".")

	// Handle negative signs correctly
	negative := false
	if strings.HasPrefix(parts[0], "-") {
		negative = true
		parts[0] = strings.TrimPrefix(parts[0], "-")
	}

	// Parse Dollars
	dollars, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}

	// Parse Cents
	var cents int64
	if len(parts) > 1 {
		cStr := parts[1]
		if len(cStr) > 2 {
			cStr = cStr[:2] // Truncate sub-pennies
		} else if len(cStr) < 2 {
			cStr = cStr + "0" // Pad single digits (.5 -> .50)
		}
		cents, err = strconv.ParseInt(cStr, 10, 64)
		if err != nil {
			return 0, err
		}
	}

	totalCents := (dollars * 100) + cents
	if negative {
		totalCents = -totalCents
	}

	return totalCents, nil
}
// CHUNK_END: parse-ofx-amount-v1-uuid-m9p4r1u5
