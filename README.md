# Quick Commerce CLI (`qc`)

A fast Go command-line client for **Zepto** and **Blinkit**. Authenticate via SMS OTP (with optional automatic OTP retrieval through `imsg` on macOS), reuse saved sessions, search live catalogs, compare prices across both platforms in one command, inspect order history, and generate full category spend analytics with Mermaid charts.

This is an unofficial client. Provider websites and private APIs can change.

---

## Install

### 1. Homebrew (macOS & Linux)

```sh
brew tap 4nkitd/tap
brew install qc
```

### 2. Winget / Windows

Install from the included Winget manifest (or after the package is merged into `microsoft/winget-pkgs`):

```powershell
# Once published to winget-pkgs:
winget install 4nkitd.QuickCommerceCLI

# Or install directly from the release manifest right now:
winget install --manifest .\packaging\winget\
```

*(Note: If local manifest installation is not enabled on your Windows machine, run `winget settings --enable LocalManifestFiles` once in an elevated prompt, or download `qc_v0.2.0_windows_amd64.zip` from [Releases](https://github.com/4nkitd/quick-commerce-cli/releases/latest).)*

### 3. Go install (macOS, Linux, Windows)

Requires Go 1.25+:

```sh
go install github.com/4nkitd/quick-commerce-cli@latest
# Optional: alias or rename the installed binary to `qc`
ln -sf "$(go env GOPATH)/bin/quick-commerce-cli" "$(go env GOPATH)/bin/qc"
```

### 4. Prebuilt binaries (Linux, macOS, Windows)

Download the archive for your OS and architecture (`darwin_arm64`, `darwin_amd64`, `linux_amd64`, `linux_arm64`, `windows_amd64`, `windows_arm64`) from the [latest GitHub Release](https://github.com/4nkitd/quick-commerce-cli/releases/latest) and place `qc` (or `qc.exe`) on your `PATH`.

### 5. Build from source

```sh
git clone https://github.com/4nkitd/quick-commerce-cli.git
cd quick-commerce-cli
go build -o qc .
./qc --version
```

---

## Capabilities

| Feature | Zepto | Blinkit |
| --- | --- | --- |
| Phone OTP login (`login`) | Yes | Yes |
| Automatic OTP retrieval through `imsg` (macOS) | Yes | Yes |
| Saved session reuse & live auth check (`auth status`) | Yes | Yes |
| Live product search (`search`) | Yes | Yes |
| Cross-platform live price comparison (`compare`) | Yes | Yes |
| Full paginated order history (`orders`) | Yes | Yes |
| Itemized single-order inspection (`order`) | Yes | Yes |
| Lifetime & category spend analytics (`spend`) | Yes | Yes |
| Live order tracking (`track`), wallet (`wallet`) | Yes | Not yet |
| Cart, checkout, support claims, refunds | Disabled in this milestone | Disabled in this milestone |

See [ROADMAP.md](ROADMAP.md) for upcoming milestones and [VERIFICATION.md](VERIFICATION.md) for verification notes.

---

## Authentication

Authentication requires [`agent-browser`](https://agent-browser.dev) and Lightpanda on `PATH`. On macOS, [`imsg`](https://github.com/steipete/imsg) can automatically read incoming OTP SMS messages when Full Disk Access is enabled for your terminal.

```sh
# Automatic OTP login via iMessage (macOS)
qc zepto login --phone <10-digit-mobile> --otp-source imsg
qc blinkit login --phone <10-digit-mobile> --otp-source imsg

# Manual OTP login (macOS, Linux, Windows — reads OTP from stdin)
qc zepto login --phone <10-digit-mobile>
qc blinkit login --phone <10-digit-mobile>
```

Check or refresh saved sessions:

```sh
qc --json zepto auth status
qc --json blinkit auth status
qc --json blinkit whoami
```

* `auth status` restores your saved session in an isolated headless browser, verifies it with a read-only provider call, and persists refreshed tokens.
* Zepto API calls also automatically refresh expired sessions on `HTTP 401/403` and back off on `HTTP 429`.

---

## Usage

### Cross-platform price comparison

Compare live prices, MRP, pack sizes, and stock across both Blinkit and Zepto in a single command:

```sh
qc compare "Aashirvaad Shudh Chakki Atta 10 kg"
qc --json compare "Red Bull Sugar Free 250 ml"
```

### Product search

```sh
qc zepto search "Amul Cow Milk"
qc blinkit search "Amul Cow Milk"
qc --json blinkit search "Greek Yogurt"
```

### Order history & single-order details

```sh
qc zepto orders 10
qc zepto orders --all
qc blinkit orders 10
qc blinkit orders --all

qc zepto order <order-code>
qc blinkit order <order-id-or-code>
```

### Lifetime & category spend analytics

Analyze your complete order history across both platforms (gross delivered, direct refunds, promo voucher refunds, delivery/handling/surge fees, 10-category item breakdown, and monthly spend trends) and export a Markdown report with Mermaid pie and bar charts:

```sh
qc spend
qc spend --md --out SPENDING-REPORT.md
qc --json spend
```

### Structured JSON for AI agents

Pass `--json` as the first flag on any read, auth, comparison, or analytics command. Monetary values are integer paise (`currency: "INR"`, `amount_unit: "paise"`):

```sh
qc --json providers
qc --json compare "Mountain Dew"
qc --json zepto wallet
```

---

## Session storage & security

* **Zepto session:** Stored at `<UserConfigDir>/zepto/session.json` (macOS: `~/Library/Application Support/zepto/session.json`) to preserve backward compatibility with existing setups.
* **Blinkit session:** Stored separately at `<UserConfigDir>/quick-commerce/blinkit/session.json`.
* Session and cache files are written atomically with `0600` permissions. Never commit or share your session files.
* Every browser invocation uses a private temporary configuration that ignores user/project `agent-browser` configs and `AGENT_BROWSER_*` connection variables.

## Tests

```sh
go test -race ./...
go vet ./...
```
