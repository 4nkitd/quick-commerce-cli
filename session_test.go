package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCookieRestorePreservesSecurityAttributes(t *testing.T) {
	c := BrowserCookie{Name: "accessToken", Value: "test", Domain: ".zepto.com", Path: "/", HTTPOnly: true, Secure: true, SameSite: "Lax", Expires: 1900000000}
	want := []string{"cookies", "set", "accessToken", "test", "--domain", ".zepto.com", "--path", "/", "--httpOnly", "--secure", "--sameSite", "Lax", "--expires", "1900000000"}
	if got := cookieSetArgs(c); !reflect.DeepEqual(got, want) {
		t.Fatalf("cookie flags: %v", got)
	}
	c.Expires = -1
	c.HTTPOnly = false
	c.Secure = false
	c.SameSite = ""
	if got := cookieSetArgs(c); !reflect.DeepEqual(got, want[:8]) {
		t.Fatalf("session cookie flags: %v", got)
	}
}

func TestPrivateAtomicWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "session.json")
	if err := writePrivateJSON(path, map[string]string{"token": "test-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateJSON(path, map[string]string{"token": "replacement"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("session is not private")
	}
	data, _ := os.ReadFile(path)
	var v map[string]string
	if json.Unmarshal(data, &v) != nil || v["token"] != "replacement" {
		t.Fatal("write failed")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".qc-session-*"))
	if len(files) > 0 {
		t.Fatal("temporary session left behind")
	}
	if err := writePrivateJSON(path, make(chan int)); err == nil {
		t.Fatal("unsupported value accepted")
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), "replacement") {
		t.Fatal("failed write damaged existing session")
	}
}

func TestProviderPathsAndDomains(t *testing.T) {
	if providerConfigPath(Zepto) == providerConfigPath(Blinkit) {
		t.Fatal("sessions share a path")
	}
	if !strings.HasSuffix(providerConfigPath(Zepto), filepath.Join("zepto", "session.json")) {
		t.Fatal("legacy path changed")
	}
	for _, d := range []string{".zepto.com", "www.zepto.com", "bff-gateway.zepto.com"} {
		if !firstPartyCookie(Zepto, BrowserCookie{Domain: d}) {
			t.Fatal(d)
		}
	}
	for _, d := range []string{"evilzepto.com", "zepto.com.evil.test", "blinkit.com", ""} {
		if firstPartyCookie(Zepto, BrowserCookie{Domain: d}) {
			t.Fatal(d)
		}
	}
	if _, err := parseProvider("../zepto"); err == nil {
		t.Fatal("invalid provider accepted")
	}
}

func TestCurlImport(t *testing.T) {
	for _, c := range []string{`curl https://bff-gateway.zepto.com -H 'cookie: accessToken=fake; device_id=d; session_id=s' -H 'store_id: store'`, `curl https://bff-gateway.zepto.com -b 'accessToken=fake; device_id=d; session_id=s' -H 'store_id: store'`} {
		s, err := parseCurl(c)
		if err != nil || s.DeviceID != "d" || s.SessionID != "s" || s.StoreID != "store" {
			t.Fatalf("parse: %+v %v", s, err)
		}
	}
	if _, err := parseCurl("curl -b 'accessToken=fake'"); err == nil {
		t.Fatal("missing IDs accepted")
	}
}
