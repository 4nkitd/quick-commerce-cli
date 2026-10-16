package main

import (
	"strings"
	"testing"
)

func TestParseRupeesStringToPaise(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"₹209", 20900},
		{"₹2,447.50", 244750},
		{"~~<regular-200|{grey-600|₹2,999}>~~ ₹673", 67300},
		{"-₹280.99", -28099},
		{"FREE", 0},
		{"05 Jul, 4:54 pm", 0},
	}
	for _, tc := range cases {
		if got := parseRupeesStringToPaise(tc.in); got != tc.want {
			t.Fatalf("parseRupeesStringToPaise(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestClassifyItem(t *testing.T) {
	cases := []struct {
		name   string
		subcat string
		want   Category
	}{
		{"Amul Cow Milk", "", CatFood},
		{"McCain Air Fryer French Fries", "", CatFood},
		{"Fuel One Whey Max Protein Powder", "", CatFood},
		{"Unigen Multi Port GaN USB Type C charger (65 W, White)", "", CatElectronics},
		{"SanDisk Ultra Dual Pen Drive (64 GB, Black)", "", CatElectronics},
		{"Portronics Vayu 3.0 Tyre Inflator with 150 PSI Pressure, 4000 mAh Battery", "", CatAuto},
		{"Philips Home Air Fryer (1500W, NA120/00, Black, 4.2 ltrs)", "", CatHome},
		{"Pampers Active Baby Diaper (New Born, Upto 5 kg)", "", CatPersonalBaby},
		{"Pedigree Adult Dog Food - Chicken & Vegetable", "", CatPets},
		{"Cosco Dribble Basketball", "", CatSportsToys},
		{"The Complete Novel Of Sherlock Holmes", "", CatStationery},
		{"Gods 15.6 Inch Polyester Laptop Backpack (28 ltrs, 30 x 14 x 53 cm, Black)", "", CatApparel},
		{"Any Item", "142676b2-d759-4f6c-90e9-4fb21d5ad3fe", CatElectronics},
	}
	for _, tc := range cases {
		if got := classifyItem(tc.name, tc.subcat); got != tc.want {
			t.Fatalf("classifyItem(%q, %q) = %q, want %q", tc.name, tc.subcat, got, tc.want)
		}
	}
}

func TestBlinkitReturnAndRefundAccounting(t *testing.T) {
	retEntry := parseBlinkitDetail(blinkitDetailPayload{
		ID:           "687954168",
		CartID:       "1097202050",
		Status:       "DELIVERED",
		CardTitle:    "Return completed",
		CardAmount:   "30 Nov 2024",
		SummaryTitle: "Return order summary",
		Placed:       "placed on Sat, 30 Nov'24, 12:36 AM",
		Items: []struct {
			Name  string `json:"name"`
			Qty   string `json:"qty"`
			Price string `json:"price"`
		}{{Name: "Ailkin USB-C Superfast Charging Adapter with Cable (Black)", Qty: "1 unit x 1", Price: "₹673"}},
	})
	if !retEntry.IsReturnRecord || retEntry.NetSpentPaise != 0 {
		t.Fatalf("return entry should have 0 net spend: %+v", retEntry)
	}

	origRefunded := parseBlinkitDetail(blinkitDetailPayload{
		ID:           "687946756",
		CartID:       "1097202050",
		Status:       "DELIVERED",
		CardTitle:    "Arrived in 9 minutes",
		CardAmount:   "₹694",
		SummaryTitle: "Order summary",
		Placed:       "placed on Sat, 30 Nov'24, 12:13 AM",
		RefundHeader: "Refund of ₹694 sent to your original source",
		Items: []struct {
			Name  string `json:"name"`
			Qty   string `json:"qty"`
			Price string `json:"price"`
		}{{Name: "Ailkin USB-C Superfast Charging Adapter with Cable (Black)", Qty: "1 unit x 1", Price: "₹673"}},
		Bills: []struct {
			Label string `json:"label"`
			Val   string `json:"val"`
		}{
			{Label: "Item total", Val: "₹673"},
			{Label: "Handling charge", Val: "+₹5"},
			{Label: "Delivery charges", Val: "+₹16"},
			{Label: "Bill total", Val: "₹694"},
		},
	})
	if origRefunded.GrandTotalPaise != 69400 || origRefunded.RefundPaise != 69400 || origRefunded.NetSpentPaise != 0 {
		t.Fatalf("refunded order accounting wrong: %+v", origRefunded)
	}

	delivered := parseBlinkitDetail(blinkitDetailPayload{
		ID:           "3863355637",
		CartID:       "4080174999",
		Status:       "DELIVERED",
		CardTitle:    "Arrived in 7 minutes",
		CardAmount:   "₹209",
		SummaryTitle: "Order summary",
		Placed:       "placed on Wed, 14 Oct'26, 2:58 PM",
		Items: []struct {
			Name  string `json:"name"`
			Qty   string `json:"qty"`
			Price string `json:"price"`
		}{
			{Name: "Amul Cow Milk", Qty: "500 ml x 1", Price: "₹31"},
			{Name: "Red Bull Drink (Sugar Free) - 250 ml", Qty: "250 ml x 1", Price: "₹125"},
			{Name: "Amul Buffalo A2 Milk", Qty: "500 ml x 1", Price: "₹40"},
		},
		Bills: []struct {
			Label string `json:"label"`
			Val   string `json:"val"`
		}{
			{Label: "MRP", Val: "₹196"},
			{Label: "Handling charge", Val: "+₹13"},
			{Label: "Bill total", Val: "₹209"},
		},
	})
	rep := buildCombinedSpendReport(map[Provider][]SpendOrder{
		Blinkit: {retEntry, origRefunded, delivered},
	}, false)
	if rep.CombinedGrossDelivered != 90300 || rep.CombinedOriginalRefunds != 69400 || rep.CombinedNetSpentPaise != 20900 {
		t.Fatalf("unexpected combined report totals: %+v", rep)
	}
	md := renderSpendMarkdown(rep)
	if !strings.Contains(md, "```mermaid\npie showData") || !strings.Contains(md, "xychart-beta") {
		t.Fatalf("markdown report missing mermaid charts:\n%s", md)
	}
}

func TestValidateReportOutputPath(t *testing.T) {
	if _, err := validateReportOutputPath("SPENDING-REPORT.md"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/etc/passwd.md", "../outside.md", "notes.txt", "."} {
		if _, err := validateReportOutputPath(bad); err == nil {
			t.Fatalf("accepted bad output path %q", bad)
		}
	}
}

func TestPartialRefundAndNonDeliveredHandling(t *testing.T) {
	partial := SpendOrder{
		Provider:        Zepto,
		ID:              "o1",
		Status:          "DELIVERED",
		PlacedAt:        "2026-07-19 10:00",
		YearMonth:       "2026-07",
		GrandTotalPaise: 51100,
		ItemTotalPaise:  49900,
		FeesPaise:       1200,
		RefundPaise:     20000,
		NetSpentPaise:   31100,
		Items: []SpendLineItem{
			{Name: "Amul Cow Milk", Quantity: 1, GrossPaise: 49900},
		},
	}
	allocateItemSpend(&partial)
	if partial.Items[0].AllocatedNetPaise != 29900 {
		t.Fatalf("expected item net 29900 after 20000 refund, got %d", partial.Items[0].AllocatedNetPaise)
	}
	inTransit := SpendOrder{
		Provider:        Zepto,
		ID:              "o2",
		Status:          "PROCESSING",
		PlacedAt:        "bad",
		GrandTotalPaise: 99900,
		NetSpentPaise:   0,
	}
	ps := summarizeProviderSpend(Zepto, []SpendOrder{partial, inTransit}, false)
	if ps.DeliveredOrders != 1 || ps.ItemSubtotalNetPaise != 29900 || ps.FeesNetPaise != 1200 {
		t.Fatalf("unexpected summary: %+v", ps)
	}
}

func TestBlinkitSearchAndCompare(t *testing.T) {
	p := parseBlinkitSearchProduct(blinkitSearchItem{
		ID:         "333324",
		MerchantID: "38078",
		Name:       "Aashirvaad Shudh Chakki Atta (100% Atta, 0% Maida)",
		Brand:      "Aashirvaad",
		Unit:       "10 kg",
		PriceText:  "₹457",
		MRPText:    "₹499",
		Inventory:  2,
	})
	if p.Selling != 45700 || p.MRP != 49900 || p.OutOfStock || p.VariantID != "333324" {
		t.Fatalf("unexpected parsed Blinkit product: %+v", p)
	}
	z := Product{Name: "Aashirvaad Shudh Chakki Atta | Pure Atta 0% Maida", Brand: "Aashirvaad", Unit: "1 pack (10 kg)", Selling: 45600, Discounted: 45600, MRP: 49900}
	cmp := buildComparison("Aashirvaad Shudh Chakki Atta 10 kg", []Product{z}, []Product{p})
	if cmp.CheaperProvider != "zepto" || cmp.PriceDiffPaise != 100 || cmp.ZeptoBest == nil || cmp.BlinkitBest == nil {
		t.Fatalf("unexpected comparison result: %+v", cmp)
	}
	oosExact := Product{Name: "Aashirvaad Shudh Chakki Atta 10 kg", Unit: "10 kg", Selling: 40000, OutOfStock: true}
	inStockMatch := Product{Name: "Aashirvaad Shudh Chakki Atta", Unit: "10 kg", Selling: 45600, OutOfStock: false}
	picked := pickBestMatch(Zepto, "Aashirvaad Shudh Chakki Atta 10 kg", []Product{oosExact, inStockMatch})
	if picked == nil || picked.OutOfStock || picked.PricePaise != 45600 {
		t.Fatalf("expected in-stock match to win over OOS item: %+v", picked)
	}
	if unrelated := pickBestMatch(Zepto, "Aashirvaad Atta", []Product{{Name: "Banana", Unit: "3 pcs", Selling: 3000}}); unrelated != nil {
		t.Fatalf("expected nil match for zero-token overlap, got %+v", unrelated)
	}
}
