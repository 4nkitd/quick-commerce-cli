# Quick Commerce CLI

Go CLI for Zepto and Blinkit. Executable: `qc`. Keep the existing module and
Zepto storage identifiers so existing installations do not lose their state.

## Milestone 1: authentication and provider selection

- [x] Rename user-facing CLI and add explicit `zepto` / `blinkit` routing.
- [x] Phone OTP login, with optional local `imsg` code retrieval.
- [x] Separate private session files, reusable browser cookies, live auth checks.
- [x] JSON authentication output, bounded waits, no automatic OTP resends.
- [x] Tests, live login tests without purchases, and `/review`.
- [ ] Boss acceptance before milestone 2.

## Agent ergonomics & read-parity improvements (completed 2026-10-16)

- [x] Automatic Zepto session refresh on HTTP 401/403 and automatic rate-limit backoff on HTTP 429.
- [x] Full multi-page order history (`orders [n|--all]`) and itemized order inspection (`order <id-or-code>`) for both Zepto and Blinkit.
- [x] Live Blinkit product search (`qc blinkit search <query>`).
- [x] Cross-platform live price comparison (`qc compare <query>`) and lifetime/category spend analytics (`qc spend [--md] [--out]`).
- [x] Full `--json` output across all read, auth and analytics commands (`search`, `compare`, `orders`, `order`, `track`, `wallet`, `spend`, `whoami`, `auth status`).

## Milestone 2: shopping and order management

- [ ] Search, addresses, cart quotes, orders and tracking for both providers.
- [ ] Remove account-specific assumptions; require explicit address selection.
- [ ] Preview checkout with exact items, quantities, fees and payment method.
- [ ] Explicitly confirmed order submission, duplicate protection and recovery
      from uncertain responses. Never automatically retry an uncertain purchase.
- [ ] Mock purchase tests. No live orders without separate authorization.
- [ ] Review and Boss acceptance before milestone 3.

## Milestone 3: support and refunds

- [ ] Discover provider-supported issue categories and eligibility read-only.
- [ ] Draft issues against actual order items and user-supplied evidence.
- [ ] Explicit confirmation before sending claims or accepting settlements.
- [ ] Track case status and actual provider refund decisions. Do not fabricate
      complaints or promise approval; report app-only steps when necessary.
- [ ] Mock submission tests, review and Boss acceptance.

## Discovery, 2026-10-16

Both sites expose SMS OTP login. Browser login succeeded for both using fresh
codes from `imsg`. Zepto accepted a captured authenticated read-only request
outside the browser. Blinkit returned HTTP 403 outside the browser and HTTP 200
for its authenticated order-count request inside the browser. Browser-assisted
authentication is necessary for this implementation, not OAuth.

No orders or support/refund requests are authorized during development. No cloud
resources, global installations, repository renames or releases are in scope.
