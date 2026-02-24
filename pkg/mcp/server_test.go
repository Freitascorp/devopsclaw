// DevOpsClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 DevOpsClaw contributors

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/freitascorp/devopsclaw/pkg/tools"
)

// mockTool implements tools.Tool for testing.
type mockTool struct {
	name   string
	desc   string
	result *tools.ToolResult
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string  { return m.desc }
func (m *mockTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"arg1": map[string]any{"type": "string", "description": "An argument"},
		},
		"required": []string{"arg1"},
	}
}
func (m *mockTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	return m.result
}

func newTestRegistry() *tools.ToolRegistry {
	reg := tools.NewToolRegistry()
	reg.Register(&mockTool{
		name:   "echo",
		desc:   "Echoes input back",
		result: tools.NewToolResult("hello world"),
	})
	reg.Register(&mockTool{
		name:   "fail_tool",
		desc:   "Always fails",
		result: tools.ErrorResult("something broke"),
	})
	return reg
}

// roundTrip sends a JSON-RPC request line and returns the parsed response.
func roundTrip(t *testing.T, srv *Server, req Request) Response {
	t.Helper()

	input, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	input = append(input, '\n')

	var out bytes.Buffer
	srv.in = bytes.NewReader(input)
	srv.out = &out

	ctx := context.Background()
	if err := srv.Serve(ctx); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", out.String(), err)
	}
	return resp
}

func TestInitialize(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "initialize",
		Params: InitializeParams{
			ProtocolVersion: ProtocolVersion,
			ClientInfo:      EntityInfo{Name: "test-client"},
		},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	var result InitializeResult
	json.Unmarshal(raw, &result)

	if result.ProtocolVersion != ProtocolVersion {
		t.Errorf("protocol version = %q, want %q", result.ProtocolVersion, ProtocolVersion)
	}
	if result.ServerInfo.Name != ServerName {
		t.Errorf("server name = %q, want %q", result.ServerInfo.Name, ServerName)
	}
	if result.Capabilities.Tools == nil {
		t.Error("tools capability is nil")
	}
}

func TestToolsList(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(2),
		Method:  "tools/list",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	var result ToolsListResult
	json.Unmarshal(raw, &result)

	if len(result.Tools) != 2 {
		t.Fatalf("tools count = %d, want 2", len(result.Tools))
	}

	names := map[string]bool{}
	for _, tool := range result.Tools {
		names[tool.Name] = true
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil inputSchema", tool.Name)
		}
	}
	if !names["echo"] {
		t.Error("expected tool 'echo' not found")
	}
	if !names["fail_tool"] {
		t.Error("expected tool 'fail_tool' not found")
	}
}

func TestToolsCall_Success(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(3),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "echo",
			"arguments": map[string]any{"arg1": "test"},
		},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(raw, &result)

	if result.IsError {
		t.Error("expected success, got isError=true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(result.Content))
	}
	if result.Content[0].Text != "hello world" {
		t.Errorf("text = %q, want %q", result.Content[0].Text, "hello world")
	}
}

func TestToolsCall_Error(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(4),
		Method:  "tools/call",
		Params: map[string]any{
			"name":      "fail_tool",
			"arguments": map[string]any{"arg1": "x"},
		},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(raw, &result)

	if !result.IsError {
		t.Error("expected isError=true for failing tool")
	}
	if !strings.Contains(result.Content[0].Text, "something broke") {
		t.Errorf("error text = %q, expected to contain 'something broke'", result.Content[0].Text)
	}
}

func TestToolsCall_NotFound(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(5),
		Method:  "tools/call",
		Params: map[string]any{
			"name": "nonexistent",
		},
	})

	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %+v", resp.Error)
	}

	raw, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	json.Unmarshal(raw, &result)

	if !result.IsError {
		t.Error("expected isError=true for unknown tool")
	}
}

func TestToolsCall_MissingName(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(6),
		Method:  "tools/call",
		Params:  map[string]any{},
	})

	if resp.Error == nil {
		t.Fatal("expected error for missing tool name")
	}
	if resp.Error.Code != ErrInvalidReq {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrInvalidReq)
	}
}

func TestPing(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(7),
		Method:  "ping",
	})

	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestUnknownMethod(t *testing.T) {
	reg := newTestRegistry()
	srv := NewServerWithIO(reg, nil, nil)

	resp := roundTrip(t, srv, Request{
		JSONRPC: "2.0",
		ID:      float64(8),
		Method:  "unknown/method",
	})

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != ErrNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrNotFound)
	}
}

func TestParseError(t *testing.T) {
	reg := newTestRegistry()
	var out bytes.Buffer
	srv := NewServerWithIO(reg, strings.NewReader("not json\n"), &out)

	ctx := context.Background()
	_ = srv.Serve(ctx)

	var resp Response
	json.Unmarshal(out.Bytes(), &resp)

	if resp.Error == nil {
		t.Fatal("expected parse error")
	}
	if resp.Error.Code != ErrParse {
		t.Errorf("error code = %d, want %d", resp.Error.Code, ErrParse)
	}
}
