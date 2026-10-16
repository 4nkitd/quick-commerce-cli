package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

var version = "v0.2.0"

const helpText = `Quick Commerce CLI (qc)

Usage: qc [--json] <zepto|blinkit> <command>

Both providers:
  login --phone <number> [--otp-source stdin|imsg]
  auth status           Verify saved login with a read-only provider request
  whoami                Show local session metadata, not a live auth check
  search <query>        Products, live prices, pack size and stock
  orders [n|--all]      Order history
  order <id-or-code>    Detailed order & bill breakdown
  spend [--md] [--out]  Lifetime & category spend analytics

Cross-platform commands:
  qc compare <query>                    Compare live prices across Zepto & Blinkit
  qc spend [--md] [--out <file>]        Combined spend & category analytics
  qc providers                          Implemented provider capabilities as JSON
  qc version                            Print CLI version

Zepto-only commands:
  track <code> [--watch] Order tracking
  wallet                 Wallet balance
  import-session         Legacy Copy-as-cURL import from stdin

Login requires agent-browser + Lightpanda. imsg is optional and macOS-only.
Checkout, cart sync, support and refunds are disabled in this milestone.
JSON output (--json) is supported on all read, auth and analytics commands.
`

func printBanner(_ bool) {
	fmt.Print(helpText)
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--json" {
		jsonOutput = true
		args = args[1:]
	}
	if len(args) == 0 {
		printBanner(false)
		return
	}
	if args[0] == "--version" || args[0] == "-v" || args[0] == "version" {
		if jsonOutput {
			json.NewEncoder(os.Stdout).Encode(map[string]string{"name": "qc", "version": version})
			return
		}
		fmt.Printf("qc version %s\n", version)
		return
	}
	if args[0] == "providers" {
		if len(args) != 1 {
			fail(fmt.Errorf("usage: qc providers"))
		}
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"zepto":   map[string]any{"auth": "browser_otp", "read": []string{"search", "orders", "order", "track", "wallet", "spend", "compare"}, "checkout": false, "support": false, "refunds": false},
			"blinkit": map[string]any{"auth": "browser_otp", "read": []string{"search", "orders", "order", "spend", "compare"}, "checkout": false, "support": false, "refunds": false},
		})
		return
	}
	if args[0] == "spend" {
		cmdSpend("all", args[1:])
		return
	}
	if args[0] == "compare" || args[0] == "cmp" {
		cmdCompare(args[1:])
		return
	}
	if p, err := parseProvider(args[0]); err == nil {
		activeProvider = p
		args = args[1:]
	}
	if len(args) == 0 {
		printBanner(false)
		return
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printBanner(false)
		return
	}
	if args[0] == "login" {
		if err := login(activeProvider, args[1:]); err != nil {
			fail(err)
		}
		return
	}
	if args[0] == "auth" {
		if len(args) < 2 || args[1] != "status" {
			fail(fmt.Errorf("usage: qc %s auth status", activeProvider))
		}
		if err := authStatus(activeProvider, args[2:]); err != nil {
			fail(err)
		}
		return
	}
	if args[0] == "whoami" {
		if len(args) != 1 {
			fail(fmt.Errorf("usage: qc %s whoami", activeProvider))
		}
		cmdWhoami()
		return
	}
	if args[0] == "spend" {
		cmdSpend(string(activeProvider), args[1:])
		return
	}
	if args[0] == "compare" || args[0] == "cmp" {
		cmdCompare(args[1:])
		return
	}
	if args[0] == "checkout" || args[0] == "support" || args[0] == "refund" || args[0] == "refunds" || args[0] == "cart" {
		fail(fmt.Errorf("%s is not available in this milestone; no provider action was taken", args[0]))
	}
	if activeProvider == Blinkit {
		switch args[0] {
		case "search", "s":
			cmdBlinkitSearch(args[1:])
			return
		case "orders", "o":
			cmdBlinkitOrders(args[1:])
			return
		case "order":
			cmdBlinkitOrder(args[1:])
			return
		default:
			fail(fmt.Errorf("Blinkit %s is not implemented; use login, auth status, whoami, search, orders, order or spend", args[0]))
		}
	}
	if jsonOutput && args[0] == "import-session" {
		fail(fmt.Errorf("JSON output is not implemented for %s", args[0]))
	}
	switch args[0] {
	case "import-session":
		cmdLogin()
	case "search", "s":
		cmdSearch(args[1:])
	case "orders", "o":
		cmdOrders(args[1:])
	case "order":
		cmdOrder(args[1:])
	case "track", "t":
		cmdTrack(args[1:])
	case "wallet", "w":
		cmdWallet()
	default:
		fail(fmt.Errorf("unknown command %q; try qc --help", args[0]))
	}
}

