package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/RandyVentures/greenlight/internal/asc"
	"github.com/RandyVentures/greenlight/internal/checks"
	"github.com/RandyVentures/greenlight/internal/config"
	"github.com/RandyVentures/greenlight/internal/report"
	"github.com/spf13/cobra"
)

var (
	scanAppID     string
	scanBuildNum  string
	scanFormat    string
	scanOutput    string
	scanTier      int
	scanVersionID string
	scanPlatform  string
	scanExitCode  bool
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Run pre-submission checks against your app",
	Long: `Read-only App Store Connect checks for exactly one submission version.

Tier 1: metadata, all returned screenshot localizations, and the attached build.
Tier 2: metadata text rules and URL checks (no AI service).
Specify --version-id when multiple editable versions exist, or to inspect a
version already in review. --build asserts the attached build number.
Binary inspection is available separately through preflight --ipa.`,
	RunE: runScan,
}

func init() {
	scanCmd.Flags().StringVar(&scanAppID, "app-id", "", "App Store Connect app ID (required)")
	scanCmd.Flags().StringVar(&scanBuildNum, "build", "", "expected attached build number (never selects a different build)")
	scanCmd.Flags().StringVar(&scanFormat, "format", "terminal", "output format: terminal, json, junit")
	scanCmd.Flags().StringVar(&scanOutput, "output", "", "write report to file (stdout if omitted)")
	scanCmd.Flags().IntVar(&scanTier, "tier", 2, "max implemented tier to run (1-2)")
	scanCmd.Flags().StringVar(&scanVersionID, "version-id", "", "App Store version resource ID (auto-selects only one editable version)")
	scanCmd.Flags().StringVar(&scanPlatform, "platform", "", "optional platform filter: IOS, MAC_OS, TV_OS, VISION_OS")
	scanCmd.Flags().BoolVar(&scanExitCode, "exit-code", false, "fail on blockers or incomplete checks")
	scanCmd.MarkFlagRequired("app-id")
}

func runScan(cmd *cobra.Command, args []string) error {
	scanFormat = strings.ToLower(scanFormat)
	if scanFormat != "terminal" && scanFormat != "json" && scanFormat != "junit" {
		return fmt.Errorf("unsupported format: %s", scanFormat)
	}
	if scanTier < 1 || scanTier > 2 {
		return fmt.Errorf("scan --tier must be 1 or 2")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("not authenticated — run 'greenlight auth setup' first: %w", err)
	}

	// Init API client
	client, err := asc.NewClient(cfg.KeyID, cfg.IssuerID, cfg.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	// Run checks
	start := time.Now()
	runner := checks.NewRunner(client, verbose)
	runner.VersionID = scanVersionID
	runner.Platform = scanPlatform
	results, err := runner.Run(cmd.Context(), scanAppID, scanBuildNum, scanTier)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	elapsed := time.Since(start)

	// Generate report
	rep := report.New(results, elapsed)

	err = auditOutput(scanOutput, func(output io.Writer) error {
		switch scanFormat {
		case "json":
			return rep.WriteJSON(output)
		case "junit":
			return rep.WriteJUnit(output)
		default:
			return rep.WriteTerminal(output)
		}
	})
	if err != nil {
		return err
	}
	if scanExitCode && (!results.Summary.Passed || results.Incomplete) {
		return ErrThreshold
	}
	return nil
}
