package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Session struct {
	Cookie     string            `json:"cookie"`
	DeviceID   string            `json:"device_id"`
	SessionID  string            `json:"session_id"`
	StoreID    string            `json:"store_id"`
	Latitude   string            `json:"latitude"`
	Longitude  string            `json:"longitude"`
	Provider   Provider          `json:"provider,omitempty"`
	AddressID  string            `json:"address_id,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Cookies    []BrowserCookie   `json:"browser_cookies,omitempty"`
	Storage    map[string]string `json:"browser_storage,omitempty"`
	VerifiedAt time.Time         `json:"verified_at,omitempty"`
}

func configPath() string {
	return providerConfigPath(activeProvider)
}

func providerConfigPath(provider Provider) string {
	dir := os.Getenv("QC_CONFIG_DIR")
	if dir == "" {
		var err error
		dir, err = os.UserConfigDir()
		if err != nil {
			dir, _ = os.UserHomeDir()
			dir = filepath.Join(dir, ".config")
		}
	}
	if provider == Zepto {
		return filepath.Join(dir, "zepto", "session.json")
	}
	return filepath.Join(dir, "quick-commerce", string(provider), "session.json")
}

func loadSession() (*Session, error) {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return nil, fmt.Errorf("cannot read session; run `qc %s login`", activeProvider)
	}
	var s Session
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	if s.Provider != "" && s.Provider != activeProvider {
		return nil, fmt.Errorf("session belongs to a different provider")
	}
	if s.Cookie == "" && len(s.Cookies) == 0 {
		return nil, fmt.Errorf("session is empty; run `qc %s login`", activeProvider)
	}
	return &s, nil
}

func saveSession(s *Session) error {
	return writePrivateJSON(configPath(), s)
}

func writePrivateJSON(p string, v any) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".qc-session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p)
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
