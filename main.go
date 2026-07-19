package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const banner = `  ▸ groceries delivered in ~10 minutes, from your terminal.
`

const cmdList = `Commands:
  zepto login            Connect your Zepto account
  zepto search <query>   Find products — live prices, pack size & stock
  zepto cart             Add items, change qty, see your bill
  zepto checkout         Place your order (COD, UPI)
  zepto orders [n]       Your order history
  zepto order <code>     Details of one order
  zepto track <code>     Live tracking — status, ETA, per-item progress
  zepto wallet           Your wallet balance
  zepto whoami           Show which account you're using
`

const loginHelp = `Getting started:
  1. Open https://www.zepto.com in your browser — log in, set your delivery address.
  2. DevTools → Network tab → filter "bff-gateway"
  3. Right-click any request → Copy → Copy as cURL
  4. Run: zepto login   → paste the cURL → press Ctrl-D
`

func printBanner(loggedIn bool) {
	if loggedIn {
		fmt.Print(banner)
		fmt.Print(cmdList)
	} else {
		fmt.Println("zepto — order groceries from your terminal")
		fmt.Print(banner)
		fmt.Print(loginHelp)
	}
}

func main() {
	if len(os.Args) < 2 {
		_, loggedIn := loadSession()
		printBanner(loggedIn == nil)
		return
	}
	switch os.Args[1] {
	case "login":
		cmdLogin()
	case "search", "s":
		cmdSearch(os.Args[2:])
	case "orders", "o":
		cmdOrders(os.Args[2:])
	case "order":
		cmdOrder(os.Args[2:])
	case "track", "t":
		cmdTrack(os.Args[2:])
	case "wallet", "w":
		cmdWallet()
	case "cart":
		cmdCart(os.Args[2:], client())
	case "whoami":
		cmdWhoami()
	case "-h", "--help", "help":
		_, loggedIn := loadSession()
		printBanner(loggedIn == nil)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q — try %q\n", os.Args[1], "zepto --help")
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func rupees(paise int) string {
	return fmt.Sprintf("₹%.2f", float64(paise)/100)
}

func cmdLogin() {
	fmt.Fprintln(os.Stderr, "Paste your Zepto request (Copy as cURL), then press Ctrl-D:")
	data, _ := io.ReadAll(os.Stdin)
	s, err := parseCurl(string(data))
	if err != nil {
		fail(err)
	}
	if err := saveSession(s); err != nil {
		fail(err)
	}
	fmt.Println("session saved. try: zepto search milk")
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
		fail(fmt.Errorf("usage: zepto search <query>"))
	}
	q := strings.Join(args, " ")
	c := client()
	prods, total, err := c.Search(q)
	if err != nil {
		fail(err)
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
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil {
			n = v
		}
	}
	c := client()
	orders, err := c.Orders(n)
	if err != nil {
		fail(err)
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
		fail(fmt.Errorf("usage: zepto order <code>"))
	}
	code := strings.ToUpper(args[0])
	c := client()
	orders, err := c.Orders(50)
	if err != nil {
		fail(err)
	}
	for _, o := range orders {
		if strings.EqualFold(o.Code, code) {
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
	fail(fmt.Errorf("order %s not found in your recent orders", code))
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
		fail(fmt.Errorf("usage: zepto track <code> [--watch]"))
	}
	c := client()
	orders, err := c.Orders(50)
	if err != nil {
		fail(err)
	}
	var id string
	for _, o := range orders {
		if strings.EqualFold(o.Code, code) {
			id = o.ID
			break
		}
	}
	if id == "" {
		fail(fmt.Errorf("order %s not found in your recent orders", code))
	}
	for {
		t, err := c.Track(id)
		if err != nil {
			fail(err)
		}
		printTracking(t)
		if !watch || isTerminal(t.Status) {
			return
		}
		time.Sleep(20 * time.Second)
		fmt.Println("\n\033[2m— refreshing —\033[0m")
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
	fmt.Printf("Wallet balance: %s\n", rupees(bal))
}

func cmdWhoami() {
	s, err := loadSession()
	if err != nil {
		fail(err)
	}
	fmt.Printf("session file : %s\n", configPath())
	fmt.Printf("device_id    : %s\n", s.DeviceID)
	fmt.Printf("session_id   : %s\n", s.SessionID)
	fmt.Printf("store_id     : %s\n", s.StoreID)
	if s.Latitude != "" {
		fmt.Printf("location     : %s, %s\n", s.Latitude, s.Longitude)
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
