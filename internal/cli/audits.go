package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/RandyVentures/greenlight/internal/purchases"
	"github.com/RandyVentures/greenlight/internal/screenshots"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newScreenshotsCommand(), newPurchasesCommand())
}

// Refusing to overwrite files prevents a report path from destroying an input
// screenshot, source file, or catalog. Existing files are never modified.
func auditOutput(path string, write func(io.Writer) error) error {
	if path == "" {
		return write(os.Stdout)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}

func newScreenshotsCommand() *cobra.Command {
	var format, output, displayType, language string
	var exitCode bool
	cmd := &cobra.Command{Use: "screenshots <file-or-directory>", Short: "Audit local screenshots with offline OCR and evidence highlights", Args: cobra.ExactArgs(1)}
	cmd.Flags().StringVar(&format, "format", "terminal", "terminal, json, or html")
	cmd.Flags().StringVar(&output, "output", "", "create a new report file (never overwrite inputs)")
	cmd.Flags().StringVar(&displayType, "display-type", "", "ASC display type, e.g. APP_IPHONE_67; omitted means dimensions unverified")
	cmd.Flags().StringVar(&language, "ocr-language", "eng", "installed Tesseract language(s), e.g. eng+spa")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "fail on suspected issues, failed checks, or incomplete required checks")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if format != "terminal" && format != "json" && format != "html" {
			return fmt.Errorf("unsupported format: %s", format)
		}
		r, err := screenshots.Scan(cmd.Context(), args[0], screenshots.Options{DisplayType: displayType, Language: language})
		if err != nil {
			return err
		}
		err = auditOutput(output, func(w io.Writer) error {
			if format == "html" {
				return r.WriteHTML(w)
			}
			if format == "json" {
				e := json.NewEncoder(w)
				e.SetIndent("", "  ")
				return e.Encode(r)
			}
			return r.Report.Write(w, format)
		})
		if err != nil {
			return err
		}
		if exitCode && r.Gate() {
			return ErrThreshold
		}
		return nil
	}
	return cmd
}

func newPurchasesCommand() *cobra.Command {
	var format, output string
	var exitCode bool
	cmd := &cobra.Command{Use: "purchases <config.json>", Short: "Compare product IDs and entitlement mappings using local evidence", Args: cobra.ExactArgs(1)}
	cmd.Flags().StringVar(&format, "format", "terminal", "terminal or json")
	cmd.Flags().StringVar(&output, "output", "", "create a new report file")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "fail on mismatches, warnings, or missing required evidence")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if format != "terminal" && format != "json" {
			return fmt.Errorf("unsupported format: %s", format)
		}
		r, err := purchases.Scan(args[0])
		if err != nil {
			return err
		}
		if err := auditOutput(output, func(w io.Writer) error { return r.Write(w, format) }); err != nil {
			return err
		}
		if exitCode && r.Gate() {
			return ErrThreshold
		}
		return nil
	}
	return cmd
}
