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
	"path/filepath"
	"slices"
	"syscall"

	"github.com/nlink-jp/pathguard"

	"github.com/nlink-jp/report-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/report-studio-mcp/internal/tools"
	"github.com/nlink-jp/report-studio-mcp/internal/transport"
	"github.com/nlink-jp/report-studio-mcp/internal/workdir"
)

// Version is overridden at build time via -ldflags "-X .../cmd.Version=<vX.Y.Z>".
var Version = "dev"

// Execute parses os.Args and runs the CLI, exiting the process with the
// resulting status code.
func Execute() {
	out := io.Writer(os.Stdout)
	if serving(os.Args[1:]) {
		f, err := isolateStdout()
		if err != nil {
			fmt.Fprintln(os.Stderr, "report-studio-mcp: cannot isolate stdout:", err)
			os.Exit(1)
		}
		out = f
	}
	os.Exit(Run(os.Args[1:], os.Stdin, out, os.Stderr))
}

// serving reports whether the arguments start the MCP server.
func serving(args []string) bool {
	return len(args) == 0 || (len(args) == 1 && args[0] == "serve")
}

// isolateStdout keeps the JSON-RPC stream for this process alone. Core Text,
// Core Graphics and ImageIO run inside the process, and anything native code
// prints to file descriptor 1 would corrupt the stream (it does not today:
// phase 0, P0-1; this is the second layer). The real stdout is duplicated to a
// private descriptor the transport writes, and descriptor 1 is pointed at
// stderr, so a stray print lands in the log instead.
func isolateStdout() (*os.File, error) {
	saved, err := syscall.Dup(int(os.Stdout.Fd()))
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(saved)
	if err := syscall.Dup2(int(os.Stderr.Fd()), int(os.Stdout.Fd())); err != nil {
		_ = syscall.Close(saved)
		return nil, err
	}
	return os.NewFile(uintptr(saved), "mcp-stdout"), nil
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
	tools.RegisterAll(srv, workdir.NewResolverFor(ownPlaces()...))

	if err := srv.Serve(context.Background()); err != nil {
		logger.Error("serve", "err", err)
		return 1
	}
	return 0
}

// ownPlaces are this server's own directories, which a work directory may not
// be and a manuscript may not name: its configuration, wherever it is looked
// up.
func ownPlaces() []pathguard.Place {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	dirs := []string{
		filepath.Join(home, ".config", "report-studio-mcp"),
		filepath.Join(home, "Library", "Application Support", "report-studio-mcp"),
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(x) {
		dirs = append(dirs, filepath.Join(x, "report-studio-mcp"))
	}
	var out []pathguard.Place
	for _, d := range dirs {
		out = append(out, pathguard.Place{Path: d, Kind: pathguard.Protected, Reason: "server_config",
			Why: "it is report-studio-mcp's own configuration directory"})
	}
	return out
}
