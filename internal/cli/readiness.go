package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/RandyVentures/greenlight/internal/audit"
	"github.com/RandyVentures/greenlight/internal/history"
	"github.com/RandyVentures/greenlight/internal/readiness"
	"github.com/spf13/cobra"
)

func init() {
	var opts readiness.Options
	var format, output string
	var exit bool
	cmd := &cobra.Command{Use: "readiness", Short: "Run the portfolio release checklist with candidate-bound human evidence"}
	cmd.Flags().StringVar(&opts.Artifact, "artifact", "", "candidate .app, .xcarchive, or .ipa")
	cmd.Flags().StringVar(&opts.Metadata, "metadata", "", "local release metadata JSON")
	cmd.Flags().StringVar(&opts.Evidence, "evidence", "", "hash-bound human attestation JSON")
	cmd.Flags().StringSliceVar(&opts.Inputs, "input", nil, "additional release inputs to bind (repeat or comma-separate)")
	for _, flag := range []struct {
		name   string
		target *bool
	}{{"purchases", &opts.Features.Purchases}, {"subscriptions", &opts.Features.Subscriptions}, {"accounts", &opts.Features.Accounts}, {"public-content", &opts.Features.PublicContent}, {"third-party-content", &opts.Features.ThirdPartyContent}, {"cloud-sync", &opts.Features.CloudSync}, {"safety-sensitive", &opts.Features.SafetySensitive}} {
		cmd.Flags().BoolVar(flag.target, flag.name, false, "include the "+flag.name+" acceptance gate")
	}
	cmd.Flags().StringVar(&format, "format", "terminal", "terminal or json")
	cmd.Flags().StringVar(&output, "output", "", "new report file")
	cmd.Flags().BoolVar(&exit, "exit-code", false, "fail on blockers, suspected issues, or missing evidence")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if format != "terminal" && format != "json" {
			return fmt.Errorf("unsupported format")
		}
		r, err := readiness.Scan(opts)
		if err != nil {
			return err
		}
		err = auditOutput(output, func(w io.Writer) error {
			if format == "json" {
				e := json.NewEncoder(w)
				e.SetIndent("", "  ")
				return e.Encode(r)
			}
			fmt.Fprintf(w, "Ruleset: %s\nBinding: %s\n", r.Ruleset, r.Binding)
			return r.Report.Write(w, format)
		})
		if err != nil {
			return err
		}
		if exit && r.Gate() {
			return ErrThreshold
		}
		return nil
	}
	rootCmd.AddCommand(cmd)
	var stateDir, project string
	h := &cobra.Command{Use: "history", Short: "Keep a private local rejection log and reusable prevention checks"}
	h.PersistentFlags().StringVar(&stateDir, "state-dir", "", "private history directory (defaults to OS user config)")
	h.PersistentFlags().StringVar(&project, "project", ".", "project whose rejection history to use")
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, e := history.New(stateDir)
		if e != nil {
			return e
		}
		r, e := s.List(project)
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(r)
	}}
	var record history.Record
	add := &cobra.Command{Use: "add", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		s, e := history.New(stateDir)
		if e != nil {
			return e
		}
		r, e := s.Add(project, record)
		if e != nil {
			return e
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(r)
	}}
	add.Flags().StringVar(&record.Summary, "summary", "", "what was rejected")
	add.Flags().StringVar(&record.Version, "version", "", "app version reviewed")
	add.Flags().StringVar(&record.Build, "build", "", "build reviewed")
	add.Flags().StringVar(&record.RejectedAt, "review-date", "", "review date, YYYY-MM-DD")
	add.Flags().StringVar(&record.RootCause, "cause", "", "why the problem occurred")
	add.Flags().StringVar(&record.Prevention, "prevention", "", "how to check for this problem next time")
	add.Flags().StringVar(&record.Guideline, "guideline", "", "Apple guideline reference")
	add.Flags().StringVar(&record.RuleID, "rule", "", "exact current check ID to retest")
	add.Flags().StringVar(&record.Origin, "origin", "apple_review", "apple_review or internal_preflight")
	var resolution string
	resolve := &cobra.Command{Use: "resolve <id>", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, e := history.New(stateDir)
		if e != nil {
			return e
		}
		return s.Resolve(project, args[0], resolution)
	}}
	resolve.Flags().StringVar(&resolution, "fix", "", "the fix applied; future audits still retest it")
	var historyExit bool
	check := &cobra.Command{Use: "check <report.json>", Short: "Retest history against a current audit, readiness, or UI report", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		data, e := os.ReadFile(args[0])
		if e != nil {
			return e
		}
		var report struct {
			Checks []audit.Check `json:"checks"`
			Rows   []audit.Check `json:"rows"`
		}
		if e := json.Unmarshal(data, &report); e != nil {
			return e
		}
		if report.Checks == nil && report.Rows == nil {
			return fmt.Errorf("supply a current audit, readiness, or UI JSON report containing checks or rows")
		}
		store, e := history.New(stateDir)
		if e != nil {
			return e
		}
		records, e := store.List(project)
		if e != nil {
			return e
		}
		result := history.Check(records, append(report.Checks, report.Rows...))
		if e := json.NewEncoder(cmd.OutOrStdout()).Encode(result); e != nil {
			return e
		}
		if historyExit && result.Gate() {
			return ErrThreshold
		}
		return nil
	}}
	check.Flags().BoolVar(&historyExit, "exit-code", false, "fail when previous issues recur or cannot be retested")
	h.AddCommand(list, add, resolve, check)
	rootCmd.AddCommand(h)
}