func fail(err error) {
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"error": map[string]string{"code": "command_failed", "message": err.Error()}})
	} else {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
	os.Exit(1)
}

func rupees(paise int) string {
	return fmt.Sprintf("₹%.2f", float64(paise)/100)
}

func cmdLogin() {
	unlock, err := authLock(Zepto)
	if err != nil {
		fail(err)
	}
	defer unlock()
	fmt.Fprintln(os.Stderr, "Paste your Zepto request (Copy as cURL), then press Ctrl-D:")
	data, err := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		fail(fmt.Errorf("cannot read session input, or input exceeds 1 MiB"))
	}
	s, err := parseCurl(string(data))
	if err != nil {
		fail(err)
	}
	if err := saveSession(s); err != nil {
		fail(err)
	}
	fmt.Println("session imported, not verified. Run: qc zepto auth status")
}

func client() *Client {
	s, err := loadSession()
	if err != nil {
		fail(err)
	}
	return newClient(s)
}

func cmdSearch(args []string) {
	if len(args) == 0 {
		fail(fmt.Errorf("usage: qc zepto search <query>"))
	}
	q := strings.Join(args, " ")
	c := client()
	prods, total, err := c.Search(q)
	if err != nil {
		fail(err)
	}
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"provider": activeProvider, "products": prods, "total": total, "currency": "INR", "amount_unit": "paise"})
		return
	}
	if len(prods) == 0 {
		fmt.Printf("no products found for %q\n", q)
		return
	}
	printSearchTable(prods, total, q)
}

func printSearchTable(prods []Product, total int, query string) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintf(tw, "#\tPRODUCT\tUNIT\tPRICE\tMRP\tSTOCK\n")
	for i, p := range prods {
		price := p.Selling
		if p.Discounted > 0 && p.Discounted < price {
			price = p.Discounted
		}
		stock := "in stock"
		if p.OutOfStock {
			stock = "OUT"
		}
		mrp := ""
		if p.MRP > price {
			mrp = rupees(p.MRP)
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", i+1, truncate(p.Name, 40), p.Unit, rupees(price), mrp, stock)
	}
	tw.Flush()
	fmt.Printf("\n%d shown · %d total results for %q\n", len(prods), total, query)
}

func cmdOrders(args []string) {
	n := 10
	if len(args) > 1 {
		fail(fmt.Errorf("usage: qc zepto orders [1-500|--all]"))
	}
	if len(args) > 0 {
		if args[0] == "--all" {
			n = 0
		} else {
			v, err := strconv.Atoi(args[0])
			if err != nil || v < 1 || v > 500 {
				fail(fmt.Errorf("order count must be between 1 and 500"))
			}
			n = v
		}
	}
	c := client()
	orders, err := c.Orders(n)
	if err != nil {
		fail(err)
	}
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"provider": activeProvider, "orders": orders, "currency": "INR", "amount_unit": "paise"})
		return
	}
	if len(orders) == 0 {
		fmt.Println("no orders yet")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "CODE\tDATE\tSTATUS\tITEMS\tTOTAL")
	for _, o := range orders {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", o.Code, fmtDate(o.PlacedTime), o.FormattedStatus, o.ItemCount, rupees(o.GrandTotal))
	}
	tw.Flush()
}

