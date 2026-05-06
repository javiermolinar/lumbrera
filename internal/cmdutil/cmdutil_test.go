package cmdutil

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsHelp(t *testing.T) {
	for _, arg := range []string{"--help", "-h", "help"} {
		if !IsHelp(arg) {
			t.Fatalf("IsHelp(%q) = false, want true", arg)
		}
	}
	if IsHelp("--helper") {
		t.Fatal("IsHelp accepted non-help flag")
	}
}

func TestSplitInlineFlag(t *testing.T) {
	name, value, ok := SplitInlineFlag("--limit=5")
	if name != "--limit" || value != "5" || !ok {
		t.Fatalf("SplitInlineFlag inline = (%q, %q, %t)", name, value, ok)
	}
	name, value, ok = SplitInlineFlag("query")
	if name != "query" || value != "" || ok {
		t.Fatalf("SplitInlineFlag positional = (%q, %q, %t)", name, value, ok)
	}
}

func TestOptionValue(t *testing.T) {
	value, next, err := OptionValue([]string{"--limit", "5"}, 0, "--limit", "", false)
	if err != nil || value != "5" || next != 1 {
		t.Fatalf("OptionValue next = (%q, %d, %v)", value, next, err)
	}
	value, next, err = OptionValue([]string{"--limit=7"}, 0, "--limit", "7", true)
	if err != nil || value != "7" || next != 0 {
		t.Fatalf("OptionValue inline = (%q, %d, %v)", value, next, err)
	}
	if _, _, err := OptionValue([]string{"--limit"}, 0, "--limit", "", false); err == nil || !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("missing value error = %v", err)
	}
	if _, _, err := OptionValue([]string{"--limit="}, 0, "--limit", "", true); err == nil || !strings.Contains(err.Error(), "requires a non-empty value") {
		t.Fatalf("empty inline value error = %v", err)
	}
}

func TestNonNilStrings(t *testing.T) {
	if NonNilStrings(nil) == nil {
		t.Fatal("NonNilStrings(nil) returned nil")
	}
	values := []string{"a"}
	if got := NonNilStrings(values); len(got) != 1 || got[0] != "a" {
		t.Fatalf("NonNilStrings values = %#v", got)
	}
}

func TestWriteJSON(t *testing.T) {
	var out bytes.Buffer
	if err := WriteJSON(&out, map[string][]string{"items": nil}); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}
	want := "{\n  \"items\": null\n}\n"
	if out.String() != want {
		t.Fatalf("WriteJSON = %q, want %q", out.String(), want)
	}
}
