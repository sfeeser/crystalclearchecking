# Crystal Clear Checking (CCC)

### A sovereign, single-binary joint checkbook that actually reconciles.

I am Stuart, and my wife is Crystal. Crystal wants me to keep our checkbook balanced and current with the bank's records. She cannot read my handwriting. I cannot read hers.

Our paper check register used to be a war zone of deciphering "abstract art" scribbles. We’d argue over whether a check was for $47 or $74, whether it had cleared, or why we hadn't reconciled in three months. Since most of our life is automated, the only way to know the truth was to check online and copy those details into a paper ledger. It was exhausting.

**We tried the usual apps.** They all wanted our bank logins. They wanted to turn our simple checkbook into a full personal-finance empire. I felt like we were giving away the keys to our castle just to see the balance.

Crystal looked at me one day and said: **“You need to fix this NOW. I am sick of not knowing what is going on.”**

### What it is (and what it isn't)

It’s not a budgeting app. It’s not a money manager. It’s a bulletproof digital check register that fixes one problem: **Why does the bank know our balance better than we do?**

## Workflow

1. **Single Entry:** Whoever writes a paper check opens the phone browser. Tap “Add Check,” type the number, date, and amount. Done in 12 seconds. It's typed, legible, and shared instantly.
2. **The Import:** Later, one of us downloads the latest **OFX** file from the bank and uploads it to CCC. No "connecting accounts," no third-party APIs, no passwords.
3. **The Handshake:** The importer reads the bank’s `<FITID>` (transaction ID) and `<CHECKNUM>` fields. OFX transactions are deduplicated using these identifiers, making imports **idempotent**. Importing the same file multiple times will not create duplicate transactions.
4. **Shared Truth:** The app reconciles manual entries with bank truth, marking them "Cleared" while preserving your manual memos.

## Quick Start

### Requirements

* Go 1.22+
* Linux (Ubuntu, Debian, Raspberry Pi OS, or any Linux capable of running Go)

### Build

```bash
git clone https://github.com/yourname/crystalclearchecking
cd crystalclearchecking
go build -o ccc-server

```

### Run

```bash
./ccc-server --db ~/.crystalclearchecking/ledger.db

```

Then open your browser to `http://localhost:8080`.

---

## Design Principles

Crystal Clear Checking follows a strict manifesto:

1. **Single Entry:** Checks are entered once when written.
2. **Bank Truth:** OFX imports reconcile and clear transactions automatically.
3. **Local First:** All data lives in a local SQLite database. No cloud.
4. **Minimal Surface Area:** No unnecessary features.
5. **Deterministic Ledger:** Integer cents only. No floating point rounding errors. Ever.

## Security Model: Physical Proximity

CCC does not use traditional username/password authentication. Instead, it uses a **Physical-Proximity Pairing** model.

* **No Password Login:** Access requires pairing a device locally. This is initiated via an SSH tunnel to the host (loopback), which generates a one-time QR code.
* **The Token:** Scanning the QR code grants the device a rotating **JWT (JSON Web Token)**. Unpaired devices cannot authenticate.
* **Secure Remote Access:** While designed for LAN use, CCC works over a VPN or secure port forward. Because the JWT is granted locally, your ledger remains a private fortress.

## Explicit Non-Goals

CCC intentionally avoids feature creep. It does **NOT** include:

* Budgeting tools or "envelope" systems
* Automatic bank credential syncing (Plaid, etc.)
* Investment or 401k tracking
* Bill pay or automation
* Multi-user enterprise accounting

## Technical Reality Check

This is built for people who value control over convenience.

* **Self-Hosted:** You own the binary, the database, and the security.
* **CLI Comfort:** You should be comfortable building a Go binary and managing a Linux service via SSH.

## License

[MIT License](https://www.google.com/search?q=LICENSE)

