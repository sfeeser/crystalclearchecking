**SAAYN Project Name (https://saayn.org/manifesto)**  
Crystal Clear Checking (crystalclearchecking.com) – A sovereign, self-hosted, minimal joint checkbook tracker with OFX ingestion and automatic paper-check reconciliation.

The name is a loving tribute to Stuart's wife, Crystal, who rightfully demanded clearer, more reliable tracking of their shared finances than Stuart's previous ad-hoc methods were providing. "Crystal clear" captures both her name and the app's core promise: transparent, honest visibility into every transaction, balance, and cleared check—no confusion, no double-entry, just straightforward sovereignty over your money.

**Overall System Purpose**  
A radically simple, private, local-first financial ledger for a household (two people managing one shared joint account). Replaces paper checkbooks and bloated SaaS with full ownership: data in local SQLite on an Ubuntu laptop or Raspberry Pi, access via phone/LAN browser with QR-paired JWT and silent refresh, no cloud, no subscriptions, no vendor lock-in. Emphasizes: enter paper checks once, never double-entry, auto-reconcile via OFX, view honest current balances, minimal work (phone form \+ OFX drop).

**Deployment Target**

- Single statically-linked Go executable (binary: ccc-server).  
- Runs on stock Ubuntu laptop or Raspberry Pi (arm64/arm supported).  
- SQLite database file (ledger.db) on local filesystem (default: \~/.crystalclearchecking/ledger.db).  
- No external runtimes or services required.  
- Local-only access (LAN via main port, pairing via SSH-forwarded loopback).

**Chapter 0: Core Intent & Non-Negotiables**

1. Enter paper check at time of writing, and only once — manual entry creates canonical record immediately via phone form.  
2. Never double-entry anything — one truth: manual check persists until OFX merges/clears it; bank transactions deduplicated via .  
3. Automatic checkbook reconciliation — OFX import uses as absolute truth and for matching paper checks → marks cleared, updates fields from bank truth, preserves manual category/memo if unchanged.  
4. See current balances on the phone — real-time computed running balance(s) per account (including starting balance), prominently displayed, with clear "as-of last import" timestamp. Assumes regular OFX imports.  
5. The only real work is filling out a few fields on the phone and uploading a new OFX file — minimal form, simple upload → process → feedback.  
6. Powerful reports possible if users leverage account/category features — sums/groupings by account, category, parent\_category.  
7. Household as single logical user — Stuart & Crystal share one JWT and full read/write access to the joint ledger. No per-user separation in v1; real-world overwrite risks apply. Optional device labeling for logging "who likely entered this".

**Non-Negotiables**

- Single entry for checks; never require re-entry post-clearing.  
- Sovereignty: local SQLite only, no mandatory cloud, no telemetry.  
- Minimal friction: phone-first UX, SSH-forwarded pairing, OFX drop ingest.  
- Honesty: disclose data freshness and use starting balances for true account totals.  
- Starve complexity: no splits, no offline queuing, no budgeting/envelopes in v1.  
- Single-binary purity: everything compiles into one executable (include github.com/aclindsa/ofxgo).

**Chapter 1: Data Model & Persistence**  
**Core Entity** — Transaction (table: transactions)

Fields:

- id INTEGER PRIMARY KEY AUTOINCREMENT  
- date TEXT NOT NULL (ISO 8601 YYYY-MM-DD, indexed)  
- check\_number TEXT (indexed, key for merging paper checks)  
- description TEXT  
- original\_description TEXT (nullable)  
- amount INTEGER NOT NULL (cents; debits negative, credits positive)  
- type TEXT CHECK(type IN ('Debit', 'Credit', 'Pending', 'Check'))  
- category TEXT DEFAULT 'Uncategorized'  
- parent\_category TEXT  
- account TEXT (references accounts.name)  
- tags TEXT (comma-separated)  
- memo TEXT  
- device\_label TEXT (optional, e.g., "Stuart-iPhone")  
- cleared BOOLEAN DEFAULT FALSE  
- voided BOOLEAN DEFAULT FALSE  
- bank\_fitid TEXT UNIQUE (OFX – absolute unique key per bank account)  
- source TEXT DEFAULT 'manual' ('manual', 'ofx', 'csv\_legacy')  
- imported\_at DATETIME DEFAULT CURRENT\_TIMESTAMP

Constraints: UNIQUE(date, check\_number, amount) partial; bank\_fitid UNIQUE; NOT NULL on date, amount

**Accounts Table** (for starting balances \+ OFX matching)

- id INTEGER PRIMARY KEY AUTOINCREMENT  
- name TEXT UNIQUE NOT NULL (e.g., "Relationship Banking Checking")  
- ext\_id TEXT UNIQUE (bank's or equivalent identifier for OFX matching)  
- starting\_balance INTEGER NOT NULL (cents)  
- starting\_date TEXT NOT NULL (ISO YYYY-MM-DD)  
- currency TEXT DEFAULT 'USD'

**Storage**

- Single SQLite file, WAL mode enabled.  
- Balance calc: starting\_balance \+ SUM(amount) WHERE date \>= starting\_date AND voided \= FALSE AND account \= ?

## Chapter 1.1: Data Durability & Portability

### 1. Hot Backup Mechanism

Since the system operates in WAL (Write-Ahead Logging) mode, a simple file `cp` while the server is active can result in a malformed or "checkpoint-trapped" backup. CCC must provide a safe internal backup mechanism.

* **Implementation:** Utilize the `sqlite3_backup` API (via the Go driver) to perform a "Hot Backup." This locks the source database only momentarily to initialize and then streams pages to a destination file.
* **CLI Command:** `ccc-server backup --path <destination_path>`
* If no path is provided, default to `~/.crystalclearchecking/backups/ccc_backup_YYYYMMDD_HHMMSS.db`.


* **Safety Check:** Every backup operation must conclude with a `PRAGMA integrity_check` on the *newly created* backup file before reporting success.

### 2. Startup Integrity Verification

On every application launch, before the web server starts, CCC shall:

* Run `PRAGMA integrity_check`.
* If the check fails, the binary must **panic and exit** with a clear error message, preventing the user from writing new (possibly corrupt) data over a failing database.
* **Console Output:** "Database integrity verified. [OK]"

### 3. Portable Data Export (The "Sovereign" Exit)

To prevent vendor lock-in and ensure the data survives even if the SQLite format is deprecated in 50 years, CCC provides a human-readable export.

* **Format:** Standardized CSV (UTF-8).
* **Fields:** All fields from the `transactions` table, including `bank_fitid` and `cleared` status.
* **Command:** `ccc-server export --format csv`
* **UI Option:** A "Download Full History (CSV)" button located in the **Settings** or **Reports** section of the web dashboard.

### 4. Restoration Workflow

Restoration is intentionally manual to prevent accidental data overwrites via the UI.

1. Stop the `ccc-server` service.
2. Rename the existing (corrupt or old) `ledger.db` to `ledger.db.old`.
3. Copy the desired backup file into the primary location as `ledger.db`.
4. Restart `ccc-server`.

### 5. Recommended Backup Policy (Admin Guide)

The README shall include a recommended `crontab` entry for the Linux admin:

```bash
# Every day at 3:00 AM, perform a safe hot backup
0 3 * * * /path/to/ccc-server backup --path /path/to/backups/daily_backup.db

```

**Chapter 2: File Ingestion & Reconciliation**  
**Trigger** — Multipart upload via /upload (primary OFX, CSV fallback).

**Parsing Rules**

- **Primary: OFX** — Use github.com/aclindsa/ofxgo to parse SGML (v1.x) or XML (v2.x) files.  
- **Account Validation** — For each statement (BankStatementResponse, CCStatementResponse, etc.):  
  - Extract (or equivalent from , , etc.).  
  - Match against accounts.ext\_id (primary) or accounts.name (fallback).  
  - If no match → skip the entire statement, log warning ("Skipping unmatched account: ").  
  - If match → process transactions only for that account.  
- **Standardized Mapping** (OFX tags):  
  - → type (CHECK → Check/Debit, DEBIT → Debit, CREDIT → Credit, PAYMENT → Debit, etc.)  
  - → date (parse YYYYMMDD\[HHMMSS\] → YYYY-MM-DD)  
  - → amount (float → integer cents, signed by TRNTYPE)  
  - → bank\_fitid  
  - → check\_number  
  - → description  
  - → memo  
- **Fallback: CSV** — Multi-format date parsing ("1/2/2006", "2/1/2006", "2006-01-02"); amount float → cents signed by Type.

**Deduplication & Merge Logic** (in transaction)

1. **bank\_fitid present** (OFX):  
   - INSERT ... ON CONFLICT(bank\_fitid) DO NOTHING  
     - Optional: DO UPDATE SET description \= EXCLUDED.description, memo \= EXCLUDED.memo WHERE EXCLUDED.imported\_at \> imported\_at (refresh bank truth if newer)  
2. **No bank\_fitid** (CSV/manual fallback):  
   - If check\_number present: SELECT uncleared (cleared=FALSE, voided=FALSE) WHERE check\_number \= ? AND ABS(amount \- ?) \<= 1 LIMIT 1  
     - Match → UPDATE: set bank\_fitid (if available), cleared=TRUE, adopt bank date/amount/desc/memo; preserve manual category/tags.  
     - No match → INSERT.  
   - Else: Probable dupe on date/amount/description hash → skip if match. Else INSERT.

**Chapter 3: Manual Check Entry & Editing**  
**Forms** (mobile-first, server-rendered)

- Add: check\_number (req), date, amount (dollars → cents), description/memo, category, account. → INSERT source='manual', cleared=FALSE, voided=FALSE, device\_label if tracked.  
- Edit: Pre-filled form → UPDATE. Include "Void" button → set voided=TRUE (excludes from uncleared/balances).  
- List: Paginated table, sort date DESC default, filters (uncleared, voided, category, account).

**Chapter 4: Authentication & Access (Loopback QR-Pairing with Silent Refresh)**  
**Mechanism** — Shared household JWT access via QR pairing (accessed through SSH local port forwarding for headless/remote setups), with silent refresh for long-lived sessions using short-lived access tokens and rotating refresh tokens. Brute-force protection via gated endpoint availability and per-IP rate limiting.

- **Tokens Issued During Pairing**:  
  - Access Token (JWT): Short-lived, 30 minutes expiry.  
  - Refresh Token: Long-lived, 90 days initial, HttpOnly/Secure/SameSite=Lax cookie, rotated on use.  
- **Silent Refresh**: POST /refresh → validate → new access \+ new refresh token.  
- **Refresh Tokens Table**: token\_hash, jti, expiry, device\_fingerprint, revoked.  
- **Pairing**: SSH forward → [http://127.0.0.1:55888/pair](http://127.0.0.1:55888/pair) → QR with nonce (UUID, 5–10 min).  
- **/pair/approve**:  
  - Ignore if no active nonce → immediate 404 Not Found.  
  - Per-IP rate limit: 5 requests/min → 429 Too Many Requests on exceed.  
- **Logout**: POST /logout → delete current device's refresh token.  
- **Shared access**: Identical tokens across devices.  
- **Revocation**: CLI revoke-all \+ /logout UI.  
- **Non-negotiables**: LAN-only, loopback pairing, brute-force gated, no public exposure.

**Chapter 5: Web Interface Requirements**  
**Tech** — Go stdlib net/http \+ html/template (or lightweight router); embedded assets via //go:embed.

**Pages/Endpoints**

- / (dashboard): recent transactions, uncleared checks summary, current balance(s) per account \+ total, last-import timestamp, server status (/health badge), upload button.  
- /ledger: full list with filters/sort.  
- /add-check: minimal form.  
- /edit/:id: edit form with Void option.  
- /upload: multipart → parse/process OFX/CSV → redirect with feedback.  
- /health: 200 OK if alive.  
- /logout: POST to revoke current refresh token.  
- Static: minimal CSS (Pico.css embedded).

**UX Principles**

- Mobile-first.  
- Server-rendered \+ optional htmx for reactivity.  
- Clear feedback on actions.  
- No JS frameworks, ads, telemetry.

**Chapter 6: Reporting & Balances**

- **Balances**: starting\_balance \+ SUM(amount) WHERE date \>= starting\_date AND voided \= FALSE AND account \= ?. Show "as-of \[latest imported\_at\]". Highlight pending/uncleared outflows.  
- **Reports** (minimal v1): Spending by category (debits grouped), per-account summaries, uncategorized list.  
- Note: Splits not supported in v1; use memo for breakdowns.

**Chapter 7: Deployment & Runtime (Single Executable)**

- Compile: go build \-o ccc-server. Cross-compile for arm64/arm.  
- Embedded: templates, CSS, QR libs (pure-Go: skip2/go-qrcode, mdp/qrterminal).  
- Dependencies: github.com/aclindsa/ofxgo (OFX parsing), golang-jwt/jwt, modernc.org/sqlite.  
- Listeners: main on :8080, pairing on 127.0.0.1:55888.  
- CLI: cobra or flag-based (run, pair, list-tokens, revoke-all).  
- Security: no deserialization risks; minimal deps.  
- Run: ./ccc-server \--db \~/.crystalclearchecking/ledger.db.

**Chapter 8: Future-Proofing & Extensibility**  
Planned (out-of-v1, spec-compatible):

- Directory watcher for OFX drops.  
- Basic auto-categorization rules.  
- Export: CSV/JSON dump.  
- Full OFX statement download via ofxgo querying (stretch).

**Non-Goals**

- Multi-tenancy beyond shared household.  
- Remote/cloud hosting (beyond VPN/SSH).  
- Native mobile app.  
- Complex budgeting/forecasting.  
- Offline entry (requires server reachability).  
- Splits (use memo in v1).

