package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cmdlinezero/mcp-term/internal"
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
	var labInput, labOutput, labLog string
	var labBindOnly bool
	lab := &cobra.Command{Use: "lab-import", Short: "Execute Markdown terminal fences into JSONL (optionally bind Markdown)", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		if labInput == "" {
			return fmt.Errorf("--input is required")
		}
		if labBindOnly && labOutput == "" {
			return fmt.Errorf("--bind-only requires --output")
		}
		inAbs, err := filepath.Abs(labInput)
		if err != nil {
			return err
		}
		logAbs, err := filepath.Abs(labLog)
		if err != nil {
			return err
		}
		if inAbs == logAbs {
			return fmt.Errorf("input Markdown and JSONL log must differ")
		}
		if labOutput != "" {
			outAbs, err := filepath.Abs(labOutput)
			if err != nil {
				return err
			}
			if outAbs == inAbs || outAbs == logAbs {
				return fmt.Errorf("input, output and log must be different files")
			}
			if _, err := os.Stat(labOutput); err == nil {
				return fmt.Errorf("output already exists: %s", labOutput)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		data, err := os.ReadFile(labInput)
		if err != nil {
			return err
		}
		commands, err := internal.ParseLab(data)
		if err != nil {
			return err
		}
		if len(commands) == 0 {
			return fmt.Errorf("no terminal=true bash/sh fences found")
		}
		var matched []internal.CommandLog
		if labBindOnly {
			entries, err := internal.ReadLogs(labLog)
			if err != nil {
				return err
			}
			matched, err = internal.BindLab(commands, entries)
			if err != nil {
				return err
			}
		} else {
			// The generated recording is a new artifact, never silently append to
			// an existing session or overwrite a previously generated log.
			f, err := os.OpenFile(labLog, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return fmt.Errorf("create recording: %w", err)
			}
			if err := f.Close(); err != nil {
				return err
			}
			logger, err := internal.OpenLogger(labLog)
			if err != nil {
				return err
			}
			matched, err = internal.ExecuteLab(context.Background(), commands, logger)
			closeErr := logger.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
		if labOutput != "" {
			rewritten, err := internal.RewriteLab(data, commands, matched, labLog)
			if err != nil {
				return err
			}
			if err := internal.WriteLabExclusive(labInput, labOutput, rewritten); err != nil {
				return err
			}
		}
		failures := 0
		for _, e := range matched {
			if e.ExitCode != 0 {
				failures++
			}
		}
		if labBindOnly {
			fmt.Fprintf(os.Stderr, "Bound %d terminal fences to %s (no execution)\n", len(matched), labOutput)
		} else {
			fmt.Fprintf(os.Stderr, "Recorded %d terminal commands to %s (%d nonzero exits)\n", len(matched), labLog, failures)
			if labOutput != "" {
				fmt.Fprintf(os.Stderr, "Wrote bound Markdown to %s\n", labOutput)
			}
		}
		if failures > 0 && !labBindOnly {
			return fmt.Errorf("%d of %d commands exited nonzero; recordings preserved", failures, len(matched))
		}
		return nil
	}}
	lab.Flags().StringVar(&labInput, "input", "", "Source Markdown lab (required)")
	lab.Flags().StringVar(&labOutput, "output", "", "Optional new Markdown with recording references (must not exist)")
	lab.Flags().StringVar(&labLog, "log", "session.jsonl", "JSONL recording output (must not exist unless --bind-only)")
	lab.Flags().BoolVar(&labBindOnly, "bind-only", false, "Bind existing recordings without executing commands (requires --output)")
	root.AddCommand(serve, replay, export, lab)
	return root
}
