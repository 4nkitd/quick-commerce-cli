package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

type blinkitSearchItem struct {
	ID         string  `json:"id"`
	MerchantID string  `json:"merchant_id"`
	Name       string  `json:"name"`
	Brand      string  `json:"brand"`
	Unit       string  `json:"unit"`
	PriceText  string  `json:"price_text"`
	MRPText    string  `json:"mrp_text"`
	CartPrice  float64 `json:"cart_price"`
	CartMRP    float64 `json:"cart_mrp"`
	Inventory  int     `json:"inventory"`
	IsSoldOut  bool    `json:"is_sold_out"`
}

func parseBlinkitSearchProduct(it blinkitSearchItem) Product {
	selling := parseRupeesStringToPaise(it.PriceText)
	if selling <= 0 && it.CartPrice > 0 {
		selling = int(math.Round(it.CartPrice * 100))
	}
	mrp := parseRupeesStringToPaise(it.MRPText)
	if mrp <= 0 && it.CartMRP > 0 {
		mrp = int(math.Round(it.CartMRP * 100))
	}
	if mrp < selling {
		mrp = selling
	}
	oos := it.IsSoldOut || it.Inventory == 0
	return Product{
		Name:        strings.TrimSpace(it.Name),
		Brand:       strings.TrimSpace(it.Brand),
		MRP:         mrp,
		Selling:     selling,
		Discounted:  selling,
		OutOfStock:  oos,
		VariantID:   it.ID,
		StoreProdID: it.MerchantID,
		Unit:        strings.TrimSpace(it.Unit),
	}
}

