package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/RandyVentures/greenlight/internal/localui"
	"github.com/spf13/cobra"
)

func init() {
	var port int
	var noOpen bool
	cmd := &cobra.Command{Use: "ui [workspace]", Short: "Open the lightweight local React audit workspace", Args: cobra.MaximumNArgs(1)}
	cmd.Flags().IntVar(&port, "port", 0, "loopback port (0 chooses an available port)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print the local URL without opening a browser")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		s, err := localui.New(ctx, root)
		if err != nil {
			return err
		}
		return s.Serve(port, func(url string) {
			fmt.Fprintf(cmd.OutOrStdout(), "\n  Greenlight · local workspace\n  %s\n\n  Press Ctrl+C to stop. Reports stay in memory until you export them.\n", url)
			if noOpen {
				return
			}
			var opener *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				opener = exec.Command("open", url)
			case "windows":
				opener = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
			default:
				opener = exec.Command("xdg-open", url)
			}
			if err := opener.Start(); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "Open the printed URL in your browser to continue.")
			} else {
				go opener.Wait()
			}
		})
	}
	rootCmd.AddCommand(cmd)
}
