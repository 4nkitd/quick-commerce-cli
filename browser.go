package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type BrowserCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite,omitempty"`
}

type browser struct {
	ctx        context.Context
	name       string
	provider   Provider
	configDir  string
	configPath string
}

func newBrowser(ctx context.Context, p Provider) (*browser, error) {
	dir, err := os.MkdirTemp("", "qc-browser-")
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(dir, "agent-browser.json")
	if err = writePrivateJSON(configPath, map[string]string{"engine": "lightpanda"}); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return &browser{ctx: ctx, name: "qc-auth-" + string(p) + "-" + uuidv4(), provider: p, configDir: dir, configPath: configPath}, nil
}

func (b *browser) commandWithTimeout(timeout time.Duration, args ...string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(b.ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agent-browser", append([]string{"--config", b.configPath, "--session", b.name, "--engine", "lightpanda", "--json"}, args...)...)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "AGENT_BROWSER_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "AGENT_BROWSER_ENGINE=lightpanda")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("browser %s failed; check agent-browser and Lightpanda installation or retry login", args[0])
	}
	var r struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &r) != nil || !r.Success {
		return nil, fmt.Errorf("browser %s failed; the provider page may have changed", args[0])
	}
	return r.Data, nil
}

func (b *browser) command(args ...string) (json.RawMessage, error) {
	return b.commandWithTimeout(45*time.Second, args...)
}

func (b *browser) evalWithTimeout(timeout time.Duration, script string, target any) error {
	raw, err := b.commandWithTimeout(timeout, "eval", script)
	if err != nil {
		return err
	}
	var r struct {
		Result json.RawMessage `json:"result"`
	}
	if err = json.Unmarshal(raw, &r); err != nil {
		return fmt.Errorf("invalid browser response")
	}
	if target == nil {
		return nil
	}
	if err = json.Unmarshal(r.Result, target); err != nil {
		return fmt.Errorf("unexpected provider response")
	}
	return nil
}

func (b *browser) eval(script string, target any) error {
	return b.evalWithTimeout(45*time.Second, script, target)
}

func (b *browser) close() {
	defer os.RemoveAll(b.configDir)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cleanup := *b
	cleanup.ctx = ctx
	cleanup.command("close")
}

func (b *browser) open() error {
	_, err := b.command("open", b.provider.website())
	if err != nil {
		return fmt.Errorf("open provider site: %w", err)
	}
	_, err = b.command("wait", "--fn", `!!document.querySelector('input,button,a[href]')`)
	if err != nil {
		return fmt.Errorf("waiting for provider page: %w", err)
	}
	return nil
}

func firstPartyCookie(p Provider, c BrowserCookie) bool {
	d := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
	root := string(p) + ".com"
	return d == root || strings.HasSuffix(d, "."+root)
}