func searchBlinkitProducts(query string) ([]Product, int, error) {
	prev := activeProvider
	activeProvider = Blinkit
	defer func() { activeProvider = prev }()
	unlock, err := authLock(Blinkit)
	if err != nil {
		return nil, 0, err
	}
	defer unlock()
	s, err := loadSession()
	if err != nil {
		return nil, 0, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	b, err := newBrowser(ctx, Blinkit)
	if err != nil {
		return nil, 0, err
	}
	defer b.close()
	if err = b.restore(s); err != nil {
		return nil, 0, err
	}
	if err = b.verify(s); err != nil {
		return nil, 0, err
	}
	lat := firstNonEmpty(s.Latitude, cookieValue(s.Cookies, "gr_1_lat"))
	lon := firstNonEmpty(s.Longitude, cookieValue(s.Cookies, "gr_1_lon"))
	ep := "/v1/layout/search?q=" + url.QueryEscape(query)
	var items []blinkitSearchItem
	script := fmt.Sprintf(`(async()=>{
		const a=JSON.parse(localStorage.getItem("auth")||"{}");
		const loc=JSON.parse(localStorage.getItem("location")||"{}");
		const coords=loc.coords||{};
		const lat=String(%q||coords.lat||window.grofers?.CONFIG?.defaultLat||"");
		const lon=String(%q||coords.lon||window.grofers?.CONFIG?.defaultLon||"");
		if(!lat || !lon)throw new Error("missing_blinkit_location");
		const cfg=window.grofers.CONFIG;
		const headers={
			"Content-Type":"application/json",
			access_token:a.accessToken,
			auth_key:localStorage.getItem("authKey"),
			app_client:cfg.appClient,
			app_version:cfg.appVersion,
			device_id:localStorage.getItem("deviceId"),
			rn_bundle_version:"1009003012",
			web_app_version:"1008010016",
			platform:"desktop_web",
			lat,
			lon
		};
		const r=await fetch(%q,{method:"POST",headers,body:JSON.stringify({})});
		if(!r.ok)throw new Error("search_http_"+r.status);
		const d=await r.json();
		const out=[];
		for(const sn of (d.response?.snippets||[])){
			const sd=sn.data;
			if(!sd)continue;
			const ci=sd.atc_action?.add_to_cart?.cart_item||{};
			const name=sd.name?.text||sd.display_name?.text||ci.display_name||ci.product_name||"";
			const priceText=sd.normal_price?.text||"";
			if(!name || (!priceText && !ci.price))continue;
			const inv=Number(sd.inventory??ci.inventory??1);
			out.push({
				id:String(sd.product_id||sd.identity?.id||ci.product_id||""),
				merchant_id:String(sd.merchant_id||ci.merchant_id||""),
				name,
				brand:sd.brand_name?.text||ci.brand||"",
				unit:sd.variant?.text||ci.unit||"",
				price_text:priceText,
				mrp_text:sd.mrp?.text||"",
				cart_price:Number(ci.price||0),
				cart_mrp:Number(ci.mrp||0),
				inventory:inv,
				is_sold_out:Boolean(sd.is_sold_out || inv===0 || sd.product_state==="out_of_stock")
			});
		}
		return out;
	})()`, lat, lon, ep)
	if err = b.evalWithTimeout(45*time.Second, script, &items); err != nil {
		return nil, 0, err
	}
	var out []Product
	seen := map[string]bool{}
	for _, it := range items {
		p := parseBlinkitSearchProduct(it)
		if p.Name == "" || p.effectivePrice() <= 0 {
			continue
		}
		key := p.VariantID
		if key == "" {
			key = p.Name + "|" + p.Unit
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out, len(out), nil
}

type CompareMatch struct {
	Provider       Provider `json:"provider"`
	Name           string   `json:"name"`
	Brand          string   `json:"brand,omitempty"`
	Unit           string   `json:"unit"`
	PricePaise     int      `json:"price_paise"`
	MRPPaise       int      `json:"mrp_paise"`
	OutOfStock     bool     `json:"out_of_stock"`
	RelevanceScore float64  `json:"relevance_score"`
}

type CompareResult struct {
	Query           string        `json:"query"`
	Currency        string        `json:"currency"`
	AmountUnit      string        `json:"amount_unit"`
	CheaperProvider string        `json:"cheaper_provider"`
	PriceDiffPaise  int           `json:"price_diff_paise"`
	ZeptoBest       *CompareMatch `json:"zepto_best,omitempty"`
	BlinkitBest     *CompareMatch `json:"blinkit_best,omitempty"`
	ZeptoTop        []Product     `json:"zepto_top"`
	BlinkitTop      []Product     `json:"blinkit_top"`
	Recommendation  string        `json:"recommendation"`
}

func tokenizeQuery(s string) []string {
	s = strings.ToLower(s)
	repl := strings.NewReplacer("(", " ", ")", " ", "|", " ", "-", " ", ",", " ", "/", " ", ".", " ")
	s = repl.Replace(s)
	var out []string
	for _, tok := range strings.Fields(s) {
		if len(tok) >= 2 || (len(tok) == 1 && tok[0] >= '0' && tok[0] <= '9') {
			out = append(out, tok)
		}
	}
	return out
}

func scoreProductForQuery(query string, p Product, rank int) float64 {
	qTokens := tokenizeQuery(query)
	if len(qTokens) == 0 {
		return 0
	}
	hay := strings.ToLower(p.Name + " " + p.Brand + " " + p.Unit)
	matched := 0
	for _, tok := range qTokens {
		if strings.Contains(hay, tok) {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	score := float64(matched) / float64(len(qTokens))
	score -= float64(rank) * 0.02
	if score < 0.01 {
		score = 0.01
	}
	return math.Round(score*1000) / 1000
}

func pickBestMatch(provider Provider, query string, prods []Product) *CompareMatch {
	var bestInStock *CompareMatch
	var bestOOS *CompareMatch
	for i, p := range prods {
		if i >= 10 {
			break
		}
		sc := scoreProductForQuery(query, p, i)
		if sc <= 0 {
			continue
		}
		cand := &CompareMatch{
			Provider:       provider,
			Name:           p.Name,
			Brand:          p.Brand,
			Unit:           p.Unit,
			PricePaise:     p.effectivePrice(),
			MRPPaise:       p.MRP,
			OutOfStock:     p.OutOfStock,
			RelevanceScore: sc,
		}
		if !p.OutOfStock {
			if bestInStock == nil || cand.RelevanceScore > bestInStock.RelevanceScore {
				bestInStock = cand
			}
		} else if bestOOS == nil || cand.RelevanceScore > bestOOS.RelevanceScore {
			bestOOS = cand
		}
	}
	if bestInStock != nil {
		return bestInStock
	}
	return bestOOS
}

func buildComparison(query string, zProds, bProds []Product) CompareResult {
	if len(zProds) > 5 {
		zProds = zProds[:5]
	}
	if len(bProds) > 5 {
		bProds = bProds[:5]
	}
	res := CompareResult{
		Query:       query,
		Currency:    "INR",
		AmountUnit:  "paise",
		ZeptoTop:    zProds,
		BlinkitTop:  bProds,
		ZeptoBest:   pickBestMatch(Zepto, query, zProds),
		BlinkitBest: pickBestMatch(Blinkit, query, bProds),
	}
	zb, bb := res.ZeptoBest, res.BlinkitBest
	switch {
	case zb != nil && bb != nil && !zb.OutOfStock && !bb.OutOfStock:
		diff := bb.PricePaise - zb.PricePaise
		switch {
		case diff > 0:
			res.CheaperProvider = string(Zepto)
			res.PriceDiffPaise = diff
			res.Recommendation = fmt.Sprintf("Zepto is cheaper by %s (%s vs %s); verify pack size (%s vs %s)", rupees(diff), rupees(zb.PricePaise), rupees(bb.PricePaise), zb.Unit, bb.Unit)
		case diff < 0:
			res.CheaperProvider = string(Blinkit)
			res.PriceDiffPaise = -diff
			res.Recommendation = fmt.Sprintf("Blinkit is cheaper by %s (%s vs %s); verify pack size (%s vs %s)", rupees(-diff), rupees(bb.PricePaise), rupees(zb.PricePaise), bb.Unit, zb.Unit)
		default:
			res.CheaperProvider = "tie"
			res.Recommendation = fmt.Sprintf("Identical price (%s) on both platforms", rupees(zb.PricePaise))
		}
	case zb != nil && !zb.OutOfStock:
		res.CheaperProvider = string(Zepto)
		res.Recommendation = "Available in stock on Zepto only"
	case bb != nil && !bb.OutOfStock:
		res.CheaperProvider = string(Blinkit)
		res.Recommendation = "Available in stock on Blinkit only"
	default:
		res.CheaperProvider = "none"
		res.Recommendation = "No in-stock match found"
	}
	return res
}

func printCompareTable(res CompareResult) {
	fmt.Printf("Comparison for %q:\n", res.Query)
	if res.Recommendation != "" {
		fmt.Printf("  Verdict: %s\n\n", res.Recommendation)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PLATFORM\t#\tPRODUCT\tUNIT\tPRICE\tMRP\tSTOCK")
	for i, p := range res.BlinkitTop {
		stock := "in stock"
		if p.OutOfStock {
			stock = "OUT"
		}
		mrp := ""
		if p.MRP > p.effectivePrice() {
			mrp = rupees(p.MRP)
		}
		fmt.Fprintf(tw, "blinkit\t%d\t%s\t%s\t%s\t%s\t%s\n", i+1, truncate(p.Name, 42), p.Unit, rupees(p.effectivePrice()), mrp, stock)
	}
	for i, p := range res.ZeptoTop {
		stock := "in stock"
		if p.OutOfStock {
			stock = "OUT"
		}
		mrp := ""
		if p.MRP > p.effectivePrice() {
			mrp = rupees(p.MRP)
		}
		fmt.Fprintf(tw, "zepto\t%d\t%s\t%s\t%s\t%s\t%s\n", i+1, truncate(p.Name, 42), p.Unit, rupees(p.effectivePrice()), mrp, stock)
	}
	tw.Flush()
}

func cmdCompare(args []string) {
	if len(args) == 0 {
		fail(fmt.Errorf("usage: qc [--json] compare <query>"))
	}
	query := strings.Join(args, " ")
	activeProvider = Zepto
	zProds, _, zErr := client().Search(query)
	if zErr != nil {
		fail(zErr)
	}
	bProds, _, bErr := searchBlinkitProducts(query)
	if bErr != nil {
		fail(bErr)
	}
	res := buildComparison(query, zProds, bProds)
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(res)
		return
	}
	printCompareTable(res)
}

func cmdBlinkitSearch(args []string) {
	if len(args) == 0 {
		fail(fmt.Errorf("usage: qc [--json] blinkit search <query>"))
	}
	q := strings.Join(args, " ")
	prods, total, err := searchBlinkitProducts(q)
	if err != nil {
		fail(err)
	}
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"provider":    Blinkit,
			"products":    prods,
			"total":       total,
			"currency":    "INR",
			"amount_unit": "paise",
		})
		return
	}
	if len(prods) == 0 {
		fmt.Printf("no products found for %q\n", q)
		return
	}
	printSearchTable(prods, total, q)
}

func findBlinkitOrder(codeOrID string, refresh bool) (*SpendOrder, error) {
	target := strings.TrimSpace(codeOrID)
	if !refresh {
		if cached, err := loadSpendCache(Blinkit); err == nil {
			for i := range cached {
				if strings.EqualFold(cached[i].ID, target) || strings.EqualFold(cached[i].Code, target) {
					return &cached[i], nil
				}
			}
		}
	}
	orders, err := fetchBlinkitSpendOrders(refresh, 50)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		if strings.EqualFold(orders[i].ID, target) || strings.EqualFold(orders[i].Code, target) {
			return &orders[i], nil
		}
	}
	return nil, fmt.Errorf("order %s not found in Blinkit history", codeOrID)
}

func cmdBlinkitOrder(args []string) {
	if len(args) == 0 {
		fail(fmt.Errorf("usage: qc [--json] blinkit order <id-or-code> [--refresh]"))
	}
	refresh := false
	var target string
	for _, a := range args {
		if a == "--refresh" {
			refresh = true
		} else {
			target = a
		}
	}
	if target == "" {
		fail(fmt.Errorf("usage: qc [--json] blinkit order <id-or-code> [--refresh]"))
	}
	o, err := findBlinkitOrder(target, refresh)
	if err != nil {
		fail(err)
	}
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"provider":    Blinkit,
			"order":       o,
			"currency":    "INR",
			"amount_unit": "paise",
		})
		return
	}
	fmt.Printf("Order %s (ID: %s)\n", o.Code, o.ID)
	fmt.Printf("  Placed  : %s\n", o.PlacedAt)
	fmt.Printf("  Status  : %s\n", o.Status)
	if o.PaymentMethod != "" {
		fmt.Printf("  Payment : %s\n", o.PaymentMethod)
	}
	fmt.Printf("  Items   : %d\n", o.ItemCount)
	fmt.Printf("  Total   : %s\n", rupees(o.GrandTotalPaise))
	if o.RefundPaise > 0 {
		fmt.Printf("  Refund  : %s (%s)\n", rupees(o.RefundPaise), o.RefundMethod)
	}
	fmt.Printf("  Net     : %s\n", rupees(o.NetSpentPaise))
	fmt.Println("  Products:")
	for _, it := range o.Items {
		fmt.Printf("    %2dx %-42s [%s]  %s  (%s)\n", it.Quantity, truncate(it.Name, 42), it.Unit, rupees(it.GrossPaise), it.Category)
	}
}
