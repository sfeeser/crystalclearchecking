package main

import (
	"fmt"
	"io"
	"math"

	"github.com/aclindsa/ofxgo"
)

func (s *Store) IngestOFX(r io.Reader) (int, error) {
	parsed, err := ofxgo.ParseResponse(r)
	if err != nil {
		return 0, fmt.Errorf("failed to parse OFX: %w", err)
	}

	count := 0
	for _, snt := range parsed.Bank {
		// Use the correct type name: StatementResponse
		statement, ok := snt.(*ofxgo.StatementResponse)
		if !ok {
			continue
		}

		accountName, err := s.getAccountByExtID(string(statement.BankAcctFrom.AcctID))
		if err != nil {
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

func (s *Store) reconcileOFXTransaction(accountName string, t ofxgo.Transaction) error {
	// Conversion from ofxgo.Amount (Decimal) to float64
	amountFloat, _ := t.TrnAmt.Float64()
	cents := int64(math.Round(amountFloat * 100))

	fitid := string(t.FiTID) // Fixed: FiTID not FITID
	date := t.DtPosted.Time.Format("2006-01-02")
	
	var checkNum *string
	if t.CheckNum != "" {
		s := string(t.CheckNum)
		checkNum = &s
	}

	if checkNum != nil {
		var manualID int64
		err := s.db.QueryRow(`
			SELECT id FROM transactions 
			WHERE account = ? AND check_number = ? AND cleared = 0 AND voided = 0 
			LIMIT 1`, accountName, *checkNum).Scan(&manualID)

		if err == nil {
			_, err = s.db.Exec(`
				UPDATE transactions SET bank_fitid = ?, cleared = 1, original_description = ?
				WHERE id = ?`, fitid, t.Name, manualID)
			return err
		}
	}

	_, err := s.db.Exec(`
		INSERT INTO transactions (date, check_number, description, original_description, amount, type, account, bank_fitid, source, cleared)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'ofx', 1)
		ON CONFLICT(bank_fitid) DO NOTHING`,
		date, checkNum, t.Name, t.Name, cents, t.TrnType.String(), accountName, fitid)

	return err
}
