// Package cmd implements the CLI entry point.
//
// report-studio-mcp takes no third-party dependencies (see CLAUDE.md), so the
// org's usual cobra scaffold is replaced with stdlib argument handling.
package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"

	"github.com/nlink-jp/report-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/report-studio-mcp/internal/tools"
	"github.com/nlink-jp/report-studio-mcp/internal/transport"
)

// Version is overridden at build time via -ldflags "-X .../cmd.Version=<vX.Y.Z>".
var Version = "dev"

// Execute parses os.Args and runs the CLI, exiting the process with the
// resulting status code.
func Execute() {
	os.Exit(Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// Run is the testable entry point behind Execute.
//
// With no arguments it serves MCP over stdin/stdout. `version` and
// `--version` print the version; both spellings must print the identical
// string because the shared homebrew formula template runs `--version` in
// its test block.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "version" {
		fmt.Fprintln(stdout, Version)
		return 0
	}
	if slices.Contains(args, "--version") || slices.Contains(args, "-version") {
		fmt.Fprintln(stdout, Version)
		return 0
	}
	// `serve` is the implicit default; accept it explicitly too so client
	// configs can spell out the intent.
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	if len(args) > 0 {
		fmt.Fprintf(stderr, "report-studio-mcp: unknown argument %q (usage: report-studio-mcp [serve] | --version)\n", args[0])
		return 2
	}
	return serve(stdin, stdout, stderr)
}

func serve(stdin io.Reader, stdout, stderr io.Writer) int {
	// MCP owns stdout; all logging goes to stderr.
	logger := slog.New(slog.NewTextHandler(stderr, nil))
	tr := transport.NewStdioTransport(stdin, stdout)
	srv := mcpserver.New("report-studio-mcp", Version, tr, logger)
	srv.SetInstructions(tools.Instructions)
	tools.RegisterAll(srv)

	if err := srv.Serve(context.Background()); err != nil {
		logger.Error("serve", "err", err)
		return 1
	}
	return 0
}
