package servecmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpAndInvalidConfiguration(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, "test", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Git is not required") {
		t.Fatal(out.String())
	}
	for _, args := range [][]string{{}, {"--unknown"}, {"extra"}, {"--brain", ".", "--repository-url", "http://insecure.example/repo"}} {
		if err := run(args, "test", &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
