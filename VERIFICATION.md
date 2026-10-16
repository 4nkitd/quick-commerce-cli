# Verification

## Milestone 1, 2026-10-16

Verified locally through the built `./qc` binary:

- Blinkit OTP requested, fresh code retrieved with `imsg`, login completed,
  authenticated order-count request returned HTTP 200, session saved.
- Blinkit `auth status` restored that session in a fresh browser without another
  OTP and returned `status: verified`.
- Zepto OTP requested, fresh code retrieved with `imsg`, login completed,
  authenticated ongoing-order request returned HTTP 200, session saved.
- Zepto `auth status` restored that session in a fresh browser without another
  OTP and returned `status: verified`.
- Zepto `orders 1` returned one order. `search milk` returned 28 products.
  Only counts were printed in the test summary.
- Full cross-platform order history and category spend analytics (`./qc spend`)
  verified against live accounts: 16 Zepto records (10 delivered, 6 cancelled)
  and 408 Blinkit records (397 delivered, 5 cancelled, 6 return-pickup entries),
  with itemized bill/refund reconciliation and Markdown + Mermaid chart output.
- Agent ergonomics improvements verified: Blinkit product search
  (`qc blinkit search`), cross-platform live price comparison (`qc compare`),
  single-order inspection (`qc blinkit order`, `qc zepto order`), and `--json`
  support across all read commands were verified live. Automatic Zepto 401/403
  session refresh + 429 backoff are verified by unit tests (`client_test.go`).
- No checkout, payment, cancellation, support case or refund submission occurred.

Issues found and fixed during live testing:

- Wait for Zepto's React click handler, not just its server-rendered login button.
- Wait for Blinkit's authenticated local state before checking the session.
- Restore cookies individually. agent-browser bulk import drops cookie domains
  and security attributes; it is not used by this implementation.
- Restore Blinkit storage before loading its app, so guest-state initialization
  cannot overwrite the restored authentication data. Two consecutive fresh-browser
  checks passed after this fix.
- Isolate agent-browser configuration as well as its environment, including cleanup.
- Hold the provider authentication lock during legacy session import too.

Automated checks cover provider routing helpers, phone validation, OTP sender and
freshness filtering, repeated/ambiguous codes, private atomic writes, cookie
security attributes, legacy imports, credential-safe HTTP errors, disabled
redirects and missing-address cart protection.
Regression tests also cover controlled browser configuration, restore ordering,
import locking, automatic 401/403 refresh + 429 backoff, Blinkit search/compare
scoring, refund/fee allocation and both providers' blocked side-effect commands.

Not verified: live natural expired-token renewal on Blinkit, Linux live
authentication, manual OTP login, legacy cookie import against a live provider,
Blinkit cart/checkout APIs, purchase flow, support or refunds. No global binary
was installed.

Browser configuration isolation also passed a live saved-session check while the
working directory's `agent-browser.json` specified an invalid CDP connection and
provider, and the environment supplied a conflicting CDP connection.

Review status: independent `/review` found two issues, both fixed with regression
tests. Follow-up review reported no remaining actionable findings.
`go test -race ./...`, `go vet ./...` and `git diff --check` passed.
