package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/video"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// IntentProviderFactory provides access to the LLM provider used for intent/vision analysis.
type IntentProviderFactory interface {
	GetIntentProvider() (providers.LLMProvider, error)
	GetIntentModelName() string
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_analyze_rtsp_frame
// ─────────────────────────────────────────────────────────────────────────────

// RTSPAnalyzeTool captures a single frame from an RTSP stream and sends it to
// the intent LLM provider for visual analysis.
type RTSPAnalyzeTool struct {
	grabber         *video.FrameGrabber
	providerFactory IntentProviderFactory
}

// NewRTSPAnalyzeTool creates an RTSPAnalyzeTool backed by the given FrameGrabber
// and intent provider factory.
func NewRTSPAnalyzeTool(grabber *video.FrameGrabber, factory IntentProviderFactory) *RTSPAnalyzeTool {
	return &RTSPAnalyzeTool{
		grabber:         grabber,
		providerFactory: factory,
	}
}

func (t *RTSPAnalyzeTool) Name() string { return "hc_internal_3" }

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

	// 1. Capture a frame from the RTSP stream
	dataURI, err := t.grabber.GrabFrameAsDataURI(ctx, rtspURL)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("failed to capture frame from %q: %v", rtspURL, err))
	}

	// 2. Get the intent LLM provider
	provider, err := t.providerFactory.GetIntentProvider()
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("intent provider unavailable: %v", err))
	}
	modelName := t.providerFactory.GetIntentModelName()

	// 3. Build a multimodal message: text prompt + captured frame
	messages := []providers.Message{
		{
			Role:    "user",
			Content: prompt,
			Media:   []string{dataURI},
		},
	}

	// 4. Call the provider
	resp, err := provider.Chat(ctx, messages, nil, modelName, nil)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("vision analysis failed: %v", err))
	}

	// 5. Return the analysis result as JSON
	result := map[string]any{
		"analysis": resp.Content,
		"rtsp_url": rtspURL,
	}
	b, _ := json.Marshal(result)
	return tools.NewToolResult(string(b))
}
