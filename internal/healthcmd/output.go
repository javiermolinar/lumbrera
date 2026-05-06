package healthcmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/javiermolinar/lumbrera/internal/cmdutil"
	"github.com/javiermolinar/lumbrera/internal/searchindex"
)

type jsonOutput struct {
	Candidates []jsonCandidate `json:"candidates"`
	StopRule   string          `json:"stop_rule"`
}

type jsonCandidate struct {
	Type              string       `json:"type"`
	Confidence        string       `json:"confidence"`
	Score             float64      `json:"score"`
	Pages             []string     `json:"pages"`
	Sources           []string     `json:"sources"`
	Reasons           []jsonReason `json:"reasons"`
	SuggestedQueries  []string     `json:"suggested_queries"`
	ReviewInstruction string       `json:"review_instruction"`
}

type jsonReason struct {
	Code  string `json:"code"`
	Value string `json:"value,omitempty"`
}

func writeJSON(out io.Writer, response searchindex.CandidateResponse) error {
	payload := jsonOutput{
		Candidates: make([]jsonCandidate, 0, len(response.Candidates)),
		StopRule:   response.StopRule,
	}
	for _, candidate := range response.Candidates {
		item := jsonCandidate{
			Type:              candidate.Type,
			Confidence:        candidate.Confidence,
			Score:             candidate.Score,
			Pages:             cmdutil.NonNilStrings(candidate.Pages),
			Sources:           cmdutil.NonNilStrings(candidate.Sources),
			Reasons:           make([]jsonReason, 0, len(candidate.Reasons)),
			SuggestedQueries:  cmdutil.NonNilStrings(candidate.SuggestedQueries),
			ReviewInstruction: candidate.ReviewInstruction,
		}
		for _, reason := range candidate.Reasons {
			item.Reasons = append(item.Reasons, jsonReason{Code: reason.Code, Value: reason.Value})
		}
		payload.Candidates = append(payload.Candidates, item)
	}
	return cmdutil.WriteJSON(out, payload)
}

func writeHuman(out io.Writer, response searchindex.CandidateResponse) error {
	if len(response.Candidates) == 0 {
		_, err := fmt.Fprintf(out, "No health candidates found.\nstop_rule: %s\n", response.StopRule)
		return err
	}
	for i, candidate := range response.Candidates {
		if _, err := fmt.Fprintf(out, "%d. %s %s score=%.3f\n", i+1, candidate.Type, candidate.Confidence, candidate.Score); err != nil {
			return err
		}
		if len(candidate.Pages) > 0 {
			if _, err := fmt.Fprintf(out, "   pages: %s\n", strings.Join(candidate.Pages, ", ")); err != nil {
				return err
			}
		}
		if len(candidate.Sources) > 0 {
			if _, err := fmt.Fprintf(out, "   sources: %s\n", strings.Join(candidate.Sources, ", ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(out, "   reasons: %s\n", formatReasons(candidate.Reasons)); err != nil {
			return err
		}
		if len(candidate.SuggestedQueries) > 0 {
			if _, err := fmt.Fprintf(out, "   next: %s; %s\n", formatSuggestedQueries(candidate.SuggestedQueries), candidate.ReviewInstruction); err != nil {
				return err
			}
		} else if candidate.ReviewInstruction != "" {
			if _, err := fmt.Fprintf(out, "   next: %s\n", candidate.ReviewInstruction); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintf(out, "\nstop_rule: %s\n", response.StopRule)
	return err
}

func formatReasons(reasons []searchindex.CandidateReason) string {
	if len(reasons) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		if reason.Value == "" {
			parts = append(parts, reason.Code)
			continue
		}
		parts = append(parts, reason.Code+"="+reason.Value)
	}
	return strings.Join(parts, ", ")
}

func formatSuggestedQueries(queries []string) string {
	parts := make([]string, 0, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("search %q", query))
	}
	return strings.Join(parts, "; ")
}
