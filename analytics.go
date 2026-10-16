package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

type SpendLineItem struct {
	Name              string   `json:"name"`
	Brand             string   `json:"brand,omitempty"`
	Unit              string   `json:"unit,omitempty"`
	Category          Category `json:"category"`
	SubcategoryID     string   `json:"subcategory_id,omitempty"`
	Quantity          int      `json:"quantity"`
	GrossPaise        int      `json:"gross_paise"`
	AllocatedNetPaise int      `json:"allocated_net_paise"`
}

type SpendOrder struct {
	Provider        Provider        `json:"provider"`
	ID              string          `json:"id"`
	Code            string          `json:"code"`
	CartID          string          `json:"cart_id,omitempty"`
	Status          string          `json:"status"`
	Title           string          `json:"title,omitempty"`
	PlacedAt        string          `json:"placed_at"`
	YearMonth       string          `json:"year_month,omitempty"`
	PaymentMethod   string          `json:"payment_method,omitempty"`
	ItemCount       int             `json:"item_count"`
	GrandTotalPaise int             `json:"grand_total_paise"`
	ItemTotalPaise  int             `json:"item_total_paise"`
	FeesPaise       int             `json:"fees_paise"`
	PromoDiscount   int             `json:"promo_discount_paise,omitempty"`
	RefundPaise     int             `json:"refund_paise,omitempty"`
	RefundMethod    string          `json:"refund_method,omitempty"`
	RefundNote      string          `json:"refund_note,omitempty"`
	NetSpentPaise   int             `json:"net_spent_paise"`
	IsReturnRecord  bool            `json:"is_return_record,omitempty"`
	ParentOrderID   string          `json:"parent_order_id,omitempty"`
	Items           []SpendLineItem `json:"items"`
}

type CategoryTotal struct {
	Category         Category `json:"category"`
	GrossPaise       int      `json:"gross_paise"`
	NetPaise         int      `json:"net_paise"`
	ZeptoNetPaise    int      `json:"zepto_net_paise"`
	BlinkitNetPaise  int      `json:"blinkit_net_paise"`
	ItemUnits        int      `json:"item_units"`
	OrderAppearances int      `json:"order_appearances"`
	SharePercent     float64  `json:"share_percent"`
}

type ProviderSpendSummary struct {
	Provider                 Provider        `json:"provider"`
	TotalRecords             int             `json:"total_records"`
	DeliveredOrders          int             `json:"delivered_orders"`
	CancelledOrders          int             `json:"cancelled_orders"`
	ReturnRecords            int             `json:"return_records"`
	FirstOrderAt             string          `json:"first_order_at"`
	LastOrderAt              string          `json:"last_order_at"`
	GrossPlacedPaise         int             `json:"gross_placed_paise"`
	CancelledPaise           int             `json:"cancelled_paise"`
	DeliveredGrossPaise      int             `json:"delivered_gross_paise"`
	OriginalMethodRefunds    int             `json:"original_method_refunds_paise"`
	PromoVoucherRefundsPaise int             `json:"promo_voucher_refunds_paise"`
	ExpiredPromoRefundsPaise int             `json:"expired_promo_refunds_paise"`
	NetEffectiveSpentPaise   int             `json:"net_effective_spent_paise"`
	ItemSubtotalNetPaise     int             `json:"item_subtotal_net_paise"`
	FeesNetPaise             int             `json:"fees_net_paise"`
	PromoDiscountsUsedPaise  int             `json:"promo_discounts_used_paise"`
	Categories               []CategoryTotal `json:"categories"`
	Orders                   []SpendOrder    `json:"orders,omitempty"`
}

type CombinedSpendReport struct {
	GeneratedAt             time.Time                       `json:"generated_at"`
	Currency                string                          `json:"currency"`
	AmountUnit              string                          `json:"amount_unit"`
	Providers               map[string]ProviderSpendSummary `json:"providers"`
	CombinedGrossDelivered  int                             `json:"combined_gross_delivered_paise"`
	CombinedOriginalRefunds int                             `json:"combined_original_refunds_paise"`
	CombinedNetSpentPaise   int                             `json:"combined_net_spent_paise"`
	CombinedCategories      []CategoryTotal                 `json:"combined_categories"`
	MonthlySpend            []MonthlyTotal                  `json:"monthly_spend"`
}

type MonthlyTotal struct {
	YearMonth       string `json:"year_month"`
	ZeptoNetPaise   int    `json:"zepto_net_paise"`
	BlinkitNetPaise int    `json:"blinkit_net_paise"`
	TotalNetPaise   int    `json:"total_net_paise"`
	OrderCount      int    `json:"order_count"`
}

var rupeeAmountRe = regexp.MustCompile(`₹\s*([0-9][0-9,]*(?:\.[0-9]{1,2})?)`)
var blinkitQtyRe = regexp.MustCompile(`x\s*([0-9]+)\s*$`)

func parseRupeesStringToPaise(s string) int {
	matches := rupeeAmountRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return 0
	}
	last := strings.ReplaceAll(matches[len(matches)-1][1], ",", "")
	val, err := strconv.ParseFloat(last, 64)
	if err != nil {
		return 0
	}
	ms := int(math.Round(val * 100))
	if strings.Contains(s, "-₹") {
		return -ms
	}
	return ms
}

func parseBlinkitQuantity(s string) int {
	m := blinkitQtyRe.FindStringSubmatch(strings.TrimSpace(s))
	if len(m) == 2 {
		if q, err := strconv.Atoi(m[1]); err == nil && q > 0 {
			return q
		}
	}
	return 1
}

var blinkitPlacedRe = regexp.MustCompile(`(?i)(\d{1,2})\s+([A-Za-z]{3})'(\d{2}),\s*(\d{1,2}:\d{2}\s*[AP]M)`)
var blinkitCardDateRe = regexp.MustCompile(`(?i)^(\d{1,2})\s+([A-Za-z]{3})(?:\s+(\d{4}))?`)

