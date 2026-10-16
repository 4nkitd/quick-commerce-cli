package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNormalizePhone(t *testing.T) {
	for _, v := range []string{"9876543210", "+919876543210", " +919876543210 "} {
		got, err := normalizePhone(v)
		if err != nil || got != "9876543210" {
			t.Fatalf("normalize %q: %q %v", v, got, err)
		}
	}
	for _, v := range []string{"", "919876543210", "1234567890", "98765 43210", "9876543210;echo"} {
		if _, err := normalizePhone(v); err == nil {
			t.Fatalf("accepted %q", v)
		}
	}
}

func TestExtractOTP(t *testing.T) {
	now := time.Now().UTC()
	base := smsMessage{Sender: "ZEPTOO-T", Text: "Use OTP 123456 to verify your mobile number on Zepto. @www.zepto.com #123456", CreatedAt: now.Add(-time.Second)}
	tests := []struct {
		name string
		p    Provider
		edit func(*smsMessage)
		want string
	}{
		{"zepto repeated code", Zepto, func(*smsMessage) {}, "123456"},
		{"blinkit", Blinkit, func(m *smsMessage) {
			m.Sender = "AD-BLNKIT-S"
			m.Text = "Please use OTP 1234 to login to Blinkit. OTP valid for 4 minutes"
		}, "1234"},
		{"wrong sender", Zepto, func(m *smsMessage) { m.Sender = "HDFCBK-T" }, ""},
		{"outgoing", Zepto, func(m *smsMessage) { m.IsFromMe = true }, ""},
		{"old", Zepto, func(m *smsMessage) { m.CreatedAt = now.Add(-time.Hour) }, ""},
		{"future", Zepto, func(m *smsMessage) { m.CreatedAt = now.Add(time.Hour) }, ""},
		{"wrong provider", Blinkit, func(*smsMessage) {}, ""},
		{"two codes", Zepto, func(m *smsMessage) { m.Text = "Zepto OTP 123456 or 654321" }, ""},
		{"not an OTP", Zepto, func(m *smsMessage) { m.Text = "Zepto order 123456 delivered" }, ""},
		{"sender spoof", Zepto, func(m *smsMessage) { m.Sender = "person-ZEPTOO-T" }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			tt.edit(&m)
			raw, _ := json.Marshal(m)
			got, err := extractOTP(tt.p, raw, now.Add(-time.Minute), now)
			if err != nil || got != tt.want {
				t.Fatalf("got %q %v, want %q", got, err, tt.want)
			}
		})
	}
	a, _ := json.Marshal(base)
	base.Text = "Zepto OTP 654321"
	b, _ := json.Marshal(base)
	if _, err := extractOTP(Zepto, []byte(string(a)+"\n"+string(b)), now.Add(-time.Minute), now); err == nil {
		t.Fatal("accepted multiple fresh codes")
	}
	if _, err := extractOTP(Zepto, []byte("bad JSON"), now.Add(-time.Minute), now); err == nil {
		t.Fatal("accepted malformed JSON")
	}
	if got, err := extractOTP(Zepto, []byte(strings.Join([]string{string(a), string(a)}, "\n")), now.Add(-time.Minute), now); err != nil || got != "123456" {
		t.Fatal("duplicate message rejected")
	}
}
