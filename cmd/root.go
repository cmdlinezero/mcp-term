package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/example/mcp-term/internal"
	"github.com/spf13/cobra"
)

func Execute() {
	if err := newRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func newRoot() *cobra.Command {
	root := &cobra.Command{Use: "mcp-term", Short: "MCP terminal command runner and session replay", SilenceUsage: true, SilenceErrors: true}
	var serveLog string
	serve := &cobra.Command{Use: "serve", Short: "Start MCP JSON-RPC server on stdio", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		logger, err := internal.OpenLogger(serveLog)
		if err != nil {
			return err
		}
		defer logger.Close()
		return internal.Serve(context.Background(), os.Stdin, os.Stdout, logger)
	}}
	serve.Flags().StringVarP(&serveLog, "log", "l", "session.jsonl", "Session log file")
	var input, output string
	replay := &cobra.Command{Use: "replay", Short: "Replay commands from JSONL into a new JSONL file", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		a, err := filepath.Abs(input)
		if err != nil {
			return err
		}
		b, err := filepath.Abs(output)
		if err != nil {
			return err
		}
		if a == b {
			return fmt.Errorf("input and output log must differ")
		}
		entries, err := internal.ReadLogs(input)
		if err != nil {
			return err
		}
		// Exclusive creation prevents accidentally clobbering existing results.
		f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		f.Close()
		logger, err := internal.OpenLogger(output)
		if err != nil {
			return err
		}
		defer logger.Close()
		for i, entry := range entries {
			result := internal.Execute(context.Background(), entry.Command, entry.Args, internal.DefaultTimeout)
			if err := logger.Write(result); err != nil {
				return fmt.Errorf("replay entry %d: %w", i+1, err)
			}
		}
		fmt.Fprintf(os.Stderr, "Replayed %d commands to %s\n", len(entries), output)
		return nil
	}}
	replay.Flags().StringVarP(&input, "log", "l", "session.jsonl", "Source session log")
	replay.Flags().StringVarP(&output, "output", "o", "replay-session.jsonl", "New replay log (must not exist)")
	var exportLog, exportOutput string
	export := &cobra.Command{Use: "export-asciinema", Short: "Export recorded JSONL to asciicast v2 without executing commands", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		a, err := filepath.Abs(exportLog)
		if err != nil {
			return err
		}
		b, err := filepath.Abs(exportOutput)
		if err != nil {
			return err
		}
		if a == b {
			return fmt.Errorf("input and output must differ")
		}
		return internal.ExportAsciicast(a, b)
	}}
	export.Flags().StringVar(&exportLog, "log", "session.jsonl", "Source JSONL recording")
	export.Flags().StringVar(&exportOutput, "output", "session.cast", "New asciicast v2 output file")
	root.AddCommand(serve, replay, export)
	return root
}
