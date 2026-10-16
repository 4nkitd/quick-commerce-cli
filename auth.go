package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func authContext() (context.Context, func()) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	return ctx, func() { cancel(); stop() }
}

func login(p Provider, args []string) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	phone := flags.String("phone", "", "Indian mobile number")
	source := flags.String("otp-source", "stdin", "stdin or imsg")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("usage: qc %s login --phone <number> [--otp-source stdin|imsg]", p)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected login arguments")
	}
	number, err := normalizePhone(*phone)
	if err != nil {
		return err
	}
	if *source != "stdin" && *source != "imsg" {
		return fmt.Errorf("otp-source must be stdin or imsg")
	}
	unlock, err := authLock(p)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, cancel := authContext()
	defer cancel()
	if *source == "imsg" {
		if _, err = readMessages(ctx, p); err != nil {
			return err
		}
	}
	b, err := newBrowser(ctx, p)
	if err != nil {
		return err
	}
	defer b.close()
	if err = b.open(); err != nil {
		return err
	}
	label, placeholder := "login", "Enter Phone Number"
	if p == Blinkit {
		label, placeholder = "Login", "Enter mobile number"
	}
	if _, err = b.command("wait", "--fn", fmt.Sprintf(`Array.from(document.querySelectorAll('button,div,span')).some(e=>e.textContent.trim()===%q)`, label)); err != nil {
		return fmt.Errorf("waiting for login control: %w", err)
	}
	if p == Zepto {
		// Zepto renders the login button before React attaches its click handler.
		if _, err = b.command("wait", "--fn", `(()=>{const e=document.querySelector('button[aria-label="login"]');return !!e&&Object.keys(e).some(k=>k.startsWith("__reactProps")&&typeof e[k].onClick==="function")})()`); err != nil {
			return fmt.Errorf("waiting for Zepto login hydration: %w", err)
		}
	}
	if _, err = b.command("find", "text", label, "click", "--exact"); err != nil {
		return err
	}
	selector := fmt.Sprintf("input[placeholder=%q]", placeholder)
	if _, err = b.command("wait", "--fn", fmt.Sprintf("!!document.querySelector(%q)", selector)); err != nil {
		return fmt.Errorf("waiting for phone input: %w", err)
	}
	if _, err = b.command("find", "placeholder", placeholder, "fill", number); err != nil {
		return err
	}
	since := time.Now()
	if _, err = b.command("find", "text", "Continue", "click", "--exact"); err != nil {
		return err
	}
	if _, err = b.command("wait", "--fn", `document.body.innerText.includes("OTP Verification")`); err != nil {
		return fmt.Errorf("provider did not display OTP verification; no resend attempted")
	}
	fmt.Fprintf(os.Stderr, "%s requested a login OTP for ******%s. No automatic resends.\n", p, number[len(number)-4:])
	var code string
	if *source == "imsg" {
		code, err = waitOTP(ctx, p, since)
	} else {
		fmt.Fprintln(os.Stderr, "Enter OTP on stdin:")
		type input struct {
			code string
			err  error
		}
		ch := make(chan input, 1)
		go func() {
			line, e := bufio.NewReader(io.LimitReader(os.Stdin, 64)).ReadString('\n')
			if e == io.EOF && line != "" {
				e = nil
			}
			ch <- input{strings.TrimSpace(line), e}
		}()
		select {
		case in := <-ch:
			code, err = in.code, in.err
		case <-ctx.Done():
			return fmt.Errorf("login timed out waiting for OTP")
		}
	}
	if err != nil {
		return err
	}
	if !regexp.MustCompile(fmt.Sprintf(`^[0-9]{%d}$`, otpDigits(p))).MatchString(code) {
		return fmt.Errorf("expected a %d-digit OTP", otpDigits(p))
	}
	if p == Zepto {
		_, err = b.command("fill", `[role=dialog] input[type=text]`, code)
	} else {
		for i, digit := range code {
			_, err = b.command("fill", fmt.Sprintf("input[type=tel]:nth-child(%d)", i+1), string(digit))
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		return err
	}
	if _, err = b.command("wait", "--fn", `!document.body.innerText.includes("OTP Verification")`); err != nil {
		return fmt.Errorf("OTP was not accepted; no retry or resend attempted")
	}
	var s *Session
	for attempt := 0; attempt < 10; attempt++ {
		s, err = b.snapshot()
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("login timed out")
		case <-time.After(time.Second):
		}
	}
	if err != nil {
		return err
	}
	if err = b.verify(s); err != nil {
		return err
	}
	if p == Blinkit {
		s, err = b.snapshot()
		if err != nil {
			return err
		}
	}
	s.VerifiedAt = time.Now().UTC()
	if err = saveSession(s); err != nil {
		return err
	}
	return printAuth(p, s, true)
}

func refreshProviderSession(p Provider) (*Session, error) {
	prev := activeProvider
	activeProvider = p
	defer func() { activeProvider = prev }()
	unlock, err := authLock(p)
	if err != nil {
		return nil, err
	}
	defer unlock()
	s, err := loadSession()
	if err != nil {
		return nil, err
	}
	ctx, cancel := authContext()
	defer cancel()
	b, err := newBrowser(ctx, p)
	if err != nil {
		return nil, err
	}
	defer b.close()
	if len(s.Cookies) == 0 && p == Zepto {
		if err = b.verify(s); err != nil {
			return nil, err
		}
	} else {
		if err = b.restore(s); err != nil {
			return nil, err
		}
		if p == Zepto {
			if _, err = b.command("wait", "--fn", `document.cookie.includes("isAuth=true")`); err != nil {
				return nil, fmt.Errorf("Zepto session expired; run `qc zepto login`")
			}
			var latest *Session
			for attempt := 0; attempt < 10; attempt++ {
				latest, err = b.snapshot()
				if err == nil {
					s = latest
					break
				}
				select {
				case <-ctx.Done():
					return nil, fmt.Errorf("session check timed out")
				case <-time.After(time.Second):
				}
			}
			if err != nil {
				return nil, err
			}
		}
		if err = b.verify(s); err != nil {
			return nil, err
		}
		if p == Blinkit {
			s, err = b.snapshot()
			if err != nil {
				return nil, err
			}
		}
	}
	s.VerifiedAt = time.Now().UTC()
	if err = saveSession(s); err != nil {
		return nil, err
	}
	return s, nil
}

func authStatus(p Provider, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: qc %s auth status", p)
	}
	s, err := refreshProviderSession(p)
	if err != nil {
		return err
	}
	return printAuth(p, s, true)
}

func printAuth(p Provider, s *Session, live bool) error {
	status := "saved"
	if live {
		status = "verified"
	}
	if jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"provider": p, "status": status, "verified_at": s.VerifiedAt, "session_file": providerConfigPath(p)})
	}
	fmt.Printf("%s session %s\nSession: %s\n", p, status, providerConfigPath(p))
	if !s.VerifiedAt.IsZero() {
		fmt.Printf("Last live check: %s\n", s.VerifiedAt.Format(time.RFC3339))
	}
	return nil
}
