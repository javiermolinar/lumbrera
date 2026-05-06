package generate

import (
	"strings"

	"github.com/javiermolinar/lumbrera/internal/ops"
)

func ChangelogForRepo(repo string) (string, error) {
	entries, err := ops.Read(repo)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("# Lumbrera Changelog\n\n")
	b.WriteString("Generated from the Lumbrera operation log.\n\n")
	if len(entries) == 0 {
		b.WriteString("No Lumbrera writes yet.\n")
		return b.String(), nil
	}
	for _, entry := range entries {
		b.WriteString(ops.ChangelogLine(entry))
		b.WriteByte('\n')
	}
	return b.String(), nil
}
