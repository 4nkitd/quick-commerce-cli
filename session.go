package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Session struct {
	Cookie    string `json:"cookie"`
	DeviceID  string `json:"device_id"`
	SessionID string `json:"session_id"`
	StoreID   string `json:"store_id"`
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
		dir = filepath.Join(dir, ".config")
	}
	return filepath.Join(dir, "zepto", "session.json")
}

func loadSession() (*Session, error) {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("no session found — run `zepto login` first")
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Cookie == "" {
		return nil, fmt.Errorf("session is empty — run `zepto login` again")
	}
	return &s, nil
}

func saveSession(s *Session) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

var cookieHdrRe = regexp.MustCompile(`(?i)-H\s+['"]cookie:\s*([^'"]+)['"]`)
var cookieFlagRe = regexp.MustCompile(`(?i)(?:^|\s)(?:-b|--cookie)\s+['"]([^'"]+)['"]`)

// parseCurl derives a session from a "Copy as cURL" command (browser DevTools).
func parseCurl(curl string) (*Session, error) {
	var cookie string
	if m := cookieHdrRe.FindStringSubmatch(curl); m != nil {
		cookie = strings.TrimSpace(m[1])
	} else if m := cookieFlagRe.FindStringSubmatch(curl); m != nil {
		cookie = strings.TrimSpace(m[1])
	} else {
		return nil, fmt.Errorf("could not find a cookie header in the pasted request")
	}
	s := &Session{Cookie: cookie}
	get := func(name string) string {
		re := regexp.MustCompile(`(?i)(?:^|;|\s)` + regexp.QuoteMeta(name) + `=([^;]+)`)
		if mm := re.FindStringSubmatch(cookie); mm != nil {
			return strings.TrimSpace(mm[1])
		}
		return ""
	}
	hget := func(name string) string {
		re := regexp.MustCompile(`(?i)-H\s+['"]` + regexp.QuoteMeta(name) + `:\s*([^'"]+)['"]`)
		if mm := re.FindStringSubmatch(curl); mm != nil {
			return strings.TrimSpace(mm[1])
		}
		return ""
	}
	s.DeviceID = firstNonEmpty(hget("device_id"), get("device_id"))
	s.SessionID = firstNonEmpty(hget("session_id"), get("session_id"))
	s.StoreID = firstNonEmpty(hget("store_id"), hget("storeid"))
	s.Latitude = get("latitude")
	s.Longitude = get("longitude")
	if s.DeviceID == "" || s.SessionID == "" {
		return nil, fmt.Errorf("cookie found but device_id/session_id missing — copy the request as cURL from a bff-gateway.zepto.com call")
	}
	return s, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
