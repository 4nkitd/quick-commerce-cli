package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserUsesPrivateConfigIncludingCleanup(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("QC_TEST_LOG", log)
	t.Setenv("AGENT_BROWSER_CDP", "http://127.0.0.1:9")
	t.Setenv("AGENT_BROWSER_CONFIG", "untrusted-config.json")
	t.Setenv("AGENT_BROWSER_PROVIDER", "untrusted-provider")
	script := `#!/bin/sh
[ -z "$AGENT_BROWSER_CDP$AGENT_BROWSER_CONFIG$AGENT_BROWSER_PROVIDER" ] || exit 1
[ "$1" = --config ] && [ -f "$2" ] || exit 1
printf '%s\n' "$*" >> "$QC_TEST_LOG"
printf '{"success":true,"data":{}}\n'
`
	if err := os.WriteFile(filepath.Join(dir, "agent-browser"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	b, err := newBrowser(context.Background(), Blinkit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := os.Stat(b.configDir); err == nil {
			b.close()
		}
	})
	raw, err := os.ReadFile(b.configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]string
	if json.Unmarshal(raw, &config) != nil || len(config) != 1 || config["engine"] != "lightpanda" {
		t.Fatalf("unsafe config: %s", raw)
	}
	for _, path := range []string{b.configDir, b.configPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatal("browser config is not private")
		}
	}
	if _, err = b.command("open", "about:blank"); err != nil {
		t.Fatal(err)
	}
	b.close()
	raw, err = os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(calls) != 2 {
		t.Fatalf("unexpected calls: %s", raw)
	}
	for _, call := range calls {
		if !strings.Contains(call, "--config "+b.configPath+" ") {
			t.Fatal("missing controlled config")
		}
	}
	if !strings.HasSuffix(calls[1], " close") {
		t.Fatal("cleanup not invoked")
	}
	if _, err = os.Stat(b.configDir); !os.IsNotExist(err) {
		t.Fatal("temporary config left behind")
	}
}

func TestBlinkitRestoresStorageBeforeAppLoads(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("QC_TEST_LOG", log)
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$QC_TEST_LOG"
printf '{"success":true,"data":{"result":true}}\n'
`
	if err := os.WriteFile(filepath.Join(dir, "agent-browser"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	b, err := newBrowser(context.Background(), Blinkit)
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	s := &Session{Cookies: []BrowserCookie{{Name: "gr_1_accessToken", Value: "fixture", Domain: "blinkit.com", Path: "/"}}, Storage: map[string]string{"auth": "fixture"}}
	if err = b.restore(s); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := string(raw)
	bootstrap := strings.Index(calls, " open https://blinkit.com/robots.txt\n")
	storage := strings.Index(calls, " storage local set auth fixture\n")
	app := strings.Index(calls, " open https://blinkit.com\n")
	if bootstrap < 0 || storage <= bootstrap || app <= storage {
		t.Fatal("app loaded before restoring authentication storage")
	}
}
