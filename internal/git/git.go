package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Result struct {
	Stdout string
	Stderr string
}

type CommandError struct {
	Args     []string
	Repo     string
	ExitCode int
	Stderr   string
}

func (e *CommandError) Error() string {
	cmd := "git " + strings.Join(e.Args, " ")
	message := fmt.Sprintf("%s failed in %s", cmd, e.Repo)
	if e.ExitCode >= 0 {
		message += fmt.Sprintf(" with exit code %d", e.ExitCode)
	}
	if strings.TrimSpace(e.Stderr) != "" {
		message += ": " + strings.TrimSpace(e.Stderr)
	}
	return message
}

func EnsureAvailable() error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("Git is required by Lumbrera but was not found in PATH. Install Git from https://git-scm.com/downloads and try again")
	}
	return nil
}

func Run(repo string, args ...string) (Result, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}

	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
	return result, &CommandError{Args: args, Repo: repo, ExitCode: exitCode, Stderr: result.Stderr}
}

func IsRepo(repo string) bool {
	root, err := WorkTreeRoot(repo)
	if err != nil {
		return false
	}

	repoPath, err := normalizePath(repo)
	if err != nil {
		return false
	}
	rootPath, err := normalizePath(root)
	if err != nil {
		return false
	}
	return repoPath == rootPath
}

func WorkTreeRoot(repo string) (string, error) {
	result, err := Run(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(result.Stdout)
	if root == "" {
		return "", errors.New("git rev-parse --show-toplevel returned empty path")
	}
	return root, nil
}

func normalizePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func Init(repo string) error {
	if _, err := Run(repo, "init", "-b", "main"); err == nil {
		return nil
	}
	if _, err := Run(repo, "init"); err != nil {
		return err
	}
	_, err := Run(repo, "checkout", "-B", "main")
	return err
}

func StatusPorcelain(repo string) (string, error) {
	result, err := Run(repo, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

func IsClean(repo string) (bool, error) {
	status, err := StatusPorcelain(repo)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(status) == "", nil
}

func HasCommits(repo string) bool {
	_, err := Run(repo, "rev-parse", "--verify", "HEAD")
	return err == nil
}

func CurrentBranch(repo string) (string, error) {
	result, err := Run(repo, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Stdout), nil
}

func Remotes(repo string) ([]string, error) {
	result, err := Run(repo, "remote")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines, nil
}

func AddAll(repo string) error {
	_, err := Run(repo, "add", "-A")
	return err
}

func Commit(repo, message string) error {
	_, err := Run(repo, "commit", "-m", message)
	return err
}

func Config(repo, key, value string) error {
	_, err := Run(repo, "config", key, value)
	return err
}
