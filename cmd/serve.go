package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rabidclock/localfreshllm/server"
)

var (
	serveAddr string
	serveKey  string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the LocalFresh API server",
	Long:  "Start an HTTP server that provides chat, model listing, and device management via SSE streaming.",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", "0.0.0.0:8400", "Listen address")
	serveCmd.Flags().StringVar(&serveKey, "key", "", "Master registration key (or LOCALFRESH_MASTER_KEY env)")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	key := serveKey
	if key == "" {
		key = os.Getenv("LOCALFRESH_MASTER_KEY")
	}
	if key == "" {
		return fmt.Errorf("master key required: use --key or set LOCALFRESH_MASTER_KEY")
	}

	srv := server.New(serveAddr, key)
	return srv.Run()
}
