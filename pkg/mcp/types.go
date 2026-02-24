// DevOpsClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 DevOpsClaw contributors

// Package mcp implements a Model Context Protocol (MCP) stdio server.
// It exposes devopsclaw's tool registry as MCP tools, enabling
// external AI clients like Gemini CLI to call devopsclaw tools directly.
//
// Protocol: JSON-RPC 2.0 over stdin/stdout (stdio transport).
// Spec: https://modelcontextprotocol.io/specification
package mcp

// ── JSON-RPC 2.0 envelope ──────────────────────────────────────────

// Request is a JSON-RPC 2.0 request/notification.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"` // nil for notifications
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ── MCP initialize ─────────────────────────────────────────────────

// InitializeParams is sent by the client on the "initialize" method.
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ClientInfo      EntityInfo     `json:"clientInfo,omitempty"`
}

// InitializeResult is returned by the server in response to "initialize".
type InitializeResult struct {
	ProtocolVersion string           `json:"protocolVersion"`
	Capabilities    ServerCapability `json:"capabilities"`
	ServerInfo      EntityInfo       `json:"serverInfo"`
}

// EntityInfo identifies a client or server.
type EntityInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ServerCapability advertises supported features.
type ServerCapability struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ToolsCapability describes the tools feature.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ── tools/list ─────────────────────────────────────────────────────

// ToolsListResult is the response to "tools/list".
type ToolsListResult struct {
	Tools []ToolInfo `json:"tools"`
}

// ToolInfo describes a single MCP tool.
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ── tools/call ─────────────────────────────────────────────────────

// ToolCallParams is the input for "tools/call".
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ToolCallResult is the response to "tools/call".
type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a text content block in the MCP response.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ── JSON-RPC error codes ───────────────────────────────────────────

const (
	ErrParse      = -32700
	ErrInvalidReq = -32600
	ErrNotFound   = -32601
	ErrInternal   = -32603
)
