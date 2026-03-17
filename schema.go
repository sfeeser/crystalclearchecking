package main

// CHUNK_START: imports-and-package-v1-uuid-s6t2v9w4
// BUSINESS_PURPOSE: Declares the package for the database schema definition. No external imports are currently required, but this chunk serves as the placeholder for any future additions (e.g., migration tools, schema validation libs). Single source of truth for schema-related dependencies.
// SPEC_LINK: specbook-chapter-1 (Data Model) + non-negotiables on minimal dependencies
// CHUNK_VERSION_COMMENT: Minimal; ready for expansion if schema tooling is added
// (No imports needed at present)
// CHUNK_END: imports-and-package-v1-uuid-s6t2v9w4

// CHUNK_START: dbschema-constant-v1-uuid-x8y3z1r7
// BUSINESS_PURPOSE: Defines the complete SQLite schema as a constant string, including WAL mode for concurrency, accounts table for truth anchors, transactions table for ledger entries, and unique constraints/indices to enforce financial integrity and prevent duplicates per specbook Chapter 1 (Data Model) and Chapter 1.1 (Durability & Hot Backup)
// SPEC_LINK: specbook-chapter-1 + chapter-1.1
const DBSchema = `
PRAGMA journal_mode = WAL;

-- Chapter 1: Accounts Table
-- Stores starting "truth" balances and OFX mapping IDs.
CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,         -- e.g., "Joint Checking"
    ext_id TEXT UNIQUE,               -- Bank identifier (e.g., Routing + Acct)
    starting_balance INTEGER NOT NULL, -- Integer cents
    starting_date TEXT NOT NULL,       -- ISO 8601 (YYYY-MM-DD)
    currency TEXT DEFAULT 'USD' NOT NULL
);

-- Chapter 1: Transactions Table
-- The core ledger. Supports manual entries and OFX imports.
CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,                -- ISO 8601 (YYYY-MM-DD)
    check_number TEXT,                 -- String to handle leading zeros (00123)
    description TEXT NOT NULL,
    original_description TEXT,         -- Preservation of raw bank data
    amount INTEGER NOT NULL,           -- Integer cents (Debits are negative)
    type TEXT CHECK(type IN ('Debit', 'Credit', 'Pending', 'Check')),
    category TEXT DEFAULT 'Uncategorized',
    parent_category TEXT,
    account TEXT NOT NULL,             -- References accounts.name
    tags TEXT,                         -- Comma-separated strings
    memo TEXT,
    device_label TEXT,                 -- For "Who likely entered this"
    cleared BOOLEAN DEFAULT 0,
    voided BOOLEAN DEFAULT 0,
    bank_fitid TEXT UNIQUE,            -- OFX absolute unique key per transaction
    source TEXT DEFAULT 'manual',      -- manual, ofx, csv_legacy
    imported_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(account) REFERENCES accounts(name)
);

-- Indices for performance and reconciliation logic
CREATE INDEX IF NOT EXISTS idx_tx_date ON transactions(date);
CREATE INDEX IF NOT EXISTS idx_tx_check_num ON transactions(check_number);
CREATE INDEX IF NOT EXISTS idx_tx_account ON transactions(account);

-- Chapter 1.1: Data Durability Constraints
-- Prevents Stuart and Crystal from accidentally entering the same manual 
-- check twice before it hits the bank.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tx_manual_dupe 
ON transactions(date, check_number, amount) 
WHERE check_number IS NOT NULL AND source = 'manual';
`
