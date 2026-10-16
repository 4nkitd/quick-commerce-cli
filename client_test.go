package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientHeaders(t *testing.T) {
	s := &Session{Cookie: "accessToken=fake", DeviceID: "device", SessionID: "session", StoreID: "chosen-store", Headers: map[string]string{"store_ids": "chosen-store,secondary", "app_version": "current", "host": "evil.test"}}
	c := newClient(s)
	r, _ := http.NewRequest("GET", baseURL+"/test", nil)
	c.setHeaders(r)
	if r.Header.Get("store_ids") != "chosen-store,secondary" || r.Header.Get("app_version") != "current" {
		t.Fatal("browser context not used")
	}
	if r.Header.Get("host") != "" || r.Header.Get("store_etas") != "" || r.Header.Get("store_serviceability") != "" {
		t.Fatal("invented/untrusted headers used")
	}
	if c.http.CheckRedirect(r, nil) != http.ErrUseLastResponse {
		t.Fatal("redirects can leak credentials")
	}
}

func TestClientErrorsRedacted(t *testing.T) {
	for _, status := range []int{301, 401, 403, 429, 500} {
		c := newClient(&Session{})
		c.sleep = nil
		c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"accessToken":"private"}`)), Header: make(http.Header)}, nil
		})
		_, err := c.do("GET", "/test", nil)
		if err == nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("HTTP %d: %v", status, err)
		}
	}
}

func TestClientAutoRefreshAndRateLimitBackoff(t *testing.T) {
	calls := 0
	refreshed := false
	c := newClient(&Session{Cookie: "accessToken=expired", Cookies: []BrowserCookie{{Name: "refreshToken", Value: "r"}}})
	c.sleep = func(time.Duration) {}
	c.refresh = func() (*Session, error) {
		refreshed = true
		return &Session{Cookie: "accessToken=fresh"}, nil
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		case 2:
			if r.Header.Get("cookie") != "accessToken=fresh" {
				t.Fatalf("expected refreshed cookie, got %q", r.Header.Get("cookie"))
			}
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
		default:
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Header: make(http.Header)}, nil
		}
	})
	b, err := c.do("GET", "/test", nil)
	if err != nil || !refreshed || calls != 3 || string(b) != `{"ok":true}` {
		t.Fatalf("auto refresh/backoff failed: calls=%d refreshed=%v err=%v body=%s", calls, refreshed, err, b)
	}
}

func TestCartRequiresExplicitAddress(t *testing.T) {
	c := newClient(&Session{})
	called := false
	c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("unexpected network")
	})
	if _, err := c.CartSync(nil); err == nil || called {
		t.Fatal("cart sent without selected address")
	}
}
