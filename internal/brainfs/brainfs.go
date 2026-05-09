package brainfs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/brain"
)

type MarkdownFile struct {
	RelPath string
	AbsPath string
	Kind    string
	Content []byte
}

func WalkMarkdown(repo string, roots []string, visit func(MarkdownFile) error) error {
	for _, root := range roots {
		exists, err := ValidateDirectory(repo, root, false)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		absRoot := filepath.Join(repo, filepath.FromSlash(root))
		if err := filepath.WalkDir(absRoot, func(absPath string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(repo, absPath)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if entry.Type()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("%s is not a regular Markdown file", rel)
			}
			return visit(MarkdownFile{RelPath: rel, AbsPath: absPath, Kind: kindForPath(rel)})
		}); err != nil {
			return err
		}
	}
	return nil
}

func MarkdownPaths(repo string, roots []string) ([]string, error) {
	var paths []string
	if err := WalkMarkdown(repo, roots, func(file MarkdownFile) error {
		paths = append(paths, file.RelPath)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func ReadMarkdownFiles(repo string, roots []string) ([]MarkdownFile, error) {
	var files []MarkdownFile
	if err := WalkMarkdown(repo, roots, func(file MarkdownFile) error {
		content, err := os.ReadFile(file.AbsPath)
		if err != nil {
			return fmt.Errorf("read Markdown file %s: %w", file.RelPath, err)
		}
		file.Content = content
		files = append(files, file)
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].RelPath < files[j].RelPath })
	return files, nil
}

func ValidateDirectory(repo, rel string, required bool) (bool, error) {
	absPath := filepath.Join(repo, filepath.FromSlash(rel))
	info, err := os.Lstat(absPath)
	if err != nil {
		if os.IsNotExist(err) && !required {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("%s must be a real directory, not a symlink", rel)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s must be a directory", rel)
	}
	return true, nil
}

func kindForPath(rel string) string {
	return brain.KindForPath(rel)
}
