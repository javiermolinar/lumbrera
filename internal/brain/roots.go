package brain

import "strings"

// ContentRoot describes a top-level content directory in a Lumbrera brain repo.
type ContentRoot struct {
	Dir      string // directory name: "sources", "wiki"
	Kind     string // kind string returned by path classification: "source", "wiki"
	Markdown bool   // whether files in this root must be Markdown
	Required bool   // whether the directory must exist for verify to pass
}

// ContentRoots defines the recognized content directories and their properties.
// Adding a new content type is a single entry here; all package helpers derive
// from this slice.
var ContentRoots = []ContentRoot{
	{Dir: "sources", Kind: "source", Markdown: true, Required: true},
	{Dir: "wiki", Kind: "wiki", Markdown: true, Required: true},
}

// RootForPath returns the ContentRoot whose directory prefix matches the given
// repo-relative path, or false if the path is not under any content root.
func RootForPath(p string) (ContentRoot, bool) {
	for _, root := range ContentRoots {
		prefix := root.Dir + "/"
		if strings.HasPrefix(p, prefix) && len(p) > len(prefix) {
			return root, true
		}
	}
	return ContentRoot{}, false
}

// KindForPath returns the kind string for a repo-relative path, or "" if the
// path is not under any content root.
func KindForPath(p string) string {
	root, ok := RootForPath(p)
	if !ok {
		return ""
	}
	return root.Kind
}

// IsContentPath returns true if the path falls under a recognized content root.
func IsContentPath(p string) bool {
	_, ok := RootForPath(p)
	return ok
}

// ContentDirs returns the directory names of all content roots.
func ContentDirs() []string {
	dirs := make([]string, len(ContentRoots))
	for i, root := range ContentRoots {
		dirs[i] = root.Dir
	}
	return dirs
}

// ContentDirList returns a human-readable list of content directories for error
// messages, e.g. "sources/ or wiki/" or "sources/, wiki/, or assets/".
func ContentDirList() string {
	dirs := ContentDirs()
	for i := range dirs {
		dirs[i] = dirs[i] + "/"
	}
	if len(dirs) <= 2 {
		return strings.Join(dirs, " or ")
	}
	return strings.Join(dirs[:len(dirs)-1], ", ") + ", or " + dirs[len(dirs)-1]
}
