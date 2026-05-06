package markdown

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type Reference struct {
	Path   string
	Anchor string
}

type Heading struct {
	Level  int
	Text   string
	Anchor string
}

type Analysis struct {
	FirstH1          string
	Sources          []string
	Links            []string
	Headings         []Heading
	Anchors          []string
	LinkReferences   []Reference
	SourceReferences []Reference
	SourceCitations  []Reference
}

type AnalyzeOptions struct {
	SourceCitations bool
	IgnoreLinks     bool
}

func Analyze(repoRelativePath, body string) (Analysis, error) {
	return AnalyzeWithOptions(repoRelativePath, body, AnalyzeOptions{SourceCitations: true})
}

func AnalyzeWithOptions(repoRelativePath, body string, opts AnalyzeOptions) (Analysis, error) {
	source := []byte(body)
	doc := goldmark.DefaultParser().Parse(text.NewReader(source))

	var analysis Analysis
	inSources := false
	anchorCounts := map[string]int{}
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		switch n := node.(type) {
		case *ast.Heading:
			if !entering {
				return ast.WalkContinue, nil
			}
			text := strings.TrimSpace(string(n.Text(source)))
			anchor := uniqueHeadingAnchor(text, anchorCounts)
			analysis.Headings = append(analysis.Headings, Heading{Level: n.Level, Text: text, Anchor: anchor})
			analysis.Anchors = append(analysis.Anchors, anchor)
			if n.Level == 1 && analysis.FirstH1 == "" {
				analysis.FirstH1 = text
			}
			if n.Level <= 2 {
				inSources = n.Level == 2 && strings.EqualFold(text, "Sources")
			}
		case *ast.Link:
			if !entering {
				return ast.WalkContinue, nil
			}
			if opts.IgnoreLinks {
				return ast.WalkSkipChildren, nil
			}
			ref, ok, err := NormalizeReference(repoRelativePath, string(n.Destination))
			if err != nil {
				return ast.WalkStop, err
			}
			if ok {
				if inSources {
					analysis.SourceReferences = append(analysis.SourceReferences, ref)
					analysis.Sources = append(analysis.Sources, ref.Path)
				} else {
					analysis.LinkReferences = append(analysis.LinkReferences, ref)
					if !isSelfAnchorReference(repoRelativePath, ref) {
						analysis.Links = append(analysis.Links, ref.Path)
					}
				}
			}
			return ast.WalkSkipChildren, nil
		case *ast.CodeSpan:
			if entering {
				return ast.WalkSkipChildren, nil
			}
		case *ast.Paragraph:
			if !entering || inSources || !opts.SourceCitations {
				return ast.WalkContinue, nil
			}
			refs, err := sourceCitationReferences(repoRelativePath, paragraphCitationText(n, source))
			if err != nil {
				return ast.WalkStop, err
			}
			analysis.SourceCitations = append(analysis.SourceCitations, refs...)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return Analysis{}, err
	}
	analysis.Sources = sortedUnique(analysis.Sources)
	analysis.Links = sortedUnique(analysis.Links)
	analysis.Anchors = sortedUnique(analysis.Anchors)
	analysis.LinkReferences = sortedUniqueReferences(analysis.LinkReferences)
	analysis.SourceReferences = sortedUniqueReferences(analysis.SourceReferences)
	analysis.SourceCitations = sortedUniqueReferences(analysis.SourceCitations)
	return analysis, nil
}
