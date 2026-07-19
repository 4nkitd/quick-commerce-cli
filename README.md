# zepto

A fast command-line client for [Zepto](https://www.zepto.com/) — search products,
check live prices & stock, and view your orders straight from the terminal.

It talks to Zepto's real API directly (no browser automation at runtime), so it's
quick and works anywhere a single Go binary runs.

## Install

```bash
go install github.com/ankityadav/zepto@latest
```

Or build from source:

```bash
git clone <repo> && cd zepto
go build -o zepto .
# then move ./zepto onto your PATH, e.g.
mv zepto /usr/local/bin/
```

## Login (one time)

Zepto sits behind a WAF, so the CLI reuses your real logged-in web session.

1. Open <https://www.zepto.com>, log in and set your delivery location.
2. Open DevTools → **Network**, filter for `bff-gateway`.
3. Do any action (e.g. a search). Right-click a `bff-gateway.zepto.com`
   request → **Copy → Copy as cURL**.
4. Run:

   ```bash
   zepto login
   ```

   Paste the cURL command, then press **Ctrl-D**.

Your session is saved to `~/.config/zepto/session.json` (macOS:
`~/Library/Application Support/zepto/session.json`).

## Usage

```bash
zepto search milk          # search products with live price & stock
zepto search amul butter   # multi-word queries work
zepto orders               # list recent orders
zepto orders 20            # list last 20
zepto order PNUUJGMSJ54550 # details of one order
zepto wallet               # wallet balance
zepto whoami               # show current session
```

Example:

```
$ zepto search bread
PRODUCT                        UNIT            PRICE   MRP     STOCK
Harvest Gold White Bread       1 pack (350 g)  ₹32.00  ₹33.00  in stock
English Oven Premium White...  1 pack (700 g)  ₹56.00  ₹65.00  in stock
```

## Notes

- If you see `session expired` or `blocked (429)`, run `zepto login` again with a
  fresh request — access tokens rotate.
- Placing orders involves payment flows and is intentionally left to the app.

## How it was built

The client was derived by capturing Zepto's network traffic once, identifying the
minimal request shape each endpoint needs (headers, auth, WAF fingerprint), and
re-implementing just those calls in Go. This is far more efficient than driving a
browser for every action.
