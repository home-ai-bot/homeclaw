package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/llm"
	"github.com/sipeed/picoclaw/pkg/homeclaw/video"
	"github.com/sipeed/picoclaw/pkg/media"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// IntentProviderFactory provides access to the LLM provider used for intent/vision analysis.
type IntentProviderFactory interface {
	GetLocalLLM() (*llm.LLM, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_analyze_rtsp_frame
// ─────────────────────────────────────────────────────────────────────────────

// RTSPAnalyzeTool captures a single frame from an RTSP stream and sends it to
// the intent LLM provider for visual analysis.
type RTSPAnalyzeTool struct {
	ffmpegUtil      *video.FFmpegUtil
	providerFactory IntentProviderFactory
	mediaStore      media.MediaStore
}

// NewRTSPAnalyzeTool creates an RTSPAnalyzeTool backed by the given FFmpegUtil
// and intent provider factory.
func NewRTSPAnalyzeTool(ffmpegUtil *video.FFmpegUtil, factory IntentProviderFactory) *RTSPAnalyzeTool {
	return &RTSPAnalyzeTool{
		ffmpegUtil:      ffmpegUtil,
		providerFactory: factory,
	}
}

// SetMediaStore sets the media store for sending images to channels.
func (t *RTSPAnalyzeTool) SetMediaStore(store media.MediaStore) {
	t.mediaStore = store
}

func (t *RTSPAnalyzeTool) Name() string { return "hc_private_camera_analyze" }

func (t *RTSPAnalyzeTool) Description() string {
	return "IMPORTANT: The rtsp_url MUST be obtained from hc_list_cameras. Do NOT fabricate or guess any URL."
}

func (t *RTSPAnalyzeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"rtsp_url": map[string]any{
				"type":        "string",
				"description": "Full RTSP URL of the camera stream. MUST be obtained from hc_list_cameras. Do NOT fabricate or guess any URL.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Optional instruction for the vision model, e.g. 'Is there a person in the frame?'. Defaults to a general scene description request.",
			},
		},
		"required": []string{"rtsp_url"},
	}
}

func (t *RTSPAnalyzeTool) Execute(ctx context.Context, params map[string]any) *tools.ToolResult {
	// 0. Verify ffmpeg is available (cross-platform check)
	if err := video.CheckFFmpeg(); err != nil {
		return tools.ErrorResult(fmt.Sprintf("ffmpeg prerequisite check failed: %v", err))
	}

	rtspURL, ok := params["rtsp_url"].(string)
	if !ok || rtspURL == "" {
		return tools.ErrorResult("rtsp_url parameter is required")
	}

	prompt := "Describe what you see in this camera frame in detail."
	if p, ok := params["prompt"].(string); ok && p != "" {
		prompt = p
	}

	// 1. Capture a frame from the RTSP stream (returns both dataURI and file path)
	dataURI, filePath, err := t.ffmpegUtil.GrabFrameWithPath(ctx, rtspURL)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to capture frame from %q: %v", rtspURL, err))
	}

	// 1.5. Send the captured frame as media to the current channel
	channel := tools.ToolChannel(ctx)
	chatID := tools.ToolChatID(ctx)
	var mediaRefs []string

	if t.mediaStore != nil && channel != "" && chatID != "" && filePath != "" {
		scope := fmt.Sprintf("tool:camera:%s:%s", channel, chatID)
		ref, err := t.mediaStore.Store(filePath, media.MediaMeta{
			Filename:    "camera_frame.jpg",
			ContentType: "image/jpeg",
			Source:      "tool:hc_internal_3",
		}, scope)
		if err == nil {
			mediaRefs = append(mediaRefs, ref)
		}
	}

	// 2. Get the intent LLM
	localLLM, err := t.providerFactory.GetLocalLLM()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("intent LLM unavailable: %v", err))
	}

	// 3. Build a multimodal message: text prompt + captured frame
	messages := []providers.Message{
		{
			Role:    "user",
			Content: prompt,
			Media:   []string{dataURI},
		},
	}

	// 4. Call the LLM
	resp, err := localLLM.Provider.Chat(ctx, messages, nil, localLLM.Model, nil)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("vision analysis failed: %v", err))
	}

	// 5. Return the analysis result as JSON (with media if available)
	result := map[string]any{
		"analysis": resp.Content,
		"rtsp_url": rtspURL,
	}
	b, _ := json.Marshal(result)

	if len(mediaRefs) > 0 {
		return tools.MediaResult(string(b), mediaRefs)
	}
	return tools.NewToolResult(string(b))
}
