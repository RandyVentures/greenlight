package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/RandyVentures/greenlight/internal/localui"
	"github.com/RandyVentures/greenlight/internal/purchases"
	"github.com/spf13/cobra"
)

type auditConfig struct {
	SchemaVersion int                `json:"schema_version"`
	Workspace     string             `json:"workspace"`
	Selection     *localui.Selection `json:"selection"`
}
type auditEnvelope struct {
	SchemaVersion int                `json:"schema_version"`
	Kind          string             `json:"kind"`
	ToolVersion   string             `json:"tool_version"`
	Workspace     string             `json:"workspace"`
	Selection     *localui.Selection `json:"selection"`
	Plan          []localui.Step     `json:"plan,omitempty"`
	*localui.Job  `json:",omitempty"`
}

// Only the new agent workflow uses exit 2. Existing command exit behavior stays
// compatible. Reports with findings use ErrThreshold (exit 1).
type auditCommandError struct{ err error }

func (e *auditCommandError) Error() string { return e.err.Error() }
func (e *auditCommandError) Unwrap() error { return e.err }
func (e *auditCommandError) ExitCode() int { return 2 }
func agentError(err error) error {
	if err == nil || errors.Is(err, ErrThreshold) {
		return err
	}
	return &auditCommandError{err}
}

// ExitCode supports command-specific codes without changing legacy commands.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return coded.ExitCode()
	}
	return 1
}
func init() { rootCmd.AddCommand(newAuditCommand()) }

