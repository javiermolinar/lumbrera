package generate

import (
	"github.com/javiermolinar/lumbrera/internal/ops"
)

// ChangelogForRepo reads CHANGELOG.md entries and re-renders them.
// This is used by verify to check that the file hasn't been hand-edited
// out of its expected format.
func ChangelogForRepo(repo string) (string, error) {
	entries, err := ops.Read(repo)
	if err != nil {
		return "", err
	}
	return ops.Render(entries), nil
}
