// Package tools registers report-studio-mcp's MCP tools.
package tools

import (
	"context"
	"encoding/json"

	"github.com/nlink-jp/report-studio-mcp/internal/mcpserver"
	"github.com/nlink-jp/report-studio-mcp/internal/workdir"
)

// Instructions is the hint initialize hands the client's model before any
// tool list. It names the work_dir contract (organization ADR-021).
const Instructions = "report-studio-mcp typesets a Markdown report manuscript into a PDF in your work directory. " +
	"Every call takes work_dir: the absolute path of a directory you can read back (your session or working directory); " +
	"it is required and has no default. Write the manuscript as a file under work_dir, then call render with its path; " +
	"the PDF is written to <work_dir>/<workspace_id>/out/ and returned as a path. Call get_usage first for the full reference."

// RegisterAll registers every implemented tool on the server.
func RegisterAll(s *mcpserver.Server, resolver workdir.Resolver) {
	s.RegisterTool(mcpserver.Tool{
		Name:        "get_usage",
		Description: "Returns the full reference for this server: the manuscript format, chart JSON, profiles, limits, and the error and warning table with recovery.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, getUsage)
	r := renderer{resolver: resolver}
	s.RegisterTool(mcpserver.Tool{
		Name: "render",
		Description: "Typesets a Markdown manuscript file under work_dir into a PDF at <work_dir>/<workspace_id>/out/<source path>.pdf. " +
			"Returns the output paths, the page count and warnings with manuscript line numbers. Errors only when the output would be wrong.",
		InputSchema: json.RawMessage(renderSchema),
	}, r.render)
}

func getUsage(context.Context, json.RawMessage) (any, error) {
	return usage, nil
}
