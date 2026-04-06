package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/common"
	"github.com/sipeed/picoclaw/pkg/homeclaw/llm"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_video
// ─────────────────────────────────────────────────────────────────────────────

// VideoTool is a unified video tool that dispatches to the correct method
// based on the "method" field in the commandJson argument.
//
// Supported methods:
//   - capImage: Capture a single frame from an RTSP stream
//   - capAnalyze: Capture a frame and analyze it with a vision model
//
// commandJson schema:
//
//	{
//	  "method": "capImage" | "capAnalyze",
//	  "params": {
//	    "rtsp_url": "rtsp://...",           // required
//	    "rtsp_transport": "tcp" | "udp",    // optional, defaults to "tcp"
//	    "prompt": "...",                    // optional, for capAnalyze
//	    "return_image": true|false         // optional, whether to return image in MediaResult
//	  }
//	}
//
// capImage    – capture a frame and return the file path. If return_image is true, also returns the image via MediaResult.
// capAnalyze  – capture a frame, analyze with vision model, and return the analysis. If return_image is true, also returns the image via MediaResult.
type VideoTool struct {
	localLLM   *llm.LLM
	mediaStore media.MediaStore
}

// NewVideoTool creates a VideoTool backed by the given LLM instance.
func NewVideoTool(localLLM *llm.LLM) *VideoTool {
	return &VideoTool{
		localLLM: localLLM,
	}
}

// SetMediaStore sets the media store for sending images to channels.
func (t *VideoTool) SetMediaStore(store media.MediaStore) {
	t.mediaStore = store
}

func (t *VideoTool) Name() string { return "hc_video" }

func (t *VideoTool) Description() string {
	return "Unified video tool for camera operations. " +
		"Use commandJson to specify method and params. " +
		"Supported methods: capImage, capAnalyze. " +
		"IMPORTANT: The rtsp_url MUST be obtained from hc_list_cameras. Do NOT fabricate or guess any URL."
}

func (t *VideoTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"commandJson": map[string]any{
				"type":        "string",
				"description": `JSON string with "method" and "params". Do NOT fabricate or guess any json,must follow skill `,
			},
		},
		"required": []string{"commandJson"},
	}
}

// videoCommandRequest is the parsed form of the commandJson argument.
type videoCommandRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func (t *VideoTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	// 0. Verify ffmpeg is available (cross-platform check)
	if err := common.CheckFFmpeg(); err != nil {
		return tools.ErrorResult(fmt.Sprintf("ffmpeg prerequisite check failed: %v", err))
	}

	commandJson, ok := args["commandJson"].(string)
	if !ok || commandJson == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'commandJson' parameter", IsError: true}
	}

	var req videoCommandRequest
	if err := json.Unmarshal([]byte(commandJson), &req); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse commandJson: %v", err), IsError: true}
	}

	if req.Method == "" {
		return &tools.ToolResult{ForLLM: "missing 'method' in commandJson", IsError: true}
	}

	if req.Params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' in commandJson", IsError: true}
	}

	rtspURL, ok := req.Params["rtsp_url"].(string)
	if !ok || rtspURL == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'rtsp_url' in params", IsError: true}
	}

	switch req.Method {
	case "capImage":
		return t.execCapImage(ctx, rtspURL, req.Params)
	case "capAnalyze":
		return t.execCapAnalyze(ctx, rtspURL, req.Params)
	default:
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown method '%s'; supported: capImage, capAnalyze", req.Method),
			IsError: true,
		}
	}
}

