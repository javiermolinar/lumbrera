package hooks

import (
	"os"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/git"
)

const preCommitHook = `#!/bin/sh
# Installed by Lumbrera.
# Full verification is not implemented yet.
exit 0
`

const prePushHook = `#!/bin/sh
# Installed by Lumbrera.
# Full verification is not implemented yet.
exit 0
`

const commitMsgHook = `#!/bin/sh
# Installed by Lumbrera.
subject=$(head -n 1 "$1")
if printf '%s\n' "$subject" | grep -Eq '^\[(init|source|create|append|update|delete|sync)\] \[[^]]+\]: .+$'; then
  exit 0
fi
cat >&2 <<'EOF'
Lumbrera commit subjects must use:
  [operation] [actor]: reason

Allowed operations:
  init, source, create, append, update, delete, sync

Example:
  [create] [pi-agent]: Add backendless architecture page
EOF
exit 1
`

func Install(repo string) error {
	hooksDir := filepath.Join(repo, ".brain", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}

	if err := writeHook(filepath.Join(hooksDir, "pre-commit"), preCommitHook); err != nil {
		return err
	}
	if err := writeHook(filepath.Join(hooksDir, "commit-msg"), commitMsgHook); err != nil {
		return err
	}
	if err := writeHook(filepath.Join(hooksDir, "pre-push"), prePushHook); err != nil {
		return err
	}

	return git.Config(repo, "core.hooksPath", ".brain/hooks")
}

func writeHook(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o755)
}