func defaultAuditConfig() auditConfig {
	return auditConfig{SchemaVersion: 1, Workspace: ".", Selection: &localui.Selection{Project: ".", Platform: "ios", Payments: "none", Language: "eng", DisplayType: "APP_IPHONE_67", Readiness: true, History: true, Products: []purchases.Product{}, SourceFiles: []string{}, Locales: []string{}}}
}
func loadAuditConfig(path string) (auditConfig, error) {
	cfg := auditConfig{Workspace: "."}
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return cfg, err
	}
	if len(b) > 1<<20 {
		return cfg, fmt.Errorf("audit configuration exceeds 1 MiB")
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if err := d.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("invalid audit config: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return cfg, fmt.Errorf("audit config must contain exactly one JSON document")
	}
	if cfg.SchemaVersion != 1 {
		return cfg, fmt.Errorf("unsupported audit schema_version %d; expected 1", cfg.SchemaVersion)
	}
	if cfg.Selection == nil {
		return cfg, fmt.Errorf("selection must be an object")
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "."
	}
	if !filepath.IsAbs(cfg.Workspace) {
		cfg.Workspace = filepath.Join(filepath.Dir(path), cfg.Workspace)
	}
	cfg.Workspace, err = filepath.Abs(cfg.Workspace)
	if err != nil {
		return cfg, err
	}
	if cfg.Selection.Project == "" {
		cfg.Selection.Project = "."
	}
	if cfg.Selection.Payments == "" {
		cfg.Selection.Payments = "none"
	}
	if cfg.Selection.Language == "" {
		cfg.Selection.Language = "eng"
	}
	return cfg, nil
}
func commandOutput(cmd *cobra.Command, path string, write func(io.Writer) error) error {
	if path == "" {
		return write(cmd.OutOrStdout())
	}
	return auditOutput(path, write)
}
func noAgentArgs(cmd *cobra.Command, args []string) error { return agentError(cobra.NoArgs(cmd, args)) }
func newAuditCommand() *cobra.Command {
	var configPath, format, output, stateDir string
	var plan, allowOnline, exitCode, includeImages bool
	cmd := &cobra.Command{Use: "audit", Short: "Run a configured audit headlessly for agents and CI; UI is optional", Args: noAgentArgs, SilenceUsage: true, SilenceErrors: true}
	cmd.Flags().StringVar(&configPath, "config", "greenlight.audit.json", "versioned audit configuration JSON")
	cmd.Flags().StringVar(&format, "format", "json", "json, terminal, or html")
	cmd.Flags().StringVar(&output, "output", "", "new report file; stdout when omitted")
	cmd.Flags().StringVar(&stateDir, "state-dir", "", "private history directory; defaults to OS user config")
	cmd.Flags().BoolVar(&includeImages, "include-images", false, "include local screenshot thumbnails in JSON; HTML includes them automatically")
	cmd.Flags().BoolVar(&plan, "plan", false, "validate inputs and print the plan without running checks")
	cmd.Flags().BoolVar(&allowOnline, "allow-online", false, "permit an explicitly configured live ASC step for this run")
	cmd.Flags().BoolVar(&exitCode, "exit-code", true, "exit 1 for findings or missing required evidence; false emits reports without this gate")
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error { return agentError(err) })
	cmd.RunE = func(cmd *cobra.Command, args []string) (err error) {
		defer func() { err = agentError(err) }()
		if format != "json" && format != "terminal" && format != "html" {
			return fmt.Errorf("unsupported format: %s", format)
		}
		if plan && format == "html" {
			return fmt.Errorf("--plan supports json or terminal")
		}
		// Refuse known output collisions before doing expensive work; creation still
		// uses O_EXCL afterwards so a concurrent writer cannot be overwritten.
		if output != "" {
			if _, e := os.Lstat(output); e == nil {
				return fmt.Errorf("output already exists: %s", output)
			} else if !os.IsNotExist(e) {
				return e
			}
		}
		cfg, err := loadAuditConfig(configPath)
		if err != nil {
			return err
		}
		envelope := auditEnvelope{SchemaVersion: 1, Kind: "greenlight-audit", ToolVersion: resolveVersion(), Workspace: cfg.Workspace, Selection: cfg.Selection}
		if plan {
			envelope.Kind = "greenlight-audit-plan"
			envelope.Plan, err = localui.PlanAudit(cmd.Context(), cfg.Workspace, *cfg.Selection)
			if err != nil {
				return err
			}
		} else {
			if cfg.Selection.LiveASC && !allowOnline {
				return fmt.Errorf("live_asc requires explicit --allow-online; no services were contacted")
			}
			envelope.Job, err = localui.RunAudit(cmd.Context(), cfg.Workspace, *cfg.Selection, stateDir)
			if err != nil {
				return err
			}
			if format == "json" && !includeImages {
				envelope.Job.Images = []localui.Preview{}
			}
		}
		if err := commandOutput(cmd, output, func(w io.Writer) error {
			if format == "json" {
				e := json.NewEncoder(w)
				e.SetIndent("", "  ")
				return e.Encode(envelope)
			}
			if plan {
				for _, s := range envelope.Plan {
					fmt.Fprintf(w, "%s · %s · %s\n", s.ID, s.Mode, s.Detail)
				}
				return nil
			}
			return envelope.Job.WriteReport(w, format)
		}); err != nil {
			return err
		}
		if !plan {
			if envelope.Job.HasExecutionErrors() {
				return fmt.Errorf("one or more audit steps could not execute; inspect steps and unverified rows in the report")
			}
			if exitCode && envelope.Job.Gate() {
				return ErrThreshold
			}
		}
		return nil
	}
	initCmd := &cobra.Command{Use: "init [path]", Short: "Write a starter config without overwriting a file", Args: func(cmd *cobra.Command, args []string) error { return agentError(cobra.MaximumNArgs(1)(cmd, args)) }}
	initCmd.RunE = func(cmd *cobra.Command, args []string) error {
		path := "greenlight.audit.json"
		if len(args) == 1 {
			path = args[0]
		}
		err := auditOutput(path, func(w io.Writer) error {
			e := json.NewEncoder(w)
			e.SetIndent("", "  ")
			return e.Encode(defaultAuditConfig())
		})
		if err != nil {
			return agentError(err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), path)
		return nil
	}
	schemaCmd := &cobra.Command{Use: "schema", Short: "Print the version 1 JSON config schema for agents", Args: noAgentArgs, RunE: func(cmd *cobra.Command, args []string) error {
		e := json.NewEncoder(cmd.OutOrStdout())
		e.SetIndent("", "  ")
		return agentError(e.Encode(auditSchema()))
	}}
	cmd.AddCommand(initCmd, schemaCmd)
	return cmd
}

// Reflect the shared selection type so field names/types cannot drift from the
// UI/runner. Conditional validation still runs in the shared Go validator.
func auditSchema() map[string]any {
	selection := typeSchema(reflect.TypeOf(localui.Selection{}))
	selection["required"] = []string{"platform"}
	props := selection["properties"].(map[string]any)
	props["platform"].(map[string]any)["enum"] = []string{"ios", "macos"}
	props["payments"].(map[string]any)["enum"] = []string{"none", "storekit", "revenuecat"}
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "title": "Greenlight audit configuration v1", "type": "object", "additionalProperties": false, "required": []string{"schema_version", "selection"}, "properties": map[string]any{"schema_version": map[string]any{"const": 1}, "workspace": map[string]any{"type": "string", "default": "."}, "selection": selection}}
}
func typeSchema(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice:
		return map[string]any{"type": "array", "items": typeSchema(t.Elem())}
	case reflect.Struct:
		props := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				props[name] = typeSchema(f.Type)
			}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
	}
	return map[string]any{}
}