func cmdOrder(args []string) {
	if len(args) == 0 {
		fail(fmt.Errorf("usage: qc zepto order <code>"))
	}
	code := strings.ToUpper(args[0])
	c := client()
	orders, err := c.Orders(0)
	if err != nil {
		fail(err)
	}
	for _, o := range orders {
		if strings.EqualFold(o.Code, code) || strings.EqualFold(o.ID, code) {
			if jsonOutput {
				json.NewEncoder(os.Stdout).Encode(map[string]any{"provider": activeProvider, "order": o, "currency": "INR", "amount_unit": "paise"})
				return
			}
			fmt.Printf("Order %s\n", o.Code)
			fmt.Printf("  Placed : %s\n", fmtDate(o.PlacedTime))
			fmt.Printf("  Status : %s\n", o.FormattedStatus)
			fmt.Printf("  Items  : %d\n", o.ItemCount)
			fmt.Printf("  Total  : %s\n", rupees(o.GrandTotal))
			if o.RefundAmount > 0 {
				fmt.Printf("  Refund : %s\n", rupees(o.RefundAmount))
			}
			fmt.Println("  Products:")
			for _, p := range o.Products {
				fmt.Printf("    %2dx %s\n", p.Count, p.Name)
			}
			return
		}
	}
	fail(fmt.Errorf("order %s not found in your orders", code))
}

func cmdTrack(args []string) {
	watch := false
	var code string
	for _, a := range args {
		if a == "--watch" || a == "-w" {
			watch = true
		} else {
			code = strings.ToUpper(a)
		}
	}
	if code == "" {
		fail(fmt.Errorf("usage: qc zepto track <code> [--watch]"))
	}
	c := client()
	orders, err := c.Orders(0)
	if err != nil {
		fail(err)
	}
	var id string
	for _, o := range orders {
		if strings.EqualFold(o.Code, code) || strings.EqualFold(o.ID, code) {
			id = o.ID
			break
		}
	}
	if id == "" {
		fail(fmt.Errorf("order %s not found in your orders", code))
	}
	for {
		t, err := c.Track(id)
		if err != nil {
			fail(err)
		}
		if jsonOutput {
			json.NewEncoder(os.Stdout).Encode(map[string]any{"provider": activeProvider, "tracking": t, "currency": "INR", "amount_unit": "paise"})
			if !watch || isTerminal(t.Status) {
				return
			}
		} else {
			printTracking(t)
			if !watch || isTerminal(t.Status) {
				return
			}
			fmt.Println("\n\033[2m— refreshing —\033[0m")
		}
		time.Sleep(20 * time.Second)
	}
}

func isTerminal(status string) bool {
	s := strings.ToUpper(status)
	return strings.Contains(s, "DELIVER") || strings.Contains(s, "CANCEL")
}

func printTracking(t *Tracking) {
	fmt.Printf("Order %s\n", t.Code)
	fmt.Printf("  Status : %s\n", t.Status)
	if t.EtaMins > 0 {
		fmt.Printf("  ETA    : ~%d min\n", t.EtaMins)
	}
	fmt.Printf("  Items  : %d · %s\n", t.ItemCount, rupees(t.Total))
	if t.Address != "" {
		fmt.Printf("  To     : %s\n", truncate(t.Address, 70))
	}
	if len(t.Stages) > 0 {
		fmt.Println("  Progress:")
		for _, s := range t.Stages {
			fmt.Printf("    %-16s %s\n", s.Status, s.Title)
		}
	}
}

func cmdWallet() {
	c := client()
	bal, err := c.Wallet()
	if err != nil {
		fail(err)
	}
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(map[string]any{"provider": activeProvider, "balance_paise": bal, "currency": "INR", "amount_unit": "paise"})
		return
	}
	fmt.Printf("Wallet balance: %s\n", rupees(bal))
}

func cmdWhoami() {
	s, err := loadSession()
	if err != nil {
		fail(err)
	}
	if err = printAuth(activeProvider, s, false); err != nil {
		fail(err)
	}
}

func fmtDate(iso string) string {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		if len(iso) >= 10 {
			return iso[:10]
		}
		return iso
	}
	return t.Local().Format("02 Jan 15:04")
}
