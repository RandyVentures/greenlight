// Package audit provides evidence-based reports for the fork's local auditors.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
)

type Box struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Check struct {
	ID         string `json:"id"`
	Status     string `json:"status"`     // passed, failed, suspected, unverified
	Severity   string `json:"severity"`   // block, warn, info
	Confidence string `json:"confidence"` // high, medium, low, none
	Required   bool   `json:"required"`
	File       string `json:"file,omitempty"`
	Evidence   string `json:"evidence"`
	Guideline  string `json:"guideline,omitempty"`
	Fix        string `json:"fix,omitempty"`
	Regions    []Box  `json:"regions,omitempty"`
}

type Summary struct {
	Passed     int    `json:"passed"`
	Failed     int    `json:"failed"`
	Suspected  int    `json:"suspected"`
	Unverified int    `json:"unverified"`
	Checked    int    `json:"checked"`
	Applicable int    `json:"applicable"`
	Incomplete bool   `json:"incomplete"`
	Verdict    string `json:"verdict"`
}

type Report struct {
	Kind        string   `json:"kind"`
	Checks      []Check  `json:"checks"`
	Summary     Summary  `json:"summary"`
	Limitations []string `json:"limitations"`
}

func (r *Report) Compute() {
	r.Summary = Summary{Verdict: "no_blockers_found"}
	for _, c := range r.Checks {
		if c.Required {
			r.Summary.Applicable++
			if c.Status != "unverified" {
				r.Summary.Checked++
			}
		}
		switch c.Status {
		case "passed":
			r.Summary.Passed++
		case "failed":
			r.Summary.Failed++
		case "suspected":
			r.Summary.Suspected++
		case "unverified":
			r.Summary.Unverified++
			if c.Required {
				r.Summary.Incomplete = true
			}
		}
	}
	if r.Summary.Incomplete {
		r.Summary.Verdict = "incomplete"
	}
	if r.Summary.Failed > 0 || r.Summary.Suspected > 0 {
		r.Summary.Verdict = "needs_attention"
	}
}

// Gate fails on actionable findings or missing required evidence, not on
// deliberately out-of-scope runtime tests reported as optional/unverified.
func (r *Report) Gate() bool {
	return r.Summary.Failed > 0 || r.Summary.Suspected > 0 || r.Summary.Incomplete
}

func (r *Report) Write(w io.Writer, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}
	if _, err := fmt.Fprintf(w, "%s: %s\nCoverage: %d/%d applicable checks evaluated\n", r.Kind, r.Summary.Verdict, r.Summary.Checked, r.Summary.Applicable); err != nil {
		return err
	}
	for _, c := range r.Checks {
		if _, err := fmt.Fprintf(w, "[%s] %s (%s confidence) %s\n  %s\n", c.Status, c.ID, c.Confidence, c.File, c.Evidence); err != nil {
			return err
		}
		if c.Guideline != "" {
			fmt.Fprintln(w, "  "+c.Guideline)
		}
		if c.Fix != "" {
			fmt.Fprintln(w, "  Fix: "+c.Fix)
		}
	}
	for _, l := range r.Limitations {
		fmt.Fprintln(w, "Scope: "+l)
	}
	return nil
}
