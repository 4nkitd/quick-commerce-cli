package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIHelper(t *testing.T) {
	if os.Getenv("QC_TEST_MAIN") != "1" {
		return
	}
	var args []string
	if json.Unmarshal([]byte(os.Getenv("QC_TEST_ARGS")), &args) != nil {
		os.Exit(2)
	}
	os.Args = append([]string{"qc"}, args...)
	main()
	os.Exit(0)
}

func runTestCLI(args ...string) (string, error) {
	raw, _ := json.Marshal(args)
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLIHelper$")
	cmd.Env = append(os.Environ(), "QC_TEST_MAIN=1", "QC_TEST_ARGS="+string(raw))
	cmd.Stdin = strings.NewReader(`curl -b 'accessToken=test; device_id=d; session_id=s'`)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestImportRespectsAuthLock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	unlock, err := authLock(Zepto)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	output, err := runTestCLI("zepto", "import-session")
	if err == nil || !strings.Contains(output, "another zepto authentication command is running") {
		t.Fatalf("import bypassed lock: %s %v", output, err)
	}
	if _, err := os.Stat(providerConfigPath(Zepto)); !os.IsNotExist(err) {
		t.Fatal("blocked import wrote a session")
	}
}

func TestSideEffectsBlocked(t *testing.T) {
	for _, p := range []string{"zepto", "blinkit"} {
		for _, command := range []string{"cart", "checkout", "support", "refund", "refunds"} {
			t.Run(p+"/"+command, func(t *testing.T) {
				output, err := runTestCLI("--json", p, command)
				if err == nil {
					t.Fatal("side effect command succeeded")
				}
				var result struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if json.Unmarshal([]byte(output), &result) != nil || result.Error.Code != "command_failed" || !strings.Contains(result.Error.Message, "no provider action was taken") {
					t.Fatalf("unexpected error: %s", output)
				}
			})
		}
	}
}
