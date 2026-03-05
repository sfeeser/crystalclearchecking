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

## Technical Skills Requirement

This is built for people who value control over convenience.

* **Self-Hosted:** You own the binary, the database, and the security.
* **CLI Comfort:** You should be comfortable building a Go binary and managing a Linux service via SSH.


## Pairing Your Phone (One-Time Setup)

Crystal Clear Checking uses **local device pairing instead of passwords**. There are no usernames, passwords, email tokens, or cloud accounts. Each device is paired once using a **one-time QR code** generated directly by the server. After pairing, the device receives a rotating **JWT (JSON Web Token)** that keeps the session active through silent refresh. In normal use you never need to log in again unless you log out or revoke the device. Pairing requires **local or SSH access to the machine** hosting Crystal Clear Checking. This ensures that only someone with physical access or SSH access to the server can authorize a new device.


## Step-by-Step Pairing

1. If your CCC server has a browser available, open the pairing page `http://127.0.0.1:55888/pair` then skip to step 3

2. If your CCC server is headless (Raspberry Pi, remote server, or VM), create an SSH tunnel to the CCC server)

   1. Define the server connection variables

        ```bash
        export CCC_HOST=     #CCC server IP address goes here
        export CCC_USER=     #CCC server user name goes here
        ```

    2. Create the SSH tunnel
    
        ```bash
        ssh -L 127.0.0.1:55888:127.0.0.1:55888 $CCC_USER@$CCC_HOST
        ```
    
    3. Open the ssh-forwarded pairing page locally:

        ```
        http://127.0.0.1:55888/pair
        ```

3. Start pairing in the browser

    Click the **PAIR NOW** button

4. The server generates a **one-time QR code** and displays it on the page. A plain text URL is also shown as a fallback in case QR scanning is unavailable.

5. The QR code contains a **short-lived pairing token (nonce)**.

6. Scan the QR code with your phone's camera.

7. Your phone will automatically open a link that looks like this:

    ```
    http://192.168.1.50:8080/pair/approve?nonce=abc123...
    ```

    > This link sends the pairing token to the CCC server for validation.
    > The server verifies the one-time nonce.
    > If the token is valid and has not expired:  
    > - an **access token** is issued
    > - a **refresh token** is stored in the phone's secure browser cookie
    > - The browser is redirected to the CCC dashboard
    > - Pairing tokens automatically expire after a short time (typically 5–10 minutes).
    
8. Pairing complete

    Your device is now paired and fully authorized.
   
    You can immediately:  
      - add checks
      - upload OFX files
      - view balances
      - review uncleared transactions

    Silent refresh keeps the session active automatically, so you normally never need to re-pair the device.

    You will only need to repeat the pairing process if you:
       - log out
       - clear browser cookies
       - revoke the device from the server


## Why OFX Instead of Bank Logins

CCC deliberately avoids direct bank logins or aggregation services.

Many financial apps require access to your banking credentials through services such as Plaid or similar APIs. This creates a third party that can access transaction history and account details.

Crystal Clear Checking uses **manual OFX export** instead.

Advantages:

- No sharing bank credentials
- No third-party financial aggregators
- No background access to your account
- Full control over when data is imported

The bank remains the source of truth, but you remain in control of access.