// execCapImage captures a single frame from the RTSP stream.
func (t *VideoTool) execCapImage(ctx context.Context, rtspURL string, params map[string]any) *tools.ToolResult {
	// Get RTSP transport from params (default to "tcp")
	rtspTransport := "tcp"
	if transport, ok := params["rtsp_transport"].(string); ok && transport != "" {
		rtspTransport = transport
	}

	// 1. Capture a frame from the RTSP stream (returns both dataURI and file path)
	_, filePath, err := common.CapImgBase64(ctx, rtspURL, 3, 4, 6, rtspTransport)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to capture frame from %q: %v", rtspURL, err))
	}

	// 2. Check if we should include the image in MediaResult
	includeImage := params["return_image"] == true
	var mediaRefs []string

	if includeImage && t.mediaStore != nil {
		channel := tools.ToolChannel(ctx)
		chatID := tools.ToolChatID(ctx)

		if channel != "" && chatID != "" && filePath != "" {
			scope := fmt.Sprintf("tool:video:%s:%s", channel, chatID)
			ref, err := t.mediaStore.Store(filePath, media.MediaMeta{
				Filename:    "camera_frame.jpg",
				ContentType: "image/jpeg",
				Source:      "tool:hc_video",
			}, scope)
			if err == nil {
				mediaRefs = append(mediaRefs, ref)
				// Temp file is now tracked by MediaStore for cleanup on scope release/TTL
			}
		}
	}

	// 3. Return the result
	result := map[string]any{
		"file_path": filePath,
	}
	b, _ := json.Marshal(result)

	if len(mediaRefs) > 0 {
		logger.Info("return with media ------------------------------------")
		// Return image with ResponseHandled so it's sent immediately
		return &tools.ToolResult{
			ForLLM:          string(b),
			Media:           mediaRefs,
			ResponseHandled: true,
		}
	}
	return tools.NewToolResult(string(b))
}

// execCapAnalyze captures a frame and analyzes it with the vision model.
func (t *VideoTool) execCapAnalyze(ctx context.Context, rtspURL string, params map[string]any) *tools.ToolResult {
	// Get RTSP transport from params (default to "tcp")
	rtspTransport := "tcp"
	if transport, ok := params["rtsp_transport"].(string); ok && transport != "" {
		rtspTransport = transport
	}

	// 1. Capture a frame from the RTSP stream (returns both dataURI and file path)
	dataURI, filePath, err := common.CapImgBase64(ctx, rtspURL, 3, 4, 6, rtspTransport)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to capture frame from %q: %v", rtspURL, err))
	}

	// 2. Check if we should include the image in MediaResult
	includeImage := params["return_image"] == true
	var mediaRefs []string

	if includeImage && t.mediaStore != nil {
		channel := tools.ToolChannel(ctx)
		chatID := tools.ToolChatID(ctx)

		if channel != "" && chatID != "" && filePath != "" {
			scope := fmt.Sprintf("tool:video:%s:%s", channel, chatID)
			ref, err := t.mediaStore.Store(filePath, media.MediaMeta{
				Filename:    "camera_frame.jpg",
				ContentType: "image/jpeg",
				Source:      "tool:hc_video",
			}, scope)
			if err == nil {
				mediaRefs = append(mediaRefs, ref)
				// Temp file is now tracked by MediaStore for cleanup on scope release/TTL
			}
		}
	}

	// 3. Get the intent LLM
	if t.localLLM == nil {
		return tools.ErrorResult("intent LLM not available")
	}

	// 4. Build the prompt
	prompt := "Describe what you see in this camera frame in detail."
	if p, ok := params["prompt"].(string); ok && p != "" {
		prompt = p
	}

	// 5. Build a multimodal message: text prompt + captured frame
	messages := []providers.Message{
		{
			Role:    "user",
			Content: prompt,
			Media:   []string{dataURI},
		},
	}

	// 6. Call the LLM
	resp, err := t.localLLM.Provider.Chat(ctx, messages, nil, t.localLLM.Model, nil)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("vision analysis failed: %v", err))
	}

	// 7. Return the analysis result
	result := map[string]any{
		"analysis":  resp.Content,
		"file_path": filePath,
	}
	b, _ := json.Marshal(result)

	if len(mediaRefs) > 0 {
		logger.Info("return with media ------------------------------------")
		// Return both analysis text (ForUser) and image (Media)
		return &tools.ToolResult{
			ForLLM:          string(b),
			ForUser:         resp.Content,
			Media:           mediaRefs,
			ResponseHandled: true,
		}
	}
	return tools.NewToolResult(string(b))
}
