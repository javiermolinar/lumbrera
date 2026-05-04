package ops

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const LogPath = ".brain/ops.log"

type Entry struct {
	Date      string `json:"date"`
	Operation string `json:"operation"`
	Actor     string `json:"actor"`
	Reason    string `json:"reason"`
}

func NewEntry(operation, actor, reason string, at time.Time) Entry {
	if at.IsZero() {
		at = time.Now()
	}
	return Entry{
		Date:      at.Format("2006-01-02"),
		Operation: strings.TrimSpace(operation),
		Actor:     strings.TrimSpace(actor),
		Reason:    strings.TrimSpace(reason),
	}
}

func Append(repo string, entry Entry) error {
	if err := Validate(entry); err != nil {
		return err
	}
	path := filepath.Join(repo, filepath.FromSlash(LogPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func Read(repo string) ([]Entry, error) {
	path := filepath.Join(repo, filepath.FromSlash(LogPath))
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()

	var entries []Entry
	scanner := bufio.NewScanner(file)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("%s:%d: invalid operation log entry: %w", LogPath, lineNo, err)
		}
		if err := Validate(entry); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", LogPath, lineNo, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func Validate(entry Entry) error {
	if strings.TrimSpace(entry.Date) == "" {
		return fmt.Errorf("operation date is required")
	}
	if _, err := time.Parse("2006-01-02", entry.Date); err != nil {
		return fmt.Errorf("operation date %q must use YYYY-MM-DD", entry.Date)
	}
	switch entry.Operation {
	case "source", "create", "append", "update", "delete":
	default:
		return fmt.Errorf("operation %q is not supported", entry.Operation)
	}
	if strings.TrimSpace(entry.Actor) == "" {
		return fmt.Errorf("operation actor is required")
	}
	if strings.ContainsAny(entry.Actor, "]\r\n") {
		return fmt.Errorf("operation actor must not contain ], carriage returns, or newlines")
	}
	if strings.TrimSpace(entry.Reason) == "" {
		return fmt.Errorf("operation reason is required")
	}
	if strings.ContainsAny(entry.Reason, "\r\n") {
		return fmt.Errorf("operation reason must be a single line")
	}
	return nil
}

func ChangelogLine(entry Entry) string {
	return fmt.Sprintf("%s [%s] [%s]: %s", entry.Date, entry.Operation, entry.Actor, entry.Reason)
}
