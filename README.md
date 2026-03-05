# Crystal Clear Checking (CCC)

### A Sovereign, Single-Binary, Joint Ledger that actually reconciles.

I am Stuart, and my wife is Crystal. Crystal wants me to keep our checkbook balanced and current with the bank's records. She cannot read my handwriting. I cannot read hers.

Our paper check register used to be a war zone of crossed-out numbers and deciphering "abstract art" scribbles. We’d argue over whether a check was for $47 or $74, whether it had cleared, or why we hadn't reconciled in three months. Since most of our life is automated, the only way to know the truth was to check online and copy those details into a paper ledger. It was exhausting.

**We tried the usual apps.** They all wanted our bank logins. They wanted to turn our simple checkbook into a full personal-finance empire with budgets, net-worth tracking, and monthly subscriptions. I felt like we were giving away the keys to our castle just to see the balance.

Crystal looked at me one day and said: **“You need to fix this NOW. I am sick of not knowing what is going on.”**

### What it is (and what it isn't)

It’s not a budgeting app. It’s not a money manager. It’s a bulletproof digital check register that fixes one problem: **Why does the bank know our balance better than we do?**

### The Workflow

1. **The "Single Entry" Rule:** Whoever writes a paper check opens their phone browser. Tap “Add Check,” type the number, date, and amount. Done in 12 seconds. It's typed, legible, and shared instantly.
2. **The "Truth" Import:** Later, I download the latest **OFX** file from our bank. I upload it from my phone or laptop. No "connecting accounts," no Plaid, no passwords.
3. **The Handshake:** The app reads the bank’s `<FITID>` (Unique ID) and `<CHECKNUM>`. It’s smart enough to skip duplicates—it is **idempotent by design**. If the check number matches a manual entry, it updates the record to "Cleared" while preserving our manual memo/category.
4. **Shared Truth:** We open the app from the couch and see a real-time balance (Initial balance + every signed transaction since then) and a clear list of uncleared checks.

---

### Security Architecture: The "Physical Proximity" Model

We don't use passwords because passwords can be guessed or leaked. We use **Physical-Proximity Pairing.**

* **The Access Trick:** To add a phone to your household, you must prove you have local access to the machine. You initiate a secure SSH tunnel to the host (the "Loopback"), which generates a one-time QR code.
* **The Golden Ticket:** Scanning this grants your phone a long-lived, rotating **JWT (JSON Web Token)**. This is the digital version of handing someone a physical key to your house.
* **No Public Credentials:** There is no "Login Page" for hackers to brute-force. If a device hasn't been physically paired by you, it doesn't even see the app exists.
* **Secure Remote Access:** Access the ledger on your home Wi-Fi (LAN) or over a VPN/Port Forward. Since the JWT is granted locally, your ledger remains a private fortress.

---

### Why this is different

* **Sovereign & Private:** No cloud. No telemetry. No exfiltration. Your data lives in a local SQLite database on your own hardware (Ubuntu laptop, Raspberry Pi, etc.).
* **Single Binary Purity:** Written in Go. One file to run. No external runtimes or complex dependencies.
* **Cents-Based Math:** We store everything as signed integers. No floating-point rounding errors. Ever.
* **Idempotent:** Upload the same OFX file ten times; the ledger remains perfect.

### Technical Reality Check

This is built for people who value control over convenience.

* **Linux Centric:** Runs on stock Ubuntu/Debian/PiOS.
* **Self-Hosted:** You own the binary, you own the DB, you own the security.
* **Basic CLI Comfort:** You’ll need to be comfortable building a Go binary and running a service. If that sounds like a feature instead of a barrier, you’re exactly who this is built for.

Crystal and I no longer fight over illegible scribbles. The checkbook stays balanced. We both see the same truth. Welcome to Crystal Clear Checking.

**Let’s keep it clear.**

---

### Next Step

This README is a perfect "Face" for the project. **You are ready to start the fresh chat session.** When you do, paste the **SpecBook** first, then this **README**, and tell the AI:
*"We are starting from a clean slate. This is the SpecBook and the README. Let's begin Phase 1: The Database Schema and Go Structs."*
