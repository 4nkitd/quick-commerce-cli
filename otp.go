package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type smsMessage struct {
	Text      string    `json:"text"`
	Sender    string    `json:"sender"`
	CreatedAt time.Time `json:"created_at"`
	IsFromMe  bool      `json:"is_from_me"`
}

var phonePattern = regexp.MustCompile(`^[6-9][0-9]{9}$`)
var zeptoSender = regexp.MustCompile(`(?i)^(?:[A-Z]{2}-)?ZEPTOO(?:-[A-Z])?$`)
var blinkitSender = regexp.MustCompile(`(?i)^(?:[A-Z]{2}-)?BLNKIT(?:-[A-Z])?$`)

func normalizePhone(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	phone = strings.TrimPrefix(phone, "+91")
	if !phonePattern.MatchString(phone) {
		return "", fmt.Errorf("provide a 10-digit Indian mobile number, optionally prefixed with +91")
	}
	return phone, nil
}

func otpDigits(p Provider) int {
	if p == Blinkit {
		return 4
	}
	return 6
}

func extractOTP(p Provider, raw []byte, since, now time.Time) (string, error) {
	scan := bufio.NewScanner(bytes.NewReader(raw))
	scan.Buffer(make([]byte, 4096), 1024*1024)
	codes := map[string]bool{}
	codeRE := regexp.MustCompile(fmt.Sprintf(`\b[0-9]{%d}\b`, otpDigits(p)))
	senderRE := zeptoSender
	if p == Blinkit {
		senderRE = blinkitSender
	}
	for scan.Scan() {
		var m smsMessage
		if json.Unmarshal(scan.Bytes(), &m) != nil {
			return "", fmt.Errorf("invalid imsg response")
		}
		if m.IsFromMe || m.CreatedAt.Before(since) || m.CreatedAt.After(now) || !senderRE.MatchString(m.Sender) {
			continue
		}
		text := strings.ToLower(m.Text)
		if !strings.Contains(text, string(p)) || !(strings.Contains(text, "otp") || strings.Contains(text, "verification code")) {
			continue
		}
		matches := codeRE.FindAllString(m.Text, -1)
		unique := map[string]bool{}
		for _, match := range matches {
			unique[match] = true
		}
		if len(unique) != 1 {
			continue
		}
		for match := range unique {
			codes[match] = true
		}
	}
	if err := scan.Err(); err != nil {
		return "", fmt.Errorf("imsg response too large")
	}
	if len(codes) > 1 {
		return "", fmt.Errorf("multiple fresh login codes found; retry with manual OTP input")
	}
	for code := range codes {
		return code, nil
	}
	return "", nil
}

func readMessages(ctx context.Context, p Provider) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "imsg", "search", "--query", string(p), "--limit", "20", "--json")
	raw, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("imsg cannot read Messages; enable Full Disk Access for the host terminal/app and restart it")
	}
	return raw, nil
}

func waitOTP(ctx context.Context, p Provider, since time.Time) (string, error) {
	deadline, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		raw, err := readMessages(deadline, p)
		if err != nil {
			return "", err
		}
		code, err := extractOTP(p, raw, since, time.Now())
		if err != nil || code != "" {
			return code, err
		}
		select {
		case <-deadline.Done():
			return "", fmt.Errorf("no fresh %s OTP received within 90 seconds; no resend attempted", p)
		case <-time.After(2 * time.Second):
		}
	}
}