func (b *browser) cookies() ([]BrowserCookie, error) {
	raw, err := b.command("cookies")
	if err != nil {
		return nil, err
	}
	var r struct {
		Cookies []BrowserCookie `json:"cookies"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return nil, fmt.Errorf("invalid cookie response")
	}
	var cookies []BrowserCookie
	for _, c := range r.Cookies {
		if firstPartyCookie(b.provider, c) && !strings.HasPrefix(c.Name, "_g") && c.Name != "_fbp" {
			cookies = append(cookies, c)
		}
	}
	return cookies, nil
}

func cookieValue(cookies []BrowserCookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func cookieSetArgs(c BrowserCookie) []string {
	args := []string{"cookies", "set", c.Name, c.Value, "--domain", c.Domain, "--path", c.Path}
	if c.HTTPOnly {
		args = append(args, "--httpOnly")
	}
	if c.Secure {
		args = append(args, "--secure")
	}
	if c.SameSite != "" {
		args = append(args, "--sameSite", c.SameSite)
	}
	if c.Expires > 0 {
		args = append(args, "--expires", strconv.FormatInt(int64(c.Expires), 10))
	}
	return args
}

func (b *browser) restore(s *Session) error {
	if len(s.Cookies) == 0 {
		return fmt.Errorf("session has no browser state; run `qc %s login`", b.provider)
	}
	for _, c := range s.Cookies {
		if !firstPartyCookie(b.provider, c) {
			return fmt.Errorf("session contains a foreign cookie domain")
		}
	}
	if _, err := b.command("open", "about:blank"); err != nil {
		return err
	}
	for _, c := range s.Cookies {
		if c.Expires > 0 && c.Expires <= float64(time.Now().Unix()) {
			continue
		}
		if _, err := b.command(cookieSetArgs(c)...); err != nil {
			return err
		}
	}
	if b.provider == Blinkit {
		// Restore storage on an inert same-origin page before the app can persist guest state.
		if _, err := b.command("open", b.provider.website()+"/robots.txt"); err != nil {
			return err
		}
		var sameOrigin bool
		if err := b.eval(fmt.Sprintf("location.origin===%q", b.provider.website()), &sameOrigin); err != nil {
			return err
		}
		if !sameOrigin {
			return fmt.Errorf("provider redirected storage restore to a different origin")
		}
		for _, key := range []string{"auth", "authKey", "authKeyITS", "deviceId", "location"} {
			if value := s.Storage[key]; value != "" {
				if _, err := b.command("storage", "local", "set", key, value); err != nil {
					return err
				}
			}
		}
	}
	return b.open()
}

func (b *browser) snapshot() (*Session, error) {
	cs, err := b.cookies()
	if err != nil {
		return nil, err
	}
	s := &Session{Provider: b.provider, Cookies: cs, Storage: map[string]string{}}
	if b.provider == Blinkit {
		if cookieValue(cs, "gr_1_accessToken") == "" {
			return nil, fmt.Errorf("Blinkit has not issued an authenticated session")
		}
		err = b.eval(`Object.fromEntries(["auth","authKey","authKeyITS","deviceId","location"].map(k=>[k,localStorage.getItem(k)||""]))`, &s.Storage)
		s.Latitude = cookieValue(cs, "gr_1_lat")
		s.Longitude = cookieValue(cs, "gr_1_lon")
		return s, err
	}
	if cookieValue(cs, "accessToken") == "" {
		return nil, fmt.Errorf("Zepto has not issued an authenticated session")
	}
	s.DeviceID = cookieValue(cs, "device_id")
	s.SessionID = cookieValue(cs, "session_id")
	s.Latitude = cookieValue(cs, "latitude")
	s.Longitude = cookieValue(cs, "longitude")
	raw, err := b.command("network", "requests", "--filter", "bff-gateway.zepto.com")
	if err != nil {
		return nil, err
	}
	var traffic struct {
		Requests []struct {
			Method  string            `json:"method"`
			Headers map[string]string `json:"headers"`
			Status  int               `json:"status"`
		} `json:"requests"`
	}
	if json.Unmarshal(raw, &traffic) != nil {
		return nil, fmt.Errorf("invalid browser traffic metadata")
	}
	for _, r := range traffic.Requests {
		if r.Method == "OPTIONS" || r.Status != 200 {
			continue
		}
		h := map[string]string{}
		for k, v := range r.Headers {
			h[strings.ToLower(k)] = v
		}
		if !strings.Contains(h["cookie"], "accessToken=") {
			continue
		}
		s.Cookie = h["cookie"]
		s.StoreID = h["store_id"]
		s.Headers = map[string]string{}
		for _, k := range []string{"app_version", "appversion", "compatible_components", "store_ids", "store_etas", "store_serviceability", "x-csrf-secret", "x-xsrf-token", "request-signature", "x-timezone", "user-agent"} {
			if v := h[k]; v != "" {
				s.Headers[k] = v
			}
		}
	}
	if s.Cookie == "" {
		return nil, fmt.Errorf("no authenticated Zepto API request observed; retry login")
	}
	return s, nil
}

func (b *browser) verify(s *Session) error {
	if b.provider == Zepto {
		cl := newClient(s)
		cl.refresh = nil
		_, err := cl.do("GET", "/post-order-service/api/v2/order/last-ongoing-order", nil)
		return err
	}
	if _, err := b.command("wait", "--fn", `(()=>{try{return !!(window.grofers && localStorage.getItem("authKey") && JSON.parse(localStorage.getItem("auth")||"{}").accessToken)}catch{return false}})()`); err != nil {
		return fmt.Errorf("waiting for Blinkit authenticated state: %w", err)
	}
	var result struct {
		Status int  `json:"status"`
		Valid  bool `json:"valid"`
	}
	err := b.eval(`(async()=>{const a=JSON.parse(localStorage.getItem("auth")||"{}");if(!a.accessToken)return {status:401,valid:false};const c=window.grofers.CONFIG;const r=await fetch("/v1/order_count",{headers:{access_token:a.accessToken,auth_key:localStorage.getItem("authKey"),app_client:c.appClient,app_version:c.appVersion,device_id:localStorage.getItem("deviceId")}});let d;try{d=await r.json()}catch{}return {status:r.status,valid:r.ok&&!!d&&Object.hasOwn(d,"data")}})()`, &result)
	if err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("Blinkit session verification failed (HTTP %d); run `qc blinkit login`", result.Status)
	}
	return nil
}
