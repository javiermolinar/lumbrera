package searchindex

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/javiermolinar/lumbrera/internal/frontmatter"
	"github.com/javiermolinar/lumbrera/internal/pathpolicy"
)

func sanitizeQuery(query string) (sanitizedQuery, error) {
	parts := parseSearchParts(query)
	atoms := make([]string, 0, len(parts))
	terms := make([]string, 0, len(parts))
	seenTerms := map[string]struct{}{}
	for _, part := range parts {
		var text string
		if part.quoted {
			text = normalizeFTSText(part.text)
		} else {
			text = normalizeFTSText(part.text)
			if text != "" && !strings.Contains(text, " ") && searchStopwords[text] {
				continue
			}
		}
		if text == "" {
			continue
		}
		atoms = append(atoms, quoteFTSAtom(text))
		for _, term := range strings.Fields(text) {
			if searchStopwords[term] {
				continue
			}
			if _, ok := seenTerms[term]; ok {
				continue
			}
			seenTerms[term] = struct{}{}
			terms = append(terms, term)
		}
	}
	if len(atoms) == 0 {
		return sanitizedQuery{}, errors.New("search query has no searchable terms")
	}
	return sanitizedQuery{atoms: atoms, terms: terms}, nil
}

type searchPart struct {
	text   string
	quoted bool
}

func parseSearchParts(query string) []searchPart {
	var parts []searchPart
	var b strings.Builder
	inQuote := false
	for _, r := range query {
		switch {
		case r == '"':
			if inQuote {
				parts = appendPart(parts, b.String(), true)
				b.Reset()
				inQuote = false
				continue
			}
			parts = appendPart(parts, b.String(), false)
			b.Reset()
			inQuote = true
		case !inQuote && unicode.IsSpace(r):
			parts = appendPart(parts, b.String(), false)
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	parts = appendPart(parts, b.String(), inQuote)
	return parts
}

func appendPart(parts []searchPart, text string, quoted bool) []searchPart {
	text = strings.TrimSpace(text)
	if text == "" {
		return parts
	}
	return append(parts, searchPart{text: text, quoted: quoted})
}

func normalizeFTSText(text string) string {
	text = strings.TrimSpace(strings.ToLower(text))
	var b strings.Builder
	lastSpace := false
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_' {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if b.Len() > 0 && !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func quoteFTSAtom(text string) string {
	return `"` + strings.ReplaceAll(text, `"`, `""`) + `"`
}

func (q sanitizedQuery) matchAND() string {
	return strings.Join(q.atoms, " AND ")
}

func (q sanitizedQuery) matchOR() string {
	return strings.Join(q.atoms, " OR ")
}

func normalizeSearchOptions(opts SearchOptions) (SearchOptions, error) {
	if opts.Limit <= 0 {
		opts.Limit = DefaultSearchLimit
	}
	if opts.Limit > MaxSearchLimit {
		opts.Limit = MaxSearchLimit
	}
	if opts.Kind == "" {
		opts.Kind = KindAll
	}
	if opts.Kind != KindAll && opts.Kind != KindWiki && opts.Kind != KindSource {
		return SearchOptions{}, fmt.Errorf("invalid search kind %q", opts.Kind)
	}
	prefix, err := normalizePathPrefix(opts.PathPrefix)
	if err != nil {
		return SearchOptions{}, err
	}
	tags, err := normalizeSearchTags(opts.Tags)
	if err != nil {
		return SearchOptions{}, err
	}
	sources, err := normalizeSearchSources(opts.Sources)
	if err != nil {
		return SearchOptions{}, err
	}
	tiers, err := normalizeSearchTiers(opts.Tiers)
	if err != nil {
		return SearchOptions{}, err
	}
	opts.PathPrefix = prefix
	opts.Tags = tags
	opts.Sources = sources
	opts.Tiers = tiers
	return opts, nil
}

func normalizeSearchTags(tags []string) ([]string, error) {
	out := uniqueSortedStrings(tags)
	for _, tag := range out {
		if err := frontmatter.ValidateTags([]string{tag}); err != nil {
			return nil, fmt.Errorf("invalid search tag %q: %w", tag, err)
		}
	}
	return out, nil
}

var validTiers = map[string]bool{
	TierCanonical: true,
	TierDesign:    true,
	TierReference: true,
}

func normalizeSearchTiers(tiers []string) ([]string, error) {
	out := uniqueSortedStrings(tiers)
	for _, tier := range out {
		if !validTiers[tier] {
			return nil, fmt.Errorf("invalid search tier %q; valid tiers: canonical, design, reference", tier)
		}
	}
	return out, nil
}

func normalizeSearchSources(sources []string) ([]string, error) {
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		normalized, kind, err := pathpolicy.NormalizeTargetPath(source)
		if err != nil {
			return nil, fmt.Errorf("invalid search source %q: %w", source, err)
		}
		if kind != KindSource {
			return nil, fmt.Errorf("search source %q must be under sources/", source)
		}
		out = append(out, normalized)
	}
	return uniqueSortedStrings(out), nil
}

func normalizePathPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(filepath.ToSlash(prefix))
	if prefix == "" {
		return "", nil
	}
	if filepath.IsAbs(prefix) || path.IsAbs(prefix) {
		return "", fmt.Errorf("search path prefix %q must be repo-relative", prefix)
	}
	if strings.HasPrefix(prefix, "./") {
		prefix = strings.TrimPrefix(prefix, "./")
	}
	if pathpolicy.HasParentSegment(prefix) {
		return "", fmt.Errorf("search path prefix %q must not contain ..", prefix)
	}
	clean := path.Clean(prefix)
	if clean == "." {
		return "", nil
	}
	return clean, nil
}

func escapeLikePrefix(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '%', '_':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func decodeStringArray(value string) ([]string, error) {
	if value == "" {
		return []string{}, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil, err
	}
	if out == nil {
		return []string{}, nil
	}
	return out, nil
}

var searchStopwords = map[string]bool{
	"a": true, "about": true, "an": true, "and": true, "are": true, "as": true,
	"at": true, "be": true, "between": true, "by": true, "can": true,
	"compare": true, "comparison": true, "difference": true, "differences": true,
	"do": true, "does": true, "explain": true, "find": true, "for": true,
	"from": true, "how": true, "i": true, "in": true, "information": true,
	"is": true, "it": true, "of": true, "on": true, "or": true, "read": true,
	"relationship": true, "should": true, "that": true, "the": true, "this": true,
	"to": true, "understand": true, "versus": true, "vs": true, "was": true,
	"what": true, "when": true, "where": true, "which": true, "who": true,
	"why": true, "with": true,
}