func normalizeOrderDates(raw string) (isoDate, yearMonth string) {
	s := strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		loc := t.In(time.FixedZone("IST", 5*3600+1800))
		return loc.Format("2006-01-02 15:04"), loc.Format("2006-01")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		loc := t.In(time.FixedZone("IST", 5*3600+1800))
		return loc.Format("2006-01-02 15:04"), loc.Format("2006-01")
	}
	if m := blinkitPlacedRe.FindStringSubmatch(s); len(m) == 5 {
		clean := fmt.Sprintf("%s %s 20%s %s", m[1], m[2], m[3], strings.ToUpper(strings.ReplaceAll(m[4], " ", "")))
		if t, err := time.Parse("2 Jan 2006 3:04PM", clean); err == nil {
			return t.Format("2006-01-02 15:04"), t.Format("2006-01")
		}
	}
	if m := blinkitCardDateRe.FindStringSubmatch(s); len(m) >= 3 {
		year := m[3]
		if year == "" {
			year = "2026"
		}
		clean := fmt.Sprintf("%s %s %s", m[1], m[2], year)
		if t, err := time.Parse("2 Jan 2006", clean); err == nil {
			return t.Format("2006-01-02"), t.Format("2006-01")
		}
	}
	return s, ""
}

func allocateItemSpend(order *SpendOrder) {
	for i := range order.Items {
		order.Items[i].Category = classifyItem(order.Items[i].Name, order.Items[i].SubcategoryID)
	}
	if order.NetSpentPaise <= 0 || len(order.Items) == 0 {
		for i := range order.Items {
			order.Items[i].AllocatedNetPaise = 0
		}
		return
	}
	sumGross := 0
	for _, it := range order.Items {
		if it.GrossPaise > 0 {
			sumGross += it.GrossPaise
		}
	}
	directRefund := order.GrandTotalPaise - order.NetSpentPaise
	if directRefund < 0 {
		directRefund = 0
	}
	target := order.ItemTotalPaise - order.PromoDiscount - directRefund
	if target < 0 {
		target = 0
	}
	if target > order.NetSpentPaise {
		target = order.NetSpentPaise
	}
	if target == 0 {
		for i := range order.Items {
			order.Items[i].AllocatedNetPaise = 0
		}
		return
	}
	if sumGross <= 0 {
		per := target / len(order.Items)
		rem := target - per*len(order.Items)
		for i := range order.Items {
			order.Items[i].AllocatedNetPaise = per
			if i == 0 {
				order.Items[i].AllocatedNetPaise += rem
			}
		}
		return
	}
	allocated := 0
	for i := range order.Items {
		share := int(math.Round(float64(target) * float64(order.Items[i].GrossPaise) / float64(sumGross)))
		order.Items[i].AllocatedNetPaise = share
		allocated += share
	}
	order.Items[0].AllocatedNetPaise += target - allocated
}

func fetchZeptoSpendOrders() ([]SpendOrder, error) {
	activeProvider = Zepto
	s, err := loadSession()
	if err != nil {
		return nil, err
	}
	c := newClient(s)
	list, err := c.Orders(0)
	if err != nil {
		return nil, err
	}
	var out []SpendOrder
	for _, summary := range list {
		raw, err := c.do("GET", "/api/v2/order/"+summary.ID+"/", nil)
		if err != nil {
			return nil, err
		}
		var d map[string]any
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, err
		}
		placedISO, ym := normalizeOrderDates(firstNonEmpty(str(d["placedTime"]), summary.PlacedTime))
		status := strings.ToUpper(firstNonEmpty(summary.Status, summary.FormattedStatus, str(d["formattedStatus"])))
		grand := toInt(d["grandTotalAmount"])
		if grand == 0 {
			grand = summary.GrandTotal
		}
		itemTotal := toInt(d["itemTotalAmount"])
		if itemTotal == 0 {
			itemTotal = grand
		}
		fees := grand - itemTotal
		if fees < 0 {
			fees = 0
		}
		var items []SpendLineItem
		itemRefundSum := 0
		if osd, ok := d["orderShipmentDetails"].([]any); ok {
			for _, sv := range osd {
				sm, _ := sv.(map[string]any)
				if status == "" {
					status = strings.ToUpper(firstNonEmpty(str(sm["status"]), str(sm["formattedStatus"])))
				}
				pv2, _ := sm["orderProductsV2"].(map[string]any)
				secs, _ := pv2["sectionData"].([]any)
				for _, secv := range secs {
					secm, _ := secv.(map[string]any)
					prods, _ := secm["orderProducts"].([]any)
					for _, pv := range prods {
						pm, _ := pv.(map[string]any)
						sp, _ := pm["storeProduct"].(map[string]any)
						pr, _ := sp["product"].(map[string]any)
						pvar, _ := sp["productVariant"].(map[string]any)
						qtyFul, hasFul := pm["quantityFulfilled"]
						qty := toInt(qtyFul)
						if !hasFul {
							qty = toInt(pm["quantityOrdered"])
						}
						itemRefund := toInt(pm["totalRefundAmount"])
						itemRefundSum += itemRefund
						if status == "DELIVERED" && hasFul && qty == 0 {
							continue
						}
						if qty == 0 {
							qty = 1
						}
						gross := toInt(pm["totalFinalSellingPrice"]) - itemRefund
						if gross < 0 {
							gross = 0
						}
						name := str(pr["name"])
						subcat := str(pr["primarySubCategoryId"])
						items = append(items, SpendLineItem{
							Name:          name,
							Brand:         str(pr["brand"]),
							Unit:          str(pvar["formattedPacksize"]),
							Category:      classifyItem(name, subcat),
							SubcategoryID: subcat,
							Quantity:      qty,
							GrossPaise:    gross,
						})
					}
				}
			}
		}
		refund := toInt(d["totalRefundAmount"])
		if refund == 0 && status == "DELIVERED" {
			refund = itemRefundSum
		}
		net := 0
		if status == "DELIVERED" {
			net = grand - refund
			if net < 0 {
				net = 0
			}
		}
		so := SpendOrder{
			Provider:        Zepto,
			ID:              summary.ID,
			Code:            firstNonEmpty(str(d["code"]), summary.Code),
			Status:          status,
			PlacedAt:        placedISO,
			YearMonth:       ym,
			ItemCount:       summary.ItemCount,
			GrandTotalPaise: grand,
			ItemTotalPaise:  itemTotal,
			FeesPaise:       fees,
			RefundPaise:     refund,
			NetSpentPaise:   net,
			Items:           items,
		}
		allocateItemSpend(&so)
		out = append(out, so)
	}
	return out, nil
}

