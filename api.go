package main

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type Product struct {
	Name        string `json:"name"`
	Brand       string `json:"brand"`
	MRP         int    `json:"mrp"`
	Selling     int    `json:"selling"`
	Discounted  int    `json:"discounted"`
	OutOfStock  bool   `json:"out_of_stock"`
	VariantID   string `json:"variant_id"`
	StoreProdID string `json:"store_prod_id"`
	Unit        string `json:"unit"`
}

func (p Product) effectivePrice() int {
	if p.Discounted > 0 && (p.Selling == 0 || p.Discounted < p.Selling) {
		return p.Discounted
	}
	return p.Selling
}

type searchResp struct {
	Layout []struct {
		WidgetName string          `json:"widgetName"`
		Data       json.RawMessage `json:"data"`
	} `json:"layout"`
	TotalProductCount int `json:"totalProductCount"`
}

// Search returns products for a query.
func (c *Client) Search(query string) ([]Product, int, error) {
	body, _ := json.Marshal(map[string]any{
		"query":         query,
		"pageNumber":    0,
		"intentId":      uuidv4(),
		"mode":          "AUTOSUGGEST",
		"userSessionId": c.s.SessionID,
	})
	b, err := c.do("POST", "/user-search-service/api/v3/search", body)
	if err != nil {
		return nil, 0, err
	}
	var sr searchResp
	if err := json.Unmarshal(b, &sr); err != nil {
		return nil, 0, err
	}
	var out []Product
	seen := map[string]bool{}
	for _, w := range sr.Layout {
		walkProducts(w.Data, func(p Product) {
			key := p.VariantID
			if key == "" {
				key = p.Name
			}
			if seen[key] {
				return
			}
			seen[key] = true
			out = append(out, p)
		})
	}
	return out, sr.TotalProductCount, nil
}

// walkProducts recursively hunts for productResponse nodes.
func walkProducts(raw json.RawMessage, cb func(Product)) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return
	}
	var rec func(x any)
	rec = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			if pr, ok := t["productResponse"].(map[string]any); ok {
				cb(parseProduct(pr))
				return
			}
			for _, val := range t {
				rec(val)
			}
		case []any:
			for _, val := range t {
				rec(val)
			}
		}
	}
	rec(v)
}

func parseProduct(pr map[string]any) Product {
	p := Product{}
	p.MRP = toInt(pr["mrp"])
	p.Selling = toInt(pr["sellingPrice"])
	p.Discounted = toInt(pr["discountedSellingPrice"])
	p.OutOfStock, _ = pr["outOfStock"].(bool)
	p.VariantID, _ = pr["id"].(string)
	if prod, ok := pr["product"].(map[string]any); ok {
		p.Name, _ = prod["name"].(string)
		p.Brand, _ = prod["brand"].(string)
	}
	if pv, ok := pr["productVariant"].(map[string]any); ok {
		p.StoreProdID, _ = pv["id"].(string)
		if f, ok := pv["formattedPacksize"].(string); ok {
			p.Unit = f
		}
	}
	return p
}

func toInt(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return 0
}

type Order struct {
	Code            string          `json:"code"`
	Status          string          `json:"status"`
	FormattedStatus string          `json:"formattedStatus"`
	ItemCount       int             `json:"itemQuantityCount"`
	GrandTotal      int             `json:"grandTotalAmount"`
	RefundAmount    int             `json:"totalRefundAmount"`
	PlacedTime      string          `json:"placedTime"`
	ID              string          `json:"id"`
	StatusTitle     json.RawMessage `json:"statusTitle,omitempty"`
	ArrivalMins     int             `json:"totalArrivalTimeInMinutes"`
	ArrivedTime     string          `json:"arrivedTime"`
	Products        []struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	} `json:"productsNamesAndCounts"`
}

type ordersResp struct {
	Orders []Order `json:"orders"`
}

// Tracking is the live view of a single order.
type Tracking struct {
	Code      string       `json:"code"`
	Status    string       `json:"status"`
	EtaMins   int          `json:"eta_mins,omitempty"`
	ItemCount int          `json:"item_count"`
	Total     int          `json:"total_paise"`
	Address   string       `json:"address,omitempty"`
	Stages    []TrackStage `json:"stages,omitempty"`
}

type TrackStage struct {
	Title  string `json:"title"`
	Status string `json:"status"`
}

