// DevOpsClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 DevOpsClaw contributors

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/freitascorp/devopsclaw/pkg/logger"
	"github.com/freitascorp/devopsclaw/pkg/tools"
)

const (
	// ProtocolVersion is the MCP spec version this server supports.
	ProtocolVersion = "2024-11-05"
	ServerName      = "devopsclaw"
	ServerVersion   = "1.0.0"
)

// Server implements a stdio-based MCP server that exposes a ToolRegistry.
type Server struct {
	registry *tools.ToolRegistry
	in       io.Reader
	out      io.Writer
	mu       sync.Mutex // serializes writes to stdout
}

// NewServer creates an MCP server backed by the given tool registry.
// It reads JSON-RPC from stdin and writes responses to stdout.
func NewServer(registry *tools.ToolRegistry) *Server {
	return &Server{
		registry: registry,
		in:       os.Stdin,
		out:      os.Stdout,
	}
}

// NewServerWithIO creates an MCP server with custom I/O (for testing).
func NewServerWithIO(registry *tools.ToolRegistry, in io.Reader, out io.Writer) *Server {
	return &Server{
		registry: registry,
		in:       in,
		out:      out,
	}
}

// Serve runs the MCP server loop, reading requests until EOF or ctx cancellation.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	// MCP messages can be large (tool results), increase buffer.
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, ErrParse, "parse error: "+err.Error())
			continue
		}

		s.handleRequest(ctx, &req)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stdin read error: %w", err)
	}
	return nil
}

// handleRequest dispatches a single JSON-RPC request.
func (s *Server) handleRequest(ctx context.Context, req *Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// Client ack — nothing to do.
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	case "ping":
		s.sendResult(req.ID, map[string]any{})
	default:
		// Unknown method — if it has an ID it expects a response.
		if req.ID != nil {
			s.sendError(req.ID, ErrNotFound, "method not found: "+req.Method)
		}
		// Notifications (no ID) are silently ignored per spec.
	}
}

// ── Method handlers ────────────────────────────────────────────────

func (s *Server) handleInitialize(req *Request) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: ServerCapability{
			Tools: &ToolsCapability{ListChanged: false},
		},
		ServerInfo: EntityInfo{
			Name:    ServerName,
			Version: ServerVersion,
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *Request) {
	defs := s.registry.ToProviderDefs()

	mcpTools := make([]ToolInfo, 0, len(defs))
	for _, d := range defs {
		// Convert provider ToolDefinition → MCP ToolInfo.
		// The Parameters() output already follows JSON Schema, which is
		// exactly what MCP's inputSchema expects.
		inputSchema := d.Function.Parameters
		if inputSchema == nil {
			inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		mcpTools = append(mcpTools, ToolInfo{
			Name:        d.Function.Name,
			Description: d.Function.Description,
			InputSchema: inputSchema,
		})
	}

	s.sendResult(req.ID, ToolsListResult{Tools: mcpTools})
}

func (s *Server) handleToolsCall(ctx context.Context, req *Request) {
	// Parse params.
	raw, err := json.Marshal(req.Params)
	if err != nil {
		s.sendError(req.ID, ErrInternal, "failed to marshal params")
		return
	}

	var params ToolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		s.sendError(req.ID, ErrInvalidReq, "invalid tools/call params: "+err.Error())
		return
	}

	if params.Name == "" {
		s.sendError(req.ID, ErrInvalidReq, "tool name is required")
		return
	}

	logger.InfoCF("mcp", "Tool call",
		map[string]any{"tool": params.Name})

	// Execute via the registry.
	result := s.registry.Execute(ctx, params.Name, params.Arguments)

	// Build MCP response.
	text := result.ForLLM
	if text == "" {
		text = result.ForUser
	}
	if text == "" {
		text = "(no output)"
	}

	mcpResult := ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
		IsError: result.IsError,
	}
	s.sendResult(req.ID, mcpResult)
}

// ── Wire helpers ───────────────────────────────────────────────────

func (s *Server) sendResult(id any, result any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.writeJSON(resp)
}

func (s *Server) sendError(id any, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message},
	}
	s.writeJSON(resp)
}

func (s *Server) writeJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		// Last-resort: log and drop.
		logger.ErrorCF("mcp", "Failed to marshal response",
			map[string]any{"error": err.Error()})
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// MCP stdio transport: one JSON object per line.
	_, _ = s.out.Write(data)
	_, _ = s.out.Write([]byte("\n"))
}