func spendCachePath(p Provider) string {
	return filepath.Join(filepath.Dir(providerConfigPath(p)), "orders-history-cache.json")
}

func loadSpendCache(p Provider) ([]SpendOrder, error) {
	raw, err := os.ReadFile(spendCachePath(p))
	if err != nil {
		return nil, err
	}
	var orders []SpendOrder
	if err := json.Unmarshal(raw, &orders); err != nil {
		return nil, err
	}
	for i := range orders {
		var filtered []SpendLineItem
		for _, it := range orders[i].Items {
			if strings.HasSuffix(strings.ToLower(strings.TrimSpace(it.Name)), "unavailable") && it.GrossPaise == 0 {
				continue
			}
			filtered = append(filtered, it)
		}
		orders[i].Items = filtered
		allocateItemSpend(&orders[i])
	}
	return orders, nil
}

func saveSpendCache(p Provider, orders []SpendOrder) error {
	return writePrivateJSON(spendCachePath(p), orders)
}

type blinkitCardStub struct {
	ID       string   `json:"id"`
	CartID   string   `json:"cart_id"`
	Status   string   `json:"status"`
	Title    string   `json:"title"`
	Amount   string   `json:"amount"`
	Subtitle string   `json:"subtitle"`
	Deeplink string   `json:"deeplink"`
	Items    []string `json:"items"`
}

type blinkitDetailPayload struct {
	ID           string `json:"id"`
	CartID       string `json:"cart_id"`
	Status       string `json:"status"`
	CardTitle    string `json:"card_title"`
	CardAmount   string `json:"card_amount"`
	CardSubtitle string `json:"card_subtitle"`
	SummaryTitle string `json:"summary_title"`
	OrderCode    string `json:"order_code"`
	Placed       string `json:"placed"`
	Payment      string `json:"payment"`
	RefundHeader string `json:"refund_header"`
	PromoHeader  string `json:"promo_header"`
	PromoCode    string `json:"promo_code"`
	PromoState   string `json:"promo_state"`
	Items        []struct {
		Name  string `json:"name"`
		Qty   string `json:"qty"`
		Price string `json:"price"`
	} `json:"items"`
	Bills []struct {
		Label string `json:"label"`
		Val   string `json:"val"`
	} `json:"bills"`
}

func parseBlinkitDetail(d blinkitDetailPayload) SpendOrder {
	placedISO, ym := normalizeOrderDates(firstNonEmpty(d.Placed, d.CardSubtitle, d.CardAmount))
	isReturn := strings.EqualFold(d.CardTitle, "Return completed") || strings.EqualFold(d.SummaryTitle, "Return order summary")
	status := strings.ToUpper(d.Status)
	if isReturn {
		status = "RETURN_COMPLETED"
	} else if status == "" {
		status = "DELIVERED"
	}
	var items []SpendLineItem
	itemUnitSum := 0
	itemGrossSum := 0
	for _, it := range d.Items {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(it.Name)), "unavailable") && strings.TrimSpace(it.Price) == "" {
			continue
		}
		qty := parseBlinkitQuantity(it.Qty)
		gross := parseRupeesStringToPaise(it.Price)
		if gross < 0 {
			gross = 0
		}
		itemUnitSum += qty
		itemGrossSum += gross
		items = append(items, SpendLineItem{
			Name:       it.Name,
			Unit:       it.Qty,
			Category:   classifyItem(it.Name, ""),
			Quantity:   qty,
			GrossPaise: gross,
		})
	}
	grand := 0
	itemTotal := 0
	fees := 0
	promoDiscount := 0
	for _, b := range d.Bills {
		lbl := strings.ToLower(strings.TrimSpace(b.Label))
		val := parseRupeesStringToPaise(b.Val)
		switch {
		case strings.Contains(lbl, "bill total"):
			if val > 0 {
				grand = val
			}
		case strings.Contains(lbl, "item total"):
			if val > 0 {
				itemTotal = val
			}
		case strings.Contains(lbl, "promo discount") || strings.Contains(lbl, "coupon"):
			if val < 0 {
				promoDiscount += -val
			} else if val > 0 {
				promoDiscount += val
			}
		case strings.Contains(lbl, "handling") || strings.Contains(lbl, "delivery") || strings.Contains(lbl, "surge") || strings.Contains(lbl, "small cart") || strings.Contains(lbl, "tip") || strings.Contains(lbl, "charge"):
			if val > 0 {
				fees += val
			}
		}
	}
	if grand == 0 && !isReturn {
		if v := parseRupeesStringToPaise(d.CardAmount); v > 0 {
			grand = v
		}
	}
	if itemTotal == 0 && !isReturn {
		itemTotal = itemGrossSum
	}
	refund := 0
	refundMethod := ""
	refundNote := ""
	if d.RefundHeader != "" {
		if v := parseRupeesStringToPaise(d.RefundHeader); v > 0 {
			refund = v
			refundMethod = "original_source"
			refundNote = d.RefundHeader
		}
	} else if d.PromoHeader != "" {
		if v := parseRupeesStringToPaise(d.PromoHeader); v > 0 {
			refund = v
			refundMethod = "promo_code"
			if d.PromoState != "" {
				refundMethod += "_" + strings.ToLower(d.PromoState)
			}
			refundNote = d.PromoHeader
			if d.PromoCode != "" {
				refundNote += " (" + d.PromoCode + ")"
			}
		}
	}
	net := 0
	if status == "DELIVERED" {
		net = grand
		if refundMethod == "original_source" {
			net -= refund
			if net < 0 {
				net = 0
			}
		}
	}
	so := SpendOrder{
		Provider:        Blinkit,
		ID:              d.ID,
		Code:            firstNonEmpty(d.OrderCode, d.ID),
		CartID:          d.CartID,
		Status:          status,
		Title:           firstNonEmpty(d.CardTitle, d.SummaryTitle),
		PlacedAt:        placedISO,
		YearMonth:       ym,
		PaymentMethod:   d.Payment,
		ItemCount:       itemUnitSum,
		GrandTotalPaise: grand,
		ItemTotalPaise:  itemTotal,
		FeesPaise:       fees,
		PromoDiscount:   promoDiscount,
		RefundPaise:     refund,
		RefundMethod:    refundMethod,
		RefundNote:      refundNote,
		NetSpentPaise:   net,
		IsReturnRecord:  isReturn,
		Items:           items,
	}
	allocateItemSpend(&so)
	return so
}

