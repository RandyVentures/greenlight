package localui

import (
	"context"
	"fmt"
	"io"

	"github.com/RandyVentures/greenlight/internal/audit"
)

// RunAudit uses the same input validation, scanners, evidence binding and history
// retests as the UI. It never starts Serve, creates a listener, or opens a browser.
func RunAudit(ctx context.Context, workspace string, selection Selection, stateDir string) (*Job, error) {
	s, err := New(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if stateDir != "" {
		if err := s.SetHistoryDir(stateDir); err != nil {
			return nil, err
		}
	}
	selection, err = s.prepare(selection)
	if err != nil {
		return nil, err
	}
	id, err := s.createJob(selection)
	if err != nil {
		return nil, err
	}
	s.run(id, selection)
	return s.jobs[id], nil
}

// PlanAudit validates selected input paths without running scanners or contacting
// services. As in the UI, every selected existing path must be in the workspace.
func PlanAudit(ctx context.Context, workspace string, selection Selection) ([]Step, error) {
	s, err := New(ctx, workspace)
	if err != nil {
		return nil, err
	}
	selection, err = s.prepare(selection)
	if err != nil {
		return nil, err
	}
	return Plan(selection)
}

func (j *Job) HasExecutionErrors() bool {
	for _, s := range j.Steps {
		if s.State == "error" {
			return true
		}
	}
	return false
}
func (j *Job) Gate() bool {
	return j.Summary.Failed > 0 || j.Summary.Suspected > 0 || j.Summary.Incomplete
}
func (j *Job) WriteReport(w io.Writer, format string) error {
	if format == "html" {
		return writeHTML(w, j)
	}
	if format != "terminal" {
		return fmt.Errorf("unsupported report format: %s", format)
	}
	fmt.Fprintln(w, j.Scope)
	if j.ReleaseBinding != "" {
		fmt.Fprintf(w, "Ruleset: %s\nBinding: %s\n", j.Ruleset, j.ReleaseBinding)
	}
	r := audit.Report{Kind: "greenlight-audit", Summary: j.Summary, Limitations: j.Limitations}
	for _, row := range j.Rows {
		r.Checks = append(r.Checks, row.Check)
	}
	return r.Write(w, "terminal")
}
