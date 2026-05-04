package brainlock

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	lockPath  = ".brain/lock"
	ownerFile = "OWNER"
)

type Lock struct {
	path     string
	token    string
	released bool
}

func Acquire(brain, operation string) (*Lock, error) {
	path := filepath.Join(brain, filepath.FromSlash(lockPath))
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, lockedError(path)
		}
		return nil, err
	}

	owner := ownerContent(operation, token)
	if err := os.WriteFile(filepath.Join(path, ownerFile), []byte(owner), 0o600); err != nil {
		_ = os.RemoveAll(path)
		return nil, err
	}
	return &Lock{path: path, token: token}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.released {
		return nil
	}
	content, err := os.ReadFile(filepath.Join(l.path, ownerFile))
	if err != nil {
		return err
	}
	if !strings.Contains(string(content), "token: "+l.token+"\n") {
		return fmt.Errorf("refusing to release brain lock %s: lock owner changed", l.path)
	}
	if err := os.RemoveAll(l.path); err != nil {
		return err
	}
	l.released = true
	return nil
}

func lockedError(path string) error {
	owner, err := os.ReadFile(filepath.Join(path, ownerFile))
	if err != nil {
		return fmt.Errorf("brain is locked by another Lumbrera operation at %s", path)
	}
	return fmt.Errorf("brain is locked by another Lumbrera operation at %s:\n%s", path, strings.TrimSpace(string(owner)))
}

func newToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func ownerContent(operation, token string) string {
	host, _ := os.Hostname()
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "unknown"
	}
	return fmt.Sprintf("operation: %s\npid: %d\nhost: %s\ncreated: %s\ntoken: %s\n", operation, os.Getpid(), host, time.Now().UTC().Format(time.RFC3339Nano), token)
}
