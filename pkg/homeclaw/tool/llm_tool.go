package tool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/llm"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_llm
// ─────────────────────────────────────────────────────────────────────────────

// LLMTool is a unified LLM tool that dispatches to different methods based on
// the "method" field. Supports "image" for image analysis and "text" for text
// processing using the local LLM.
//
// commandJson schema:
//
//	{
//	  "method": "image" | "text",
//	  "params": { … }
//	}
//
// image – analyze an image file; params: {"filePath":"/path/to/image.jpg","prompt":"Describe this image"}
// text  – process text content; params: {"content":"text to process","prompt":"Summarize the following"}
type LLMTool struct {
	llm *llm.LLM
}

// NewLLMTool creates an LLMTool with the given LLM instance.
func NewLLMTool(llm *llm.LLM) *LLMTool {
	return &LLMTool{
		llm: llm,
	}
}

func (t *LLMTool) Name() string { return "hc_llm" }

func (t *LLMTool) Description() string {
	return "Do NOT use directly!"
}

func (t *LLMTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"commandJson": map[string]any{
				"type":        "string",
				"description": ``,
			},
		},
		"required": []string{"commandJson"},
	}
}

// llmCommandRequest is the parsed form of the commandJson argument.
type llmCommandRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func (t *LLMTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	commandJson, ok := args["commandJson"].(string)
	if !ok || commandJson == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'commandJson' parameter", IsError: true}
	}

	var req llmCommandRequest
	if err := json.Unmarshal([]byte(commandJson), &req); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse commandJson: %v", err), IsError: true}
	}

	if req.Method == "" {
		return &tools.ToolResult{ForLLM: "missing 'method' in commandJson", IsError: true}
	}

	if t.llm == nil {
		msg := "LLM instance is not initialized"
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	switch req.Method {
	case "image":
		return t.execImage(ctx, t.llm, req.Params)
	case "text":
		return t.execText(ctx, t.llm, req.Params)
	default:
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown method '%s'; Must Confirm! tool must invoke by skills,please use the right skill!", req.Method),
			IsError: true,
		}
	}
}

// execImage analyzes an image file using the LLM.
func (t *LLMTool) execImage(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for image method", IsError: true}
	}

	filePath, ok := params["filePath"].(string)
	if !ok || filePath == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'filePath' in params", IsError: true}
	}

	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'prompt' in params", IsError: true}
	}

	// Read image file
	imageData, err := os.ReadFile(filePath)
	if err != nil {
		msg := fmt.Sprintf("failed to read image file: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	// Detect MIME type from file extension
	ext := filepath.Ext(filePath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "image/jpeg" // default fallback
	}

	// Encode image as base64 data URL
	base64Data := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)

	logger.Infof("Processing image: %s (size: %d bytes, mime: %s)", filePath, len(imageData), mimeType)

	// Build messages with image media
	messages := []providers.Message{
		{
			Role:    "user",
			Content: prompt,
			Media:   []string{dataURL},
		},
	}

	// Call LLM with messages
	result, err := llmInst.ChatWithMessages(ctx, messages)
	if err != nil {
		msg := fmt.Sprintf("failed to analyze image: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	return tools.NewToolResult(fmt.Sprintf("image analysis result: %s", result))
}

// execText processes text content using the LLM.
func (t *LLMTool) execText(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for text method", IsError: true}
	}

	content, ok := params["content"].(string)
	if !ok || content == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'content' in params", IsError: true}
	}

	prompt, ok := params["prompt"].(string)
	if !ok || prompt == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'prompt' in params", IsError: true}
	}

	logger.Infof("Processing text content (length: %d chars)", len(content))

	// Build system prompt and user message
	systemPrompt := prompt
	userMessage := content

	// Call LLM
	result, err := llmInst.Chat(ctx, systemPrompt, userMessage)
	if err != nil {
		msg := fmt.Sprintf("failed to process text: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	return tools.NewToolResult(fmt.Sprintf("text processing result: %s", result))
}

// extractImageAsBase64 reads an image file and returns it as a base64 data URL.
// This is a helper function for image processing.
func extractImageAsBase64(filePath string) (string, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Detect MIME type
	ext := filepath.Ext(filePath)
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	// Encode as base64
	base64Str := base64.StdEncoding.EncodeToString(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str)

	return dataURL, nil
}

// IsImageFile checks if a file path has an image extension.
func IsImageFile(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	imageExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
		".bmp":  true,
		".tiff": true,
	}
	return imageExts[ext]
}