type blinkitStubsCache struct {
	UpdatedAt time.Time         `json:"updated_at"`
	Stubs     []blinkitCardStub `json:"stubs"`
}

func blinkitStubsCachePath() string {
	return filepath.Join(filepath.Dir(providerConfigPath(Blinkit)), "order-stubs-cache.json")
}

func loadBlinkitStubsCache() ([]blinkitCardStub, error) {
	raw, err := os.ReadFile(blinkitStubsCachePath())
	if err != nil {
		return nil, err
	}
	var c blinkitStubsCache
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if time.Since(c.UpdatedAt) > 12*time.Hour || len(c.Stubs) == 0 {
		return nil, fmt.Errorf("stale stubs cache")
	}
	return c.Stubs, nil
}

func saveBlinkitStubsCache(stubs []blinkitCardStub) error {
	return writePrivateJSON(blinkitStubsCachePath(), blinkitStubsCache{
		UpdatedAt: time.Now().UTC(),
		Stubs:     stubs,
	})
}

func fetchBlinkitSpendOrders(refresh bool, limit int) ([]SpendOrder, error) {
	activeProvider = Blinkit
	var cached []SpendOrder
	var stubs []blinkitCardStub
	if c, err := loadSpendCache(Blinkit); err == nil && len(c) > 0 {
		cached = c
	}
	if !refresh {
		if st, err := loadBlinkitStubsCache(); err == nil && len(st) > 0 {
			stubs = st
		}
	}
	cachedByID := map[string]SpendOrder{}
	for _, o := range cached {
		cachedByID[o.ID] = o
	}
	if !refresh && len(stubs) > 0 {
		targetStubs := stubs
		if limit > 0 && len(targetStubs) > limit {
			targetStubs = targetStubs[:limit]
		}
		allCached := true
		var ready []SpendOrder
		for _, st := range targetStubs {
			o, ok := cachedByID[st.ID]
			if !ok {
				allCached = false
				break
			}
			ready = append(ready, o)
		}
		if allCached {
			return ready, nil
		}
	}
	unlock, err := authLock(Blinkit)
	if err != nil {
		return nil, err
	}
	defer unlock()
	s, err := loadSession()
	if err != nil {
		return nil, err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	b, err := newBrowser(ctx, Blinkit)
	if err != nil {
		return nil, err
	}
	defer b.close()
	if err = b.restore(s); err != nil {
		return nil, err
	}
	if err = b.verify(s); err != nil {
		return nil, err
	}
	if len(stubs) == 0 {
		nextURL := "/v1/layout/order_history"
		postback := "{}"
		seen := map[string]bool{}
		for batch := 0; batch < 15 && nextURL != ""; batch++ {
			var res struct {
				Orders   []blinkitCardStub `json:"orders"`
				NextURL  string            `json:"nextURL"`
				Postback string            `json:"postback"`
				StopWhy  string            `json:"stopWhy"`
			}
			script := fmt.Sprintf(`(async()=>{
				const sleep=ms=>new Promise(r=>setTimeout(r,ms));
				const a=JSON.parse(localStorage.getItem("auth")||"{}");
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
					platform:"desktop_web"
				};
				let url=%q;
				let body=%s;
				let stopWhy="";
				const orders=[];
				for(let i=0;i<5 && url;i++){
					let r=null;
					for(let attempt=0;attempt<4;attempt++){
						r=await fetch(url,{method:"POST",headers,body:JSON.stringify(body)});
						if(r.status!==429)break;
						await sleep(2500*(attempt+1));
					}
					if(!r.ok){stopWhy="http_"+r.status;break;}
					const d=await r.json();
					const snippets=d.response?.snippets||[];
					let pageAdded=0;
					for(const sn of snippets){
						if(sn.widget_type!=="order_history_container_vr")continue;
						const items=sn.data?.items||[];
						const h=items.find(x=>x.widget_type==="image_text_vr_type_header");
						const tags=items.find(x=>Array.isArray(x.data?.horizontal_item_list));
						const id=h?.tracking?.common_attributes?.order_id||sn.tracking?.common_attributes?.order_id||"";
						const dl=h?.tracking?.common_attributes?.deeplink||sn.tracking?.common_attributes?.deeplink||"";
						let cartId="";
						const m=dl.match(/cart_id=([^&]+)/);
						if(m)cartId=m[1];
						if(id){
							pageAdded++;
							orders.push({
								id,
								cart_id:cartId,
								status:h?.tracking?.common_attributes?.order_status||sn.tracking?.common_attributes?.order_status||"",
								title:h?.data?.title?.text||"",
								amount:h?.data?.left_underlined_subtitle?.text||"",
								subtitle:h?.data?.subtitle?.text||"",
								deeplink:dl,
								items:(tags?.data?.horizontal_item_list||[]).map(x=>x.data?.image?.accessibility_text?.text||"").filter(Boolean)
							});
						}
					}
					const next=d.response?.pagination?.next_url||"";
					body=d.postback_params||{};
					url=next;
					if(!next){stopWhy="no_next";break;}
					if(pageAdded===0){stopWhy="empty_page";break;}
					await sleep(550);
				}
				return {orders,nextURL:url,postback:JSON.stringify(body),stopWhy};
			})()`, nextURL, postback)
			if err = b.evalWithTimeout(75*time.Second, script, &res); err != nil {
				return nil, err
			}
			for _, st := range res.Orders {
				if seen[st.ID] {
					continue
				}
				seen[st.ID] = true
				stubs = append(stubs, st)
			}
			if limit > 0 && len(stubs) >= limit {
				break
			}
			nextURL = res.NextURL
			postback = res.Postback
			if res.StopWhy != "" {
				if strings.HasPrefix(res.StopWhy, "http_") {
					return nil, fmt.Errorf("Blinkit order history stopped early (%s)", res.StopWhy)
				}
				break
			}
		}
		if limit == 0 {
			_ = saveBlinkitStubsCache(stubs)
		}
	}
	targetStubs := stubs
	if limit > 0 && len(targetStubs) > limit {
		targetStubs = targetStubs[:limit]
	}
	mergeAndSaveCache := func() {
		seenID := map[string]bool{}
		var merged []SpendOrder
		for _, st := range stubs {
			if o, ok := cachedByID[st.ID]; ok && !seenID[o.ID] {
				seenID[o.ID] = true
				merged = append(merged, o)
			}
		}
		for _, old := range cached {
			if o, ok := cachedByID[old.ID]; ok && !seenID[o.ID] {
				seenID[o.ID] = true
				merged = append(merged, o)
			}
		}
		_ = saveSpendCache(Blinkit, merged)
	}
	var toFetch []blinkitCardStub
	for _, st := range targetStubs {
		if _, ok := cachedByID[st.ID]; !ok || refresh {
			toFetch = append(toFetch, st)
		}
	}
	for i := 0; i < len(toFetch); i += 5 {
		end := i + 5
		if end > len(toFetch) {
			end = len(toFetch)
		}
		chunk := toFetch[i:end]
		fmt.Fprintf(os.Stderr, "Blinkit: fetching itemized details %d-%d of %d...\n", i+1, end, len(toFetch))
		rawChunk, _ := json.Marshal(chunk)
		var details []blinkitDetailPayload
		detailScript := fmt.Sprintf(`(async()=>{
			const sleep=ms=>new Promise(r=>setTimeout(r,ms));
			const a=JSON.parse(localStorage.getItem("auth")||"{}");
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
				platform:"desktop_web"
			};
			const list=%s;
			const out=[];
			for(const f of list){
				const ep="/v1/layout/"+f.deeplink.replace("grofers://widgetized/","");
				let r=null;
				for(let attempt=0;attempt<4;attempt++){
					r=await fetch(ep,{method:"POST",headers,body:JSON.stringify({})});
					if(r.status!==429)break;
					await sleep(2500*(attempt+1));
				}
				if(!r.ok)throw new Error("detail_http_"+r.status);
				const d=await r.json();
				const items=[],bills=[];
				let summaryTitle="",orderCode="",placed="",payment="",refundHeader="",promoHeader="",promoCode="",promoState="";
				for(const sn of (d.response?.snippets||[])){
					if(sn.data?.title?.text && !summaryTitle)summaryTitle=sn.data.title.text;
					if(sn.widget_type==="z_v3_image_text_snippet_type_30"){
						items.push({name:sn.data?.title?.text||"",qty:sn.data?.subtitle1?.text||"",price:sn.data?.subtitle3?.text||""});
					}
					if(sn.widget_type==="cart_bill_item"){
						bills.push({label:sn.data?.left_header?.text||"",val:sn.data?.right_header?.text||""});
					}
					if(sn.data?.title?.text==="Order placed")placed=sn.data?.subtitle2?.text||"";
					if(sn.data?.title?.text==="Payment")payment=sn.data?.subtitle2?.text||"";
					if(sn.data?.button?.click_action?.copy_to_clipboard?.text)orderCode=sn.data.button.click_action.copy_to_clipboard.text;
					if(sn.widget_type==="refund_timeline_vr" && sn.data?.header?.title?.text){
						refundHeader=sn.data.header.title.text;
					}
					if(sn.widget_type==="order_history_container_vr" && Array.isArray(sn.data?.items)){
						for(const sub of sn.data.items){
							if(sub.data?.identity?.id==="odp_promo_header_0")promoHeader=sub.data?.title?.text||"";
							if(sub.data?.identity?.id==="odp_promo_code_0"){
								promoCode=sub.data?.subtitle?.text||sub.tracking?.common_attributes?.promo_code||"";
								promoState=sub.tracking?.common_attributes?.state||"";
							}
						}
					}
				}
				out.push({
					id:f.id,
					cart_id:f.cart_id,
					status:f.status,
					card_title:f.title,
					card_amount:f.amount,
					card_subtitle:f.subtitle,
					summary_title:summaryTitle,
					order_code:orderCode,
					placed,
					payment,
					refund_header:refundHeader,
					promo_header:promoHeader,
					promo_code:promoCode,
					promo_state:promoState,
					items,
					bills
				});
				await sleep(320);
			}
			return out;
		})()`, string(rawChunk))
		var chunkErr error
		for retry := 0; retry < 3; retry++ {
			chunkErr = b.evalWithTimeout(28*time.Second, detailScript, &details)
			if chunkErr == nil {
				break
			}
			time.Sleep(3 * time.Second)
		}
		if chunkErr != nil {
			return nil, chunkErr
		}
		for _, det := range details {
			so := parseBlinkitDetail(det)
			cachedByID[so.ID] = so
		}
		mergeAndSaveCache()
	}
	var out []SpendOrder
	for _, st := range targetStubs {
		if o, ok := cachedByID[st.ID]; ok {
			out = append(out, o)
		}
	}
	mergeAndSaveCache()
	return out, nil
}

func shortDatePrefix(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func summarizeProviderSpend(p Provider, orders []SpendOrder, includeOrders bool) ProviderSpendSummary {
	sum := ProviderSpendSummary{
		Provider:     p,
		TotalRecords: len(orders),
	}
	catMap := map[Category]*CategoryTotal{}
	for _, cat := range categoryOrder {
		catMap[cat] = &CategoryTotal{Category: cat}
	}
	for _, o := range orders {
		if o.YearMonth != "" && len(o.PlacedAt) >= 10 {
			if sum.FirstOrderAt == "" || o.PlacedAt < sum.FirstOrderAt {
				sum.FirstOrderAt = o.PlacedAt
			}
			if sum.LastOrderAt == "" || o.PlacedAt > sum.LastOrderAt {
				sum.LastOrderAt = o.PlacedAt
			}
		}
		if o.IsReturnRecord || o.Status == "RETURN_COMPLETED" {
			sum.ReturnRecords++
			continue
		}
		sum.GrossPlacedPaise += o.GrandTotalPaise
		if strings.Contains(o.Status, "CANCEL") {
			sum.CancelledOrders++
			sum.CancelledPaise += o.GrandTotalPaise
			continue
		}
		if !strings.Contains(o.Status, "DELIVER") {
			continue
		}
		sum.DeliveredOrders++
		sum.DeliveredGrossPaise += o.GrandTotalPaise
		sum.PromoDiscountsUsedPaise += o.PromoDiscount
		if o.RefundPaise > 0 {
			switch {
			case strings.HasPrefix(o.RefundMethod, "promo_code"):
				sum.PromoVoucherRefundsPaise += o.RefundPaise
				if strings.HasSuffix(o.RefundMethod, "_expired") {
					sum.ExpiredPromoRefundsPaise += o.RefundPaise
				}
			default:
				sum.OriginalMethodRefunds += o.RefundPaise
			}
		}
		sum.NetEffectiveSpentPaise += o.NetSpentPaise
		if o.NetSpentPaise <= 0 {
			continue
		}
		seenInOrder := map[Category]bool{}
		itemNet := 0
		for _, it := range o.Items {
			ct, ok := catMap[it.Category]
			if !ok {
				ct = &CategoryTotal{Category: it.Category}
				catMap[it.Category] = ct
			}
			ct.GrossPaise += it.GrossPaise
			ct.NetPaise += it.AllocatedNetPaise
			if p == Zepto {
				ct.ZeptoNetPaise += it.AllocatedNetPaise
			} else {
				ct.BlinkitNetPaise += it.AllocatedNetPaise
			}
			ct.ItemUnits += it.Quantity
			seenInOrder[it.Category] = true
			itemNet += it.AllocatedNetPaise
		}
		for cat := range seenInOrder {
			catMap[cat].OrderAppearances++
		}
		sum.ItemSubtotalNetPaise += itemNet
		feeNet := o.NetSpentPaise - itemNet
		if feeNet > 0 {
			sum.FeesNetPaise += feeNet
		}
	}
	for _, cat := range categoryOrder {
		ct := catMap[cat]
		if ct.NetPaise == 0 && ct.GrossPaise == 0 && ct.ItemUnits == 0 {
			continue
		}
		if sum.ItemSubtotalNetPaise > 0 {
			ct.SharePercent = math.Round(float64(ct.NetPaise)*10000/float64(sum.ItemSubtotalNetPaise)) / 100
		}
		sum.Categories = append(sum.Categories, *ct)
	}
	sort.Slice(sum.Categories, func(i, j int) bool {
		return sum.Categories[i].NetPaise > sum.Categories[j].NetPaise
	})
	if includeOrders {
		sum.Orders = orders
	}
	return sum
}

func buildCombinedSpendReport(providers map[Provider][]SpendOrder, includeOrders bool) CombinedSpendReport {
	rep := CombinedSpendReport{
		GeneratedAt: time.Now().UTC(),
		Currency:    "INR",
		AmountUnit:  "paise",
		Providers:   map[string]ProviderSpendSummary{},
	}
	catMap := map[Category]*CategoryTotal{}
	for _, cat := range categoryOrder {
		catMap[cat] = &CategoryTotal{Category: cat}
	}
	monthMap := map[string]*MonthlyTotal{}
	totalItemNet := 0
	for _, p := range []Provider{Blinkit, Zepto} {
		orders, ok := providers[p]
		if !ok {
			continue
		}
		ps := summarizeProviderSpend(p, orders, includeOrders)
		rep.Providers[string(p)] = ps
		rep.CombinedGrossDelivered += ps.DeliveredGrossPaise
		rep.CombinedOriginalRefunds += ps.OriginalMethodRefunds
		rep.CombinedNetSpentPaise += ps.NetEffectiveSpentPaise
		totalItemNet += ps.ItemSubtotalNetPaise
		for _, ct := range ps.Categories {
			agg := catMap[ct.Category]
			agg.GrossPaise += ct.GrossPaise
			agg.NetPaise += ct.NetPaise
			agg.ZeptoNetPaise += ct.ZeptoNetPaise
			agg.BlinkitNetPaise += ct.BlinkitNetPaise
			agg.ItemUnits += ct.ItemUnits
			agg.OrderAppearances += ct.OrderAppearances
		}
		for _, o := range orders {
			if o.IsReturnRecord || o.Status != "DELIVERED" || o.NetSpentPaise <= 0 || o.YearMonth == "" {
				continue
			}
			mt, ok := monthMap[o.YearMonth]
			if !ok {
				mt = &MonthlyTotal{YearMonth: o.YearMonth}
				monthMap[o.YearMonth] = mt
			}
			if p == Zepto {
				mt.ZeptoNetPaise += o.NetSpentPaise
			} else {
				mt.BlinkitNetPaise += o.NetSpentPaise
			}
			mt.TotalNetPaise += o.NetSpentPaise
			mt.OrderCount++
		}
	}
	for _, cat := range categoryOrder {
		ct := catMap[cat]
		if ct.NetPaise == 0 && ct.GrossPaise == 0 && ct.ItemUnits == 0 {
			continue
		}
		if totalItemNet > 0 {
			ct.SharePercent = math.Round(float64(ct.NetPaise)*10000/float64(totalItemNet)) / 100
		}
		rep.CombinedCategories = append(rep.CombinedCategories, *ct)
	}
	sort.Slice(rep.CombinedCategories, func(i, j int) bool {
		return rep.CombinedCategories[i].NetPaise > rep.CombinedCategories[j].NetPaise
	})
	for _, mt := range monthMap {
		rep.MonthlySpend = append(rep.MonthlySpend, *mt)
	}
	sort.Slice(rep.MonthlySpend, func(i, j int) bool {
		return rep.MonthlySpend[i].YearMonth < rep.MonthlySpend[j].YearMonth
	})
	return rep
}

func renderSpendMarkdown(rep CombinedSpendReport) string {
	var b bytes.Buffer
	b.WriteString("# Quick commerce spending report\n\n")
	fmt.Fprintf(&b, "Generated via `./qc spend --md` on %s UTC.\n\n", rep.GeneratedAt.Format("2006-01-02 15:04"))
	b.WriteString("## Platform summary\n\n")
	b.WriteString("| Platform | Date span | Records | Delivered | Cancelled | Return entries | Gross delivered | Direct refunds | Net effective spend |\n")
	b.WriteString("| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, p := range []Provider{Blinkit, Zepto} {
		ps, ok := rep.Providers[string(p)]
		if !ok {
			continue
		}
		span := "-"
		if ps.FirstOrderAt != "" {
			span = shortDatePrefix(ps.FirstOrderAt) + " to " + shortDatePrefix(ps.LastOrderAt)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %s | %s | **%s** |\n",
			strings.ToUpper(string(p)[:1])+string(p)[1:], span, ps.TotalRecords, ps.DeliveredOrders, ps.CancelledOrders, ps.ReturnRecords,
			rupees(ps.DeliveredGrossPaise), rupees(ps.OriginalMethodRefunds), rupees(ps.NetEffectiveSpentPaise))
	}
	if len(rep.Providers) > 1 {
		totalRec, totalDel, totalCan, totalRet := 0, 0, 0, 0
		for _, ps := range rep.Providers {
			totalRec += ps.TotalRecords
			totalDel += ps.DeliveredOrders
			totalCan += ps.CancelledOrders
			totalRet += ps.ReturnRecords
		}
		fmt.Fprintf(&b, "| **Combined** | **All retrieved history** | **%d** | **%d** | **%d** | **%d** | **%s** | **%s** | **%s** |\n",
			totalRec, totalDel, totalCan, totalRet,
			rupees(rep.CombinedGrossDelivered), rupees(rep.CombinedOriginalRefunds), rupees(rep.CombinedNetSpentPaise))
	}
	b.WriteString("\n### Accounting notes\n\n")
	b.WriteString("- Cancelled orders are excluded from net spend. Failed or retried checkout attempts that cancelled before delivery are listed under Cancelled.\n")
	if bl, ok := rep.Providers[string(Blinkit)]; ok {
		if bl.ReturnRecords > 0 {
			fmt.Fprintf(&b, "- Blinkit records %d physical item pickup(s) as separate `Return completed` entries with ₹0 bill totals. Their refunds appear on the original delivered order as a direct reversal to the payment source or as a replacement promo voucher.\n", bl.ReturnRecords)
		}
		if bl.PromoVoucherRefundsPaise > 0 {
			fmt.Fprintf(&b, "- Blinkit also issued %s in promo-code refunds on returned orders (%s of which later expired unused; claimed vouchers already reduce the bill total of the subsequent order where applied).\n",
				rupees(bl.PromoVoucherRefundsPaise), rupees(bl.ExpiredPromoRefundsPaise))
		}
	}
	b.WriteString("\n## Category breakdown\n\n")
	b.WriteString("| Category | Net item spend | Share | Blinkit net | Zepto net | Units | Orders |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	totalItemNet, totalFeesNet := 0, 0
	for _, ps := range rep.Providers {
		totalItemNet += ps.ItemSubtotalNetPaise
		totalFeesNet += ps.FeesNetPaise
	}
	for _, ct := range rep.CombinedCategories {
		fmt.Fprintf(&b, "| %s | **%s** | %.2f%% | %s | %s | %d | %d |\n",
			ct.Category, rupees(ct.NetPaise), ct.SharePercent,
			rupees(ct.BlinkitNetPaise), rupees(ct.ZeptoNetPaise), ct.ItemUnits, ct.OrderAppearances)
	}
	fmt.Fprintf(&b, "| *Delivery, handling & surge fees* | *%s* | - | - | - | - | - |\n", rupees(totalFeesNet))
	fmt.Fprintf(&b, "| **Total net effective spend** | **%s** | **100.00%%** | - | - | - | - |\n\n", rupees(totalItemNet+totalFeesNet))

	b.WriteString("```mermaid\npie showData\n    title Net merchandise spend by category (INR)\n")
	for _, ct := range rep.CombinedCategories {
		if ct.NetPaise <= 0 {
			continue
		}
		fmt.Fprintf(&b, "    %q : %.2f\n", string(ct.Category), float64(ct.NetPaise)/100)
	}
	b.WriteString("```\n\n")

	if len(rep.CombinedCategories) > 0 {
		b.WriteString("```mermaid\nxychart-beta\n    title \"Net Merchandise Spend by Category (INR)\"\n    x-axis [")
		maxCat := 0
		for i, ct := range rep.CombinedCategories {
			if i > 0 {
				b.WriteString(", ")
			}
			short := string(ct.Category)
			switch ct.Category {
			case CatFood:
				short = "Food"
			case CatElectronics:
				short = "Electronics"
			case CatPersonalBaby:
				short = "Care & Baby"
			case CatHome:
				short = "Home"
			case CatApparel:
				short = "Apparel"
			case CatAuto:
				short = "Auto & Tools"
			case CatPets:
				short = "Pets"
			case CatSportsToys:
				short = "Sports & Toys"
			case CatStationery:
				short = "Stationery"
			case CatFestive:
				short = "Festive"
			}
			fmt.Fprintf(&b, "%q", short)
			if ct.NetPaise > maxCat {
				maxCat = ct.NetPaise
			}
		}
		topCatAxis := int(math.Ceil(float64(maxCat)/1000000.0)) * 10000
		if topCatAxis < 1000 {
			topCatAxis = 1000
		}
		fmt.Fprintf(&b, "]\n    y-axis \"INR\" 0 --> %d\n    bar [", topCatAxis)
		for i, ct := range rep.CombinedCategories {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%.2f", float64(ct.NetPaise)/100)
		}
		b.WriteString("]\n```\n\n")
	}

	if len(rep.MonthlySpend) > 0 {
		b.WriteString("## Monthly net spend trend\n\n")
		b.WriteString("```mermaid\nxychart-beta\n    title \"Monthly Net Spend Across Platforms (INR)\"\n    x-axis [")
		for i, mt := range rep.MonthlySpend {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", mt.YearMonth)
		}
		b.WriteString("]\n    y-axis \"INR\" 0 --> ")
		maxMonth := 0
		for _, mt := range rep.MonthlySpend {
			if mt.TotalNetPaise > maxMonth {
				maxMonth = mt.TotalNetPaise
			}
		}
		topAxis := int(math.Ceil(float64(maxMonth)/100000.0)) * 1000
		if topAxis < 1000 {
			topAxis = 1000
		}
		fmt.Fprintf(&b, "%d\n    bar [", topAxis)
		for i, mt := range rep.MonthlySpend {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%.2f", float64(mt.TotalNetPaise)/100)
		}
		b.WriteString("]\n```\n\n")
		b.WriteString("| Month | Orders | Blinkit net | Zepto net | Combined net |\n")
		b.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
		for _, mt := range rep.MonthlySpend {
			fmt.Fprintf(&b, "| %s | %d | %s | %s | **%s** |\n",
				mt.YearMonth, mt.OrderCount, rupees(mt.BlinkitNetPaise), rupees(mt.ZeptoNetPaise), rupees(mt.TotalNetPaise))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func printSpendTable(rep CombinedSpendReport) {
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "PLATFORM\tSPAN\tDELIVERED\tCANCELLED\tRETURNS\tGROSS DELIVERED\tDIRECT REFUNDS\tNET SPENT")
	for _, p := range []Provider{Blinkit, Zepto} {
		ps, ok := rep.Providers[string(p)]
		if !ok {
			continue
		}
		span := "-"
		if ps.FirstOrderAt != "" {
			span = shortDatePrefix(ps.FirstOrderAt) + ".." + shortDatePrefix(ps.LastOrderAt)
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\t%s\t%s\n",
			p, span, ps.DeliveredOrders, ps.CancelledOrders, ps.ReturnRecords,
			rupees(ps.DeliveredGrossPaise), rupees(ps.OriginalMethodRefunds), rupees(ps.NetEffectiveSpentPaise))
	}
	if len(rep.Providers) > 1 {
		fmt.Fprintf(tw, "COMBINED\t-\t-\t-\t-\t%s\t%s\t%s\n",
			rupees(rep.CombinedGrossDelivered), rupees(rep.CombinedOriginalRefunds), rupees(rep.CombinedNetSpentPaise))
	}
	tw.Flush()
	fmt.Println()
	tw = tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "CATEGORY\tNET SPEND\tSHARE\tBLINKIT\tZEPTO\tUNITS\tORDERS")
	for _, ct := range rep.CombinedCategories {
		fmt.Fprintf(tw, "%s\t%s\t%.2f%%\t%s\t%s\t%d\t%d\n",
			ct.Category, rupees(ct.NetPaise), ct.SharePercent,
			rupees(ct.BlinkitNetPaise), rupees(ct.ZeptoNetPaise), ct.ItemUnits, ct.OrderAppearances)
	}
	tw.Flush()
}

func validateReportOutputPath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("--out must be a relative .md path inside the current directory")
	}
	clean := filepath.Clean(p)
	if clean == "." || strings.HasPrefix(clean, "..") || strings.ToLower(filepath.Ext(clean)) != ".md" {
		return "", fmt.Errorf("--out must be a relative .md path inside the current directory")
	}
	return clean, nil
}

func cmdSpend(scope string, args []string) {
	flags := flag.NewFlagSet("spend", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	mdOut := flags.Bool("md", false, "output Markdown report")
	outFile := flags.String("out", "", "write Markdown report to file")
	refresh := flags.Bool("refresh", false, "bypass local Blinkit history cache")
	includeOrders := flags.Bool("include-orders", false, "include all itemized orders in JSON output")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fail(fmt.Errorf("usage: qc [--json] [zepto|blinkit] spend [--md] [--out <file>] [--refresh]"))
	}
	var cleanOut string
	if *outFile != "" {
		var err error
		cleanOut, err = validateReportOutputPath(*outFile)
		if err != nil {
			fail(err)
		}
	}
	providers := map[Provider][]SpendOrder{}
	if scope == "all" || scope == string(Zepto) {
		zo, err := fetchZeptoSpendOrders()
		if err != nil {
			fail(err)
		}
		providers[Zepto] = zo
	}
	if scope == "all" || scope == string(Blinkit) {
		bo, err := fetchBlinkitSpendOrders(*refresh, 0)
		if err != nil {
			fail(err)
		}
		providers[Blinkit] = bo
	}
	rep := buildCombinedSpendReport(providers, *includeOrders)
	if cleanOut != "" {
		md := renderSpendMarkdown(rep)
		if err := os.WriteFile(cleanOut, []byte(md), 0o600); err != nil {
			fail(err)
		}
	}
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(rep)
		return
	}
	if *mdOut {
		fmt.Print(renderSpendMarkdown(rep))
		return
	}
	printSpendTable(rep)
}

func cmdBlinkitOrders(args []string) {
	n := 10
	refresh := false
	for _, a := range args {
		if a == "--refresh" {
			refresh = true
		} else if a == "--all" {
			n = 0
		} else {
			v, err := strconv.Atoi(a)
			if err != nil || v < 1 || v > 500 {
				fail(fmt.Errorf("usage: qc blinkit orders [1-500|--all] [--refresh]"))
			}
			n = v
		}
	}
	orders, err := fetchBlinkitSpendOrders(refresh, n)
	if err != nil {
		fail(err)
	}
	if jsonOutput {
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"provider":    Blinkit,
			"orders":      orders,
			"currency":    "INR",
			"amount_unit": "paise",
		})
		return
	}
	if len(orders) == 0 {
		fmt.Println("no orders yet")
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCODE\tDATE\tSTATUS\tITEMS\tTOTAL\tNET")
	for _, o := range orders {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			o.ID, o.Code, o.PlacedAt, o.Status, o.ItemCount, rupees(o.GrandTotalPaise), rupees(o.NetSpentPaise))
	}
	tw.Flush()
}
