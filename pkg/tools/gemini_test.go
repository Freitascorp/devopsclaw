package tools

import (
	"context"
	"testing"
)

func TestGeminiTool_Name(t *testing.T) {
	tool := NewGeminiTool("/tmp")
	if tool.Name() != "gemini" {
		t.Errorf("expected name 'gemini', got %q", tool.Name())
	}
}

func TestGeminiTool_Description(t *testing.T) {
	tool := NewGeminiTool("/tmp")
	desc := tool.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
}

func TestGeminiTool_Parameters(t *testing.T) {
	tool := NewGeminiTool("/tmp")
	params := tool.Parameters()

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("parameters should have properties")
	}

	if _, ok := props["prompt"]; !ok {
		t.Error("parameters should have 'prompt' property")
	}
	if _, ok := props["model"]; !ok {
		t.Error("parameters should have 'model' property")
	}
	if _, ok := props["working_dir"]; !ok {
		t.Error("parameters should have 'working_dir' property")
	}

	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("parameters should have required array")
	}
	if len(required) != 1 || required[0] != "prompt" {
		t.Errorf("expected required=['prompt'], got %v", required)
	}
}

func TestGeminiTool_Execute_EmptyPrompt(t *testing.T) {
	tool := NewGeminiTool("/tmp")

	result := tool.Execute(context.Background(), map[string]any{})
	if !result.IsError {
		t.Error("expected error for missing prompt")
	}

	result = tool.Execute(context.Background(), map[string]any{"prompt": ""})
	if !result.IsError {
		t.Error("expected error for empty prompt")
	}

	result = tool.Execute(context.Background(), map[string]any{"prompt": "   "})
	if !result.IsError {
		t.Error("expected error for whitespace-only prompt")
	}
}

func TestGeminiTool_Execute_GeminiNotInstalled(t *testing.T) {
	// Override PATH to make gemini CLI unfindable
	tool := NewGeminiTool("/tmp")
	t.Setenv("PATH", "/nonexistent")

	result := tool.Execute(context.Background(), map[string]any{"prompt": "test"})
	if !result.IsError {
		t.Error("expected error when gemini CLI not found")
	}
	if result.ForLLM == "" {
		t.Error("error message should not be empty")
	}
}

func TestGeminiTool_SetTimeout(t *testing.T) {
	tool := NewGeminiTool("/tmp")
	tool.SetTimeout(30_000_000_000) // 30s in nanoseconds
	if tool.timeout != 30_000_000_000 {
		t.Errorf("expected timeout 30s, got %v", tool.timeout)
	}
}

func TestGeminiTool_SetModel(t *testing.T) {
	tool := NewGeminiTool("/tmp")
	tool.SetModel("gemini-2.5-flash")
	if tool.model != "gemini-2.5-flash" {
		t.Errorf("expected model 'gemini-2.5-flash', got %q", tool.model)
	}
}

func TestGeminiTool_ImplementsToolInterface(t *testing.T) {
	var _ Tool = (*GeminiTool)(nil)
}

func TestGeminiCLIAvailable(t *testing.T) {
	// Just ensure the function doesn't panic
	_ = geminiCLIAvailable()
}
