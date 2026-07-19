package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://bff-gateway.zepto.com"

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"

type Client struct {
	s    *Session
	http *http.Client
}

func newClient(s *Session) *Client {
	return &Client{s: s, http: &http.Client{Timeout: 20 * time.Second}}
}

func uuidv4() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// setHeaders applies the full app header fingerprint that clears Zepto's WAF.
func (c *Client) setHeaders(req *http.Request) {
	rid := uuidv4()
	h := req.Header
	h.Set("accept", "application/json, text/plain, */*")
	h.Set("content-type", "application/json")
	h.Set("user-agent", userAgent)
	h.Set("origin", "https://www.zepto.com")
	h.Set("referer", "https://www.zepto.com/")
	h.Set("cookie", c.s.Cookie)
	h.Set("platform", "WEB")
	h.Set("app_sub_platform", "WEB")
	h.Set("tenant", "ZEPTO")
	h.Set("app_version", "16.16.0")
	h.Set("appversion", "16.16.0")
	h.Set("auth_from_cookie", "true")
	h.Set("x-without-bearer", "true")
	h.Set("auth_revamp_flow", "v2")
	h.Set("marketplace_type", "SUPER_SAVER")
	h.Set("source", "DIRECT")
	h.Set("request_id", rid)
	h.Set("requestid", rid)
	h.Set("device_id", c.s.DeviceID)
	h.Set("deviceid", c.s.DeviceID)
	h.Set("session_id", c.s.SessionID)
	h.Set("sessionid", c.s.SessionID)
	h.Set("compatible_components", "EXTERNAL_COUPONS,BUNDLE,MULTI_SELLER_ENABLED,COUPON_WIDGET_CART_REVAMP,CART_REDESIGN_ENABLED,ZEPTO_PASS:5,NO_PLATFORM_CHECK_ENABLED_V2,HOMEPAGE_V2,SUPER_SAVER:1,ITEMISATION_ENABLED")
	h.Set("ispass", "true")
	if c.s.StoreID != "" {
		h.Set("store_id", c.s.StoreID)
		h.Set("storeid", c.s.StoreID)
		h.Set("store_ids", c.s.StoreID+",0cdd4ac3-fbeb-4bfd-8960-ebf2192d29b4")
		h.Set("store_etas", fmt.Sprintf(`{"%s":6,"0cdd4ac3-fbeb-4bfd-8960-ebf2192d29b4":24}`, c.s.StoreID))
		h.Set("store_serviceability", fmt.Sprintf(`{"data":{"0cdd4ac3-fbeb-4bfd-8960-ebf2192d29b4":{"cost":12000,"eta":24,"type":"SECONDARY_STORE"},"%s":{"cost":350,"eta":6,"type":"PRIMARY_STORE"}},"version":"v1"}`, c.s.StoreID))
	}
}

func (c *Client) do(method, path string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, baseURL+path, r)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("session expired (HTTP %d) — run `zepto login` again", resp.StatusCode)
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("blocked by Zepto (HTTP 429) — session may be stale, run `zepto login` again")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(b), 200))
	}
	return b, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
