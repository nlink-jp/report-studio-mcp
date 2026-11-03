// Package tools registers report-studio-mcp's MCP tools.
package tools

import (
	"context"
	"encoding/json"

	"github.com/nlink-jp/report-studio-mcp/internal/mcpserver"
)

// Instructions is the hint initialize hands the client's model before any
// tool list.
const Instructions = "report-studio-mcp turns a Markdown report manuscript into a typeset PDF and an editable Word document (.docx). " +
	"Call get_usage first for the full reference."

// usage is get_usage's text. It grows with the tools; until render exists it
// says so, so a caller is not left guessing why there is nothing to call.
const usage = `report-studio-mcp — reference

This build has no render tool yet: the server is under development (see the RFP in docs/).
get_usage is the only tool.
`

// RegisterAll registers every implemented tool on the server.
func RegisterAll(s *mcpserver.Server) {
	s.RegisterTool(mcpserver.Tool{
		Name:        "get_usage",
		Description: "Returns the full reference for this server: what it accepts, its tools, limits, and the error and warning table with recovery.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, getUsage)
}

func getUsage(context.Context, json.RawMessage) (any, error) {
	return usage, nil
}
