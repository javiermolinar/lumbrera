package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
)

const Header = "lumbrera-sum-v1 sha256"

type Entry struct {
	Path string
	Hash string
}

func Generate(entries []Entry) (string, error) {
	entries = append([]Entry(nil), entries...)
	for i := range entries {
		if entries[i].Path == "" {
			return "", fmt.Errorf("manifest entry has empty path")
		}
		if strings.HasPrefix(entries[i].Path, "./") || pathpolicy.HasParentSegment(entries[i].Path) || strings.Contains(entries[i].Path, "\\") {
			return "", fmt.Errorf("manifest entry has unsafe path %q", entries[i].Path)
		}
		if entries[i].Hash == "" {
			return "", fmt.Errorf("manifest entry %q has empty hash", entries[i].Path)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	var b strings.Builder
	b.WriteString(Header)
	b.WriteByte('\n')
	for _, entry := range entries {
		fmt.Fprintf(&b, "%s sha256:%s\n", entry.Path, entry.Hash)
	}
	return b.String(), nil
}

func EntriesForRepo(repo string) ([]Entry, error) {
	var entries []Entry
	root := filepath.Join(repo, "wiki")
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("manifest refuses non-regular Markdown file %s", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("manifest refuses non-regular Markdown file %s", path)
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{Path: filepath.ToSlash(rel), Hash: HashContent(content)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

func ForRepo(repo string) (string, error) {
	entries, err := EntriesForRepo(repo)
	if err != nil {
		return "", err
	}
	return Generate(entries)
}

func HashContent(content []byte) string {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
