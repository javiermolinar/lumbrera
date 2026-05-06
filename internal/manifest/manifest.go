package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brainfs"
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
	files, err := brainfs.ReadMarkdownFiles(repo, []string{"wiki"})
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(files))
	for _, file := range files {
		entries = append(entries, Entry{Path: file.RelPath, Hash: HashContent(file.Content)})
	}
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
