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
	s       *Session
	http    *http.Client
	refresh func() (*Session, error)
	sleep   func(time.Duration)
}

func newClient(s *Session) *Client {
	return &Client{
		s:       s,
		http:    &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		refresh: func() (*Session, error) { return refreshProviderSession(Zepto) },
		sleep:   time.Sleep,
	}
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
	h.Set("app_version", "17.3.1")
	h.Set("appversion", "17.3.1")
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
	if c.s.StoreID != "" {
		h.Set("store_id", c.s.StoreID)
		h.Set("storeid", c.s.StoreID)
		h.Set("store_ids", c.s.StoreID)
	}
	for _, k := range []string{"app_version", "appversion", "compatible_components", "store_ids", "store_etas", "store_serviceability", "x-csrf-secret", "x-xsrf-token", "request-signature", "x-timezone", "user-agent"} {
		if v := c.s.Headers[k]; v != "" {
			h.Set(k, v)
		}
	}
}

func (c *Client) do(method, path string, body []byte) ([]byte, error) {
	refreshed := false
	rateRetries := 0
	for {
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
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024+1))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("could not read provider response")
		}
		if len(b) > 8*1024*1024 {
			return nil, fmt.Errorf("provider response too large")
		}
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			if !refreshed && c.s != nil && len(c.s.Cookies) > 0 && c.refresh != nil {
				refreshed = true
				if fresh, err := c.refresh(); err == nil && fresh != nil {
					c.s = fresh
					continue
				}
			}
			return nil, fmt.Errorf("session rejected (HTTP %d); run `qc zepto auth status` or log in again", resp.StatusCode)
		}
		if resp.StatusCode == 429 {
			if rateRetries < 3 {
				rateRetries++
				if c.sleep != nil {
					c.sleep(time.Duration(rateRetries) * 1500 * time.Millisecond)
				}
				continue
			}
			return nil, fmt.Errorf("Zepto rate limit (HTTP 429); wait before retrying")
		}
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("Zepto request failed (HTTP %d)", resp.StatusCode)
		}
		return b, nil
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
