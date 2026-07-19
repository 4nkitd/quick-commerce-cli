package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type SavedCart struct {
	CartID string     `json:"cart_id"`
	Items  []CartLine `json:"items"`
	Cache  []Product  `json:"cache"`
}

func cartFilePath() string {
	return filepath.Join(filepath.Dir(configPath()), "cart.json")
}

func loadCart() *SavedCart {
	b, err := os.ReadFile(cartFilePath())
	if err != nil {
		return &SavedCart{}
	}
	var c SavedCart
	json.Unmarshal(b, &c)
	return &c
}

func saveCart(sc *SavedCart) error {
	os.MkdirAll(filepath.Dir(cartFilePath()), 0o700)
	b, _ := json.MarshalIndent(sc, "", "  ")
	return os.WriteFile(cartFilePath(), b, 0o600)
}

func cmdCart(args []string, c *Client) {
	if len(args) == 0 {
		cmdCartList(c)
		return
	}
	switch args[0] {
	case "add", "a":
		cmdCartAdd(args[1:], c)
	case "remove", "rm", "r":
		cmdCartRemove(args[1:], c)
	case "list", "ls", "l":
		cmdCartList(c)
	case "clean", "clear":
		cmdCartClean(c)
	default:
		fail(fmt.Errorf("unknown cart subcommand: %s. Use add/remove/list/clean", args[0]))
	}
}

func cmdCartAdd(args []string, c *Client) {
	if len(args) == 0 {
		fail(fmt.Errorf("usage: zepto cart add <search-query>  OR  zepto cart add <index> [qty]"))
	}
	sc := loadCart()
	qty := 1
	rest := args
	if len(args) >= 2 {
		if v, err := fmt.Sscanf(args[len(args)-1], "%d", &qty); v != 1 || err != nil {
			qty = 1
		} else {
			rest = args[:len(args)-1]
		}
	}
	query := strings.Join(rest, " ")
	idx := -1
	if _, err := fmt.Sscanf(query, "%d", &idx); err == nil && idx > 0 {
		if idx > len(sc.Cache) || idx < 1 {
			fail(fmt.Errorf("index %d out of range (cached search has %d items). Run `zepto search` first", idx, len(sc.Cache)))
		}
		p := sc.Cache[idx-1]
		addToCart(sc, p, qty)
	} else {
		prods, total, err := c.Search(query)
		if err != nil {
			fail(err)
		}
		if len(prods) == 0 {
			fmt.Printf("no results for %q\n", query)
			return
		}
		sc.Cache = prods
		printSearchTable(prods, total, query)
		fmt.Print("pick one (1-" + fmt.Sprint(len(prods)) + "): ")
		var pick int
		fmt.Scanf("%d", &pick)
		if pick < 1 || pick > len(prods) {
			fmt.Println("cancelled")
			return
		}
		addToCart(sc, prods[pick-1], qty)
	}
	saveCart(sc)
	printCartBill(sc, c)
}

func addToCart(sc *SavedCart, p Product, qty int) {
	for i, it := range sc.Items {
		if it.VariantID == p.VariantID {
			sc.Items[i].Qty += qty
			return
		}
	}
	sc.Items = append(sc.Items, CartLine{
		Index: len(sc.Items), Name: p.Name, Unit: p.Unit,
		StoreID: p.StoreProdID, VariantID: p.VariantID,
		MRP: p.MRP, Qty: qty,
	})
}

func cmdCartRemove(args []string, c *Client) {
	if len(args) == 0 {
		fail(fmt.Errorf("usage: zepto cart remove <index>"))
	}
	var idx int
	fmt.Sscanf(args[0], "%d", &idx)
	if idx < 1 {
		fail(fmt.Errorf("invalid index"))
	}
	sc := loadCart()
	if idx > len(sc.Items) {
		fail(fmt.Errorf("cart has only %d items", len(sc.Items)))
	}
	sc.Items = append(sc.Items[:idx-1], sc.Items[idx:]...)
	for i := range sc.Items {
		sc.Items[i].Index = i + 1
	}
	saveCart(sc)
	printCartBill(sc, c)
}

func cmdCartList(c *Client) {
	sc := loadCart()
	printCartBill(sc, c)
}

func cmdCartClean(c *Client) {
	sc := &SavedCart{}
	saveCart(sc)
	if _, err := c.CartSync(nil); err == nil {
		fmt.Println("cart cleared")
	} else {
		fmt.Println("cart cleared (sync failed, but local state emptied)")
	}
}

func printCartBill(sc *SavedCart, c *Client) {
	if len(sc.Items) == 0 {
		fmt.Println("cart is empty")
		return
	}
	bill, err := c.CartSync(sc.Items)
	if err != nil {
		fmt.Println("(prices may be stale — sync failed:", err, ")")
		bill = &CartBill{Lines: sc.Items}
		for _, it := range sc.Items {
			bill.ItemTotal += it.MRP * it.Qty
		}
	}
	fmt.Printf("Cart (%d items):\n", len(bill.Lines))
	for i, it := range bill.Lines {
		it.Index = i + 1
		fmt.Printf("  %d. %s  [%s]  %d x %s = %s\n", it.Index, truncate(it.Name, 35), it.Unit, it.Qty, rupees(it.MRP), rupees(it.MRP*it.Qty))
	}
	fmt.Printf("  ─────────────────────\n")
	fmt.Printf("  Item total : %s\n", rupees(bill.ItemTotal))
	if bill.DeliveryFee > 0 {
		fmt.Printf("  Delivery   : %s\n", rupees(bill.DeliveryFee))
	}
	fmt.Printf("  To Pay     : %s\n", rupees(bill.GrandTotal))
	sc.Items = bill.Lines
	saveCart(sc)
}
