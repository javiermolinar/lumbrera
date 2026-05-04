package hooks

import (
	"os"
	"path/filepath"

	"github.com/javiermolinar/lumbrera/internal/git"
)

const preCommitHook = `#!/bin/sh
# Installed by Lumbrera.
# The commit subject does not exist yet in pre-commit, so CHANGELOG.md is
# checked later by pre-push after the commit has been created.
lumbrera_bin=${LUMBRERA_BIN:-lumbrera}
if command -v "$lumbrera_bin" >/dev/null 2>&1 && "$lumbrera_bin" verify --help >/dev/null 2>&1; then
  exec "$lumbrera_bin" verify --repo . --skip-changelog
fi
cat >&2 <<'EOF'
Lumbrera warning: could not find a lumbrera binary with verify support; skipping pre-commit verification.
Set LUMBRERA_BIN=/path/to/lumbrera to enable hook verification.
EOF
exit 0
`

const prePushHook = `#!/bin/sh
# Installed by Lumbrera.
lumbrera_bin=${LUMBRERA_BIN:-lumbrera}
if command -v "$lumbrera_bin" >/dev/null 2>&1 && "$lumbrera_bin" verify --help >/dev/null 2>&1; then
  exec "$lumbrera_bin" verify --repo .
fi
cat >&2 <<'EOF'
Lumbrera warning: could not find a lumbrera binary with verify support; skipping pre-push verification.
Set LUMBRERA_BIN=/path/to/lumbrera to enable hook verification.
EOF
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
