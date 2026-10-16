//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func authLock(p Provider) (func(), error) {
	dir := filepath.Dir(providerConfigPath(p))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(dir, "auth.lock")
	if info, err := os.Stat(lockPath); err == nil && time.Since(info.ModTime()) > 15*time.Minute {
		_ = os.Remove(lockPath)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("another %s authentication command is running", p)
	}
	return func() {
		f.Close()
		_ = os.Remove(lockPath)
	}, nil
}