// Track fetches live status for an order by its UUID.
func (c *Client) Track(orderID string) (*Tracking, error) {
	b, err := c.do("GET", "/api/v2/order/"+orderID+"/", nil)
	if err != nil {
		return nil, err
	}
	var d map[string]any
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	t := &Tracking{
		Code:      str(d["code"]),
		Status:    str(d["formattedStatus"]),
		EtaMins:   toInt(d["committedEtaInMins"]),
		ItemCount: toInt(d["itemQuantityCount"]),
		Total:     toInt(d["grandTotalAmount"]),
		Address:   str(d["userAddressFormatted"]),
	}
	if osd, ok := d["orderShipmentDetails"].([]any); ok {
		for _, s := range osd {
			sm, _ := s.(map[string]any)
			if sm == nil {
				continue
			}
			if pv2, ok := sm["orderProductsV2"].(map[string]any); ok {
				if secs, ok := pv2["sectionData"].([]any); ok {
					for _, sec := range secs {
						secm, _ := sec.(map[string]any)
						if secm == nil {
							continue
						}
						st := TrackStage{Title: str(secm["sectionTitle"])}
						if ss, ok := secm["sectionStatus"].(map[string]any); ok {
							st.Status = str(ss["text"])
						}
						t.Stages = append(t.Stages, st)
					}
				}
			}
		}
	}
	return t, nil
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (c *Client) Orders(size int) ([]Order, error) {
	var all []Order
	seen := map[string]bool{}
	cursor := ""
	for page := 1; page <= 50; page++ {
		path := fmt.Sprintf("/api/v2/order/?page_number=%d", page)
		if cursor != "" {
			path += "&last_order_created_on=" + url.QueryEscape(cursor)
		}
		b, err := c.do("GET", path, nil)
		if err != nil {
			return nil, err
		}
		var or struct {
			Orders         []Order `json:"orders"`
			EndOfList      bool    `json:"endOfList"`
			NextPageParams struct {
				LastOrderCreatedOn string `json:"lastOrderCreatedOn"`
			} `json:"nextPageParams"`
		}
		if err := json.Unmarshal(b, &or); err != nil {
			return nil, err
		}
		added := 0
		for _, o := range or.Orders {
			key := o.ID
			if key == "" {
				key = o.Code
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			all = append(all, o)
			added++
		}
		if size > 0 && len(all) >= size {
			return all[:size], nil
		}
		if or.EndOfList || added == 0 || or.NextPageParams.LastOrderCreatedOn == "" {
			break
		}
		cursor = or.NextPageParams.LastOrderCreatedOn
	}
	return all, nil
}

type CartLine struct {
	Index     int
	Name      string
	Unit      string
	StoreID   string
	VariantID string
	MRP       int
	Qty       int
}

type CartBill struct {
	CartID      string
	ItemTotal   int
	DeliveryFee int
	GrandTotal  int
	Lines       []CartLine
}

func (c *Client) CartSync(items []CartLine) (*CartBill, error) {
	if c.s.AddressID == "" {
		return nil, fmt.Errorf("cart sync disabled until an explicit delivery address is selected; address selection is not implemented yet")
	}
	products := []map[string]any{}
	productsV2 := []map[string]any{}
	for _, it := range items {
		p := map[string]any{
			"storeProductId":       it.StoreID,
			"productVariantId":     it.VariantID,
			"mrp":                  it.MRP,
			"quantity":             it.Qty,
			"isDiscountApplicable": true,
			"isAddedToGiftBag":     false,
		}
		products = append(products, p)
		pv2 := map[string]any{
			"storeProductId":       it.StoreID,
			"productVariantId":     it.VariantID,
			"mrp":                  it.MRP,
			"quantity":             it.Qty,
			"isDiscountApplicable": true,
			"isAddedToGiftBag":     false,
			"cartProductId":        it.VariantID,
			"addOns":               []any{},
		}
		productsV2 = append(productsV2, pv2)
	}
	body := map[string]any{
		"storeId":                   c.s.StoreID,
		"cartId":                    "",
		"cartProducts":              products,
		"cartProductsV2":            productsV2,
		"swappedPvIds":              map[string]any{},
		"removedCampaignProducts":   []any{},
		"availedCampaignProducts":   []any{},
		"availedCampaignProductIds": []any{},
		"deliveryInstructions":      map[string]any{"leaveAtGate": false, "doNotRingBell": false, "bewareOfPets": false, "returnPaperBag": false, "returnCokePetBottle": false},
		"oosProductMeta":            []any{},
		"riderTip":                  0,
		"latitude":                  c.s.Latitude,
		"longitude":                 c.s.Longitude,
		"useZeptoCash":              true,
		"userPreferences":           map[string]any{},
		"isLayoutRequired":          len(items) > 0,
		"locationToAddressDistance": 1,
		"sessionMeta":               map[string]any{"clearUnserviceableProductsWidget": false},
		"userAddressId":             c.s.AddressID,
		"hasAds":                    true,
	}
	raw, _ := json.Marshal(body)
	b, err := c.do("POST", "/cfs/api/v1/cart/create", raw)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if msg, ok := m["message"]; ok {
		return nil, fmt.Errorf("cart error: %v", msg)
	}
	bill := &CartBill{
		CartID:      str(m["cartId"]),
		ItemTotal:   toInt(m["itemTotalAmount"]),
		DeliveryFee: toInt(m["deliveryFee"]),
		GrandTotal:  toInt(m["grandTotalAmount"]),
	}
	for i, it := range items {
		bill.Lines = append(bill.Lines, CartLine{
			Index: i + 1, Name: it.Name, Unit: it.Unit,
			StoreID: it.StoreID, VariantID: it.VariantID,
			MRP: it.MRP, Qty: it.Qty,
		})
	}
	return bill, nil
}

func (c *Client) Wallet() (int, error) {
	b, err := c.do("GET", "/api/v1/wallet-balances/total", nil)
	if err != nil {
		return 0, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return 0, err
	}
	for _, k := range []string{"totalBalance", "balance", "total", "walletBalance"} {
		if v, ok := m[k]; ok {
			return toInt(v), nil
		}
	}
	return 0, nil
}
