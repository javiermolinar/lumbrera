package ops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

const header = "# Lumbrera Changelog\n\n"

type Entry struct {
	Date      string
	Operation string
	Actor     string
	Reason    string
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

// Append adds an entry line to CHANGELOG.md.
func Append(repo string, entry Entry) error {
	if err := Validate(entry); err != nil {
		return err
	}
	path := filepath.Join(repo, filepath.FromSlash(brain.ChangelogPath))
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", brain.ChangelogPath, err)
	}
	text := string(content)

	// Remove the empty-state placeholder if present.
	text = strings.Replace(text, header+"No operations yet.\n", header, 1)

	// Ensure it ends with a newline before appending.
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += FormatLine(entry) + "\n"

	return os.WriteFile(path, []byte(text), 0o644)
}

// Read parses all entries from CHANGELOG.md.
func Read(repo string) ([]Entry, error) {
	path := filepath.Join(repo, filepath.FromSlash(brain.ChangelogPath))
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
		line := scanner.Text()
		// Skip header lines and empty lines.
		if line == "" || strings.HasPrefix(line, "#") || line == "No operations yet." {
			continue
		}
		entry, err := ParseLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", brain.ChangelogPath, lineNo, err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// Render produces the full CHANGELOG.md content from entries.
func Render(entries []Entry) string {
	var b strings.Builder
	b.WriteString(header)
	if len(entries) == 0 {
		b.WriteString("No operations yet.\n")
		return b.String()
	}
	for _, entry := range entries {
		b.WriteString(FormatLine(entry))
		b.WriteByte('\n')
	}
	return b.String()
}

// FormatLine renders a single changelog entry line.
func FormatLine(entry Entry) string {
	return fmt.Sprintf("- %s [%s] [%s]: %s", entry.Date, entry.Operation, entry.Actor, entry.Reason)
}

// ParseLine parses a single changelog entry line.
// Expected format: "YYYY-MM-DD [operation] [actor]: reason"
func ParseLine(line string) (Entry, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Entry{}, fmt.Errorf("empty changelog line")
	}

	// Strip optional leading list marker.
	if strings.HasPrefix(line, "- ") {
		line = line[2:]
	}

	// Date: first 10 chars.
	if len(line) < 11 || line[10] != ' ' {
		return Entry{}, fmt.Errorf("invalid changelog line: expected date prefix")
	}
	date := line[:10]
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Entry{}, fmt.Errorf("invalid changelog date %q", date)
	}
	rest := line[11:]

	// [operation] [actor]: reason
	if !strings.HasPrefix(rest, "[") {
		return Entry{}, fmt.Errorf("invalid changelog line: expected [operation]")
	}
	opEnd := strings.Index(rest, "]")
	if opEnd < 0 {
		return Entry{}, fmt.Errorf("invalid changelog line: unclosed [operation]")
	}
	operation := rest[1:opEnd]
	rest = rest[opEnd+1:]

	if !strings.HasPrefix(rest, " [") {
		return Entry{}, fmt.Errorf("invalid changelog line: expected [actor]")
	}
	rest = rest[2:]
	actorEnd := strings.Index(rest, "]")
	if actorEnd < 0 {
		return Entry{}, fmt.Errorf("invalid changelog line: unclosed [actor]")
	}
	actor := rest[:actorEnd]
	rest = rest[actorEnd+1:]

	if !strings.HasPrefix(rest, ": ") {
		return Entry{}, fmt.Errorf("invalid changelog line: expected ': ' after [actor]")
	}
	reason := rest[2:]

	entry := Entry{Date: date, Operation: operation, Actor: actor, Reason: reason}
	if err := Validate(entry); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func Validate(entry Entry) error {
	if strings.TrimSpace(entry.Date) == "" {
		return fmt.Errorf("operation date is required")
	}
	if _, err := time.Parse("2006-01-02", entry.Date); err != nil {
		return fmt.Errorf("operation date %q must use YYYY-MM-DD", entry.Date)
	}
	switch entry.Operation {
	case "source", "create", "append", "update", "asset", "delete", "migrate", "move":
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
