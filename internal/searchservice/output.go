package searchservice

import (
	"github.com/javiermolinar/lumbrera/internal/cmdutil"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
)

type Output struct {
	Query                string                   `json:"query"`
	QueryMode            string                   `json:"query_mode"`
	RecommendedSections  []jsonRecommendedSection `json:"recommended_sections"`
	AgentInstructions    jsonAgentInstructions    `json:"agent_instructions"`
	Coverage             map[string]any           `json:"coverage"`
	Results              []jsonResult             `json:"results"`
	RecommendedReadOrder []string                 `json:"recommended_read_order"`
	StopRule             string                   `json:"stop_rule"`
}

type jsonAgentInstructions struct {
	ReadFirst string   `json:"read_first"`
	DoNot     []string `json:"do_not"`
	Fallback  string   `json:"fallback"`
}

type jsonRecommendedSection struct {
	SectionID string `json:"section_id"`
	Target    string `json:"target"`
	Path      string `json:"path"`
	Anchor    string `json:"anchor,omitempty"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Heading   string `json:"heading,omitempty"`
	Reason    string `json:"reason"`
}

type jsonResult struct {
	ID        string   `json:"id"`
	SectionID string   `json:"section_id"`
	Path      string   `json:"path"`
	Anchor    string   `json:"anchor,omitempty"`
	Kind      string   `json:"kind"`
	Tier      string   `json:"tier"`
	Title     string   `json:"title"`
	Heading   string   `json:"heading,omitempty"`
	Summary   string   `json:"summary"`
	Tags      []string `json:"tags"`
	Score     float64  `json:"score"`
	Snippet   string   `json:"snippet"`
	Sources   []string `json:"sources"`
	Links     []string `json:"links"`
}

func Project(response searchindex.SearchResponse) Output {
	payload := Output{
		Query:               response.Query,
		QueryMode:           response.QueryMode,
		RecommendedSections: make([]jsonRecommendedSection, 0, len(response.RecommendedSections)),
		AgentInstructions: jsonAgentInstructions{
			ReadFirst: response.AgentInstructions.ReadFirst,
			DoNot:     cmdutil.NonNilStrings(response.AgentInstructions.DoNot),
			Fallback:  response.AgentInstructions.Fallback,
		},
		Coverage:             jsonCoverage(response.Coverage),
		Results:              make([]jsonResult, 0, len(response.Results)),
		RecommendedReadOrder: cmdutil.NonNilStrings(response.RecommendedReadOrder),
		StopRule:             response.StopRule,
	}
	for _, section := range response.RecommendedSections {
		payload.RecommendedSections = append(payload.RecommendedSections, jsonRecommendedSection{
			SectionID: section.SectionID,
			Target:    section.Target,
			Path:      section.Path,
			Anchor:    section.Anchor,
			Kind:      section.Kind,
			Title:     section.Title,
			Heading:   section.Heading,
			Reason:    section.Reason,
		})
	}
	for _, result := range response.Results {
		payload.Results = append(payload.Results, jsonResult{
			ID:        result.DocumentID,
			SectionID: result.SectionID,
			Path:      result.Path,
			Anchor:    result.Anchor,
			Kind:      result.Kind,
			Tier:      result.Tier,
			Title:     result.Title,
			Heading:   result.Heading,
			Summary:   result.Summary,
			Tags:      cmdutil.NonNilStrings(result.Tags),
			Score:     result.Score,
			Snippet:   result.Snippet,
			Sources:   cmdutil.NonNilStrings(result.Sources),
			Links:     cmdutil.NonNilStrings(result.Links),
		})
	}
	return payload
}

func jsonCoverage(coverage searchindex.SearchCoverage) map[string]any {
	payload := make(map[string]any, len(coverage.Entities)+1)
	for key, value := range coverage.Entities {
		payload[key] = value
	}
	payload["missing"] = cmdutil.NonNilStrings(coverage.Missing)
	return payload
}
