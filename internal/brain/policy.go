package brain

import "strings"

type Kind string

const (
	KindSource Kind = "source"
	KindNote   Kind = "note"
	KindWiki   Kind = "wiki"
	KindAsset  Kind = "asset"
)

type StorageMode uint8

const (
	StorageRawMarkdown StorageMode = iota
	StorageManagedMarkdown
	StorageBinary
)

// ContentPolicy describes the closed behavior of one content kind.
type ContentPolicy struct {
	Root             string
	Kind             Kind
	Storage          StorageMode
	RequiredRoot     bool
	Mutable          bool
	RequiresEvidence bool
	EvidenceKinds    []Kind
	ProvidesEvidence bool
	CatalogPath      string
}

var contentPolicies = []ContentPolicy{
	{
		Root:             "sources",
		Kind:             KindSource,
		Storage:          StorageRawMarkdown,
		RequiredRoot:     true,
		Mutable:          false,
		RequiresEvidence: false,
		ProvidesEvidence: true,
		CatalogPath:      SourcesIndexPath,
	},
	{
		Root:             "notes",
		Kind:             KindNote,
		Storage:          StorageManagedMarkdown,
		RequiredRoot:     true,
		Mutable:          true,
		RequiresEvidence: false,
		ProvidesEvidence: true,
		CatalogPath:      NotesIndexPath,
	},
	{
		Root:             "wiki",
		Kind:             KindWiki,
		Storage:          StorageManagedMarkdown,
		RequiredRoot:     true,
		Mutable:          true,
		RequiresEvidence: true,
		EvidenceKinds:    []Kind{KindSource, KindNote},
		ProvidesEvidence: false,
		CatalogPath:      IndexPath,
	},
	{
		Root:             "assets",
		Kind:             KindAsset,
		Storage:          StorageBinary,
		RequiredRoot:     true,
		Mutable:          false,
		RequiresEvidence: false,
		ProvidesEvidence: false,
		CatalogPath:      AssetsIndexPath,
	},
}

func (p ContentPolicy) IsMarkdown() bool {
	return p.Storage == StorageRawMarkdown || p.Storage == StorageManagedMarkdown
}

func (p ContentPolicy) clone() ContentPolicy {
	p.EvidenceKinds = append([]Kind(nil), p.EvidenceKinds...)
	return p
}

func Policies() []ContentPolicy {
	return clonePolicySlice(contentPolicies)
}

func PolicyForKind(kind Kind) (ContentPolicy, bool) {
	for _, policy := range contentPolicies {
		if policy.Kind == kind {
			return policy.clone(), true
		}
	}
	return ContentPolicy{}, false
}

func PolicyForPath(p string) (ContentPolicy, bool) {
	for _, policy := range contentPolicies {
		prefix := policy.Root + "/"
		if strings.HasPrefix(p, prefix) && len(p) > len(prefix) {
			return policy.clone(), true
		}
	}
	return ContentPolicy{}, false
}

func ManagedPolicies() []ContentPolicy {
	var out []ContentPolicy
	for _, policy := range contentPolicies {
		if policy.Storage == StorageManagedMarkdown {
			out = append(out, policy.clone())
		}
	}
	return out
}

func MarkdownPolicies() []ContentPolicy {
	var out []ContentPolicy
	for _, policy := range contentPolicies {
		if policy.IsMarkdown() {
			out = append(out, policy.clone())
		}
	}
	return out
}

func ManagedRoots() []string {
	return rootsOf(ManagedPolicies())
}

func SearchRoots() []string {
	return rootsOf(MarkdownPolicies())
}

func RequiredRoots() []string {
	var required []ContentPolicy
	for _, policy := range contentPolicies {
		if policy.RequiredRoot {
			required = append(required, policy)
		}
	}
	return rootsOf(required)
}

func CatalogPathForKind(kind Kind) (string, bool) {
	policy, ok := PolicyForKind(kind)
	if !ok || policy.CatalogPath == "" {
		return "", false
	}
	return policy.CatalogPath, true
}

func IsManagedKind(kind Kind) bool {
	policy, ok := PolicyForKind(kind)
	return ok && policy.Storage == StorageManagedMarkdown
}

func IsManagedPath(path string) bool {
	policy, ok := PolicyForPath(path)
	return ok && policy.Storage == StorageManagedMarkdown
}

func AcceptsEvidence(kind Kind) bool {
	policy, ok := PolicyForKind(kind)
	return ok && len(policy.EvidenceKinds) > 0
}

func CanUseAsEvidence(documentKind, evidenceKind Kind) bool {
	documentPolicy, ok := PolicyForKind(documentKind)
	if !ok {
		return false
	}
	evidencePolicy, ok := PolicyForKind(evidenceKind)
	if !ok || !evidencePolicy.ProvidesEvidence {
		return false
	}
	for _, kind := range documentPolicy.EvidenceKinds {
		if kind == evidenceKind {
			return true
		}
	}
	return false
}

// KindForPath returns the kind string for a repo-relative path, or "" if the
// path is not under any content root.
func KindForPath(p string) string {
	policy, ok := PolicyForPath(p)
	if !ok {
		return ""
	}
	return string(policy.Kind)
}

// IsContentPath returns true if the path falls under a recognized content root.
func IsContentPath(p string) bool {
	_, ok := PolicyForPath(p)
	return ok
}

// ContentDirs returns the directory names of all content roots.
func ContentDirs() []string {
	return rootsOf(contentPolicies)
}

// ContentDirList returns a human-readable list of content directories for error
// messages, e.g. "sources/ or wiki/" or "sources/, notes/, wiki/, or assets/".
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

func clonePolicySlice(in []ContentPolicy) []ContentPolicy {
	out := make([]ContentPolicy, len(in))
	for i, policy := range in {
		out[i] = policy.clone()
	}
	return out
}

func rootsOf(policies []ContentPolicy) []string {
	roots := make([]string, len(policies))
	for i, policy := range policies {
		roots[i] = policy.Root
	}
	return roots
}
