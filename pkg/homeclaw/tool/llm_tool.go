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

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/llm"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third"
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
	llm           *llm.LLM
	workspace     string
	deviceOpStore data.DeviceOpStore
	deviceStore   data.DeviceStore
	clients       map[string]third.Client
}

// NewLLMTool creates an LLMTool with the given LLM instance.
func NewLLMTool(llm *llm.LLM, workspace string) *LLMTool {
	return &LLMTool{
		llm:       llm,
		workspace: workspace,
	}
}

// NewLLMToolWithStores creates an LLMTool with LLM instance, workspace path, and data stores for device operations.
func NewLLMToolWithStores(llm *llm.LLM, workspace string, deviceOpStore data.DeviceOpStore, deviceStore data.DeviceStore) *LLMTool {
	return &LLMTool{
		llm:           llm,
		workspace:     workspace,
		deviceOpStore: deviceOpStore,
		deviceStore:   deviceStore,
	}
}

// NewLLMToolWithClients creates an LLMTool with LLM instance, workspace path, data stores, and brand clients.
func NewLLMToolWithClients(llm *llm.LLM, workspace string, deviceOpStore data.DeviceOpStore, deviceStore data.DeviceStore, clients map[string]third.Client) *LLMTool {
	return &LLMTool{
		llm:           llm,
		workspace:     workspace,
		deviceOpStore: deviceOpStore,
		deviceStore:   deviceStore,
		clients:       clients,
	}
}

// SetClients sets the brand clients for device spec analysis.
// This can be called after construction to enable the analyzeAndSaveDeviceOps method.
func (t *LLMTool) SetClients(clients map[string]third.Client) {
	t.clients = clients
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
	case "analyzeDeviceOps":
		return t.execAnalyzeDeviceOps(ctx, t.llm, req.Params)
	case "batchAnalyzeDevices":
		return t.execBatchAnalyzeDevices(ctx, t.llm, req.Params)
	case "analyzeDeviceOpsAsync":
		return t.execAnalyzeDeviceOpsAsync(ctx, t.llm, req.Params)
	case "batchAnalyzeDevicesAsync":
		return t.execBatchAnalyzeDevicesAsync(ctx, t.llm, req.Params)
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

// execAnalyzeDeviceOps analyzes device spec using LLM and saves the generated operations.
// params: {"brand": "xiaomi"|"tuya", "from_id": "device_id"}
func (t *LLMTool) execAnalyzeDeviceOps(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for analyzeDeviceOps", IsError: true}
	}

	brand, ok := params["brand"].(string)
	if !ok || brand == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'brand' in params", IsError: true}
	}

	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'from_id' in params", IsError: true}
	}

	// Get client for the brand
	if t.clients == nil {
		return &tools.ToolResult{ForLLM: "clients map is not initialized", IsError: true}
	}

	client, found := t.clients[brand]
	if !found {
		available := make([]string, 0, len(t.clients))
		for k := range t.clients {
			available = append(available, k)
		}
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown brand '%s'; registered brands: %v", brand, available),
			IsError: true,
		}
	}

	// Fetch spec from client
	specInfo, err := client.GetSpec(fromID)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get spec for device %s: %v", fromID, err), IsError: true}
	}

	// If spec is empty, mark device as NoAction
	if specInfo == nil || specInfo.Raw == "" {
		logger.Infof("Device %s from %s has empty spec, marking as NoAction", fromID, brand)
		return t.markDeviceAsNoAction(fromID, brand)
	}

	// Marshal spec to JSON
	specJSON, err := json.Marshal(specInfo)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to marshal spec: %v", err), IsError: true}
	}

	logger.Infof("Analyzing device spec for brand=%s, from_id=%s", brand, fromID)

	// Load brand-specific parsing rules
	parsingRules, err := t.loadBrandParsingRules(brand)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to load parsing rules for brand '%s': %v", brand, err), IsError: true}
	}

	// Load supported operations reference
	opsReference, err := t.loadOpsReference()
	if err != nil {
		logger.Warnf("failed to load ops reference: %v (continuing without it)", err)
		opsReference = ""
	}

	// Build prompt for LLM to analyze spec and generate operations
	prompt := fmt.Sprintf(`You are a smart home device specification analyzer. Your task is to parse device specifications and generate standardized operations.

## Brand Parsing Rules:
%s

## Supported Operations Reference:
%s

## Device Specification:
%s

## Device Information:
- brand: %s
- from_id: %s

## Task:
Analyze the device specification according to the brand parsing rules and generate an array of operations.
Return ONLY a valid JSON array. Do not include any explanation or markdown formatting.
`, parsingRules, opsReference, specJSON, brand, fromID)

	// Call LLM to analyze spec
	result, err := llmInst.Chat(ctx, "You are a smart home device specification analyzer.", prompt)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to analyze device spec: %v", err), IsError: true}
	}

	logger.Infof("LLM analysis result length: %d chars", len(result))

	// Parse the JSON array from LLM response
	opsArray, err := t.parseOpsArrayFromLLMResult(result)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse LLM result: %v\n\nRaw result: %s", err, result), IsError: true}
	}

	if len(opsArray) == 0 {
		// No operations generated - mark device as NoAction
		return t.markDeviceAsNoAction(fromID, brand)
	}

	// Save operations
	return t.saveDeviceOperations(fromID, brand, opsArray)
}

// loadBrandParsingRules loads the parsing rules for a specific brand.
func (t *LLMTool) loadBrandParsingRules(brand string) (string, error) {
	if t.workspace == "" {
		return "", fmt.Errorf("workspace path not configured")
	}

	// Try to find the reference file in workspace
	workspacePaths := []string{
		filepath.Join(t.workspace, "skills", "device-spec-analyze", "reference"),
	}

	fileName := brand + ".md"

	for _, basePath := range workspacePaths {
		filePath := filepath.Join(basePath, fileName)
		if content, err := os.ReadFile(filePath); err == nil {
			return string(content), nil
		}
	}

	return "", fmt.Errorf("parsing rules file not found for brand '%s'", brand)
}

// loadOpsReference loads the supported operations reference.
func (t *LLMTool) loadOpsReference() (string, error) {
	if t.workspace == "" {
		return "", fmt.Errorf("workspace path not configured")
	}

	workspacePaths := []string{
		filepath.Join(t.workspace, "skills", "device-spec-analyze", "reference"),
	}

	fileName := "ops.md"

	for _, basePath := range workspacePaths {
		filePath := filepath.Join(basePath, fileName)
		if content, err := os.ReadFile(filePath); err == nil {
			return string(content), nil
		}
	}

	return "", fmt.Errorf("ops reference file not found")
}

// parseOpsArrayFromLLMResult extracts and parses the JSON array from LLM response.
func (t *LLMTool) parseOpsArrayFromLLMResult(result string) ([]map[string]any, error) {
	// Try to find JSON array in the result
	result = strings.TrimSpace(result)

	// If result starts with [ and ends with ], parse directly
	if strings.HasPrefix(result, "[") && strings.HasSuffix(result, "]") {
		var opsArray []map[string]any
		if err := json.Unmarshal([]byte(result), &opsArray); err == nil {
			return opsArray, nil
		}
	}

	// Try to extract JSON array from markdown code block
	if idx := strings.Index(result, "```"); idx != -1 {
		// Find the content between code blocks
		startIdx := strings.Index(result[idx:], "\n")
		if startIdx != -1 {
			startIdx += idx + 1
			endIdx := strings.Index(result[startIdx:], "```")
			if endIdx != -1 {
				jsonStr := strings.TrimSpace(result[startIdx : startIdx+endIdx])
				var opsArray []map[string]any
				if err := json.Unmarshal([]byte(jsonStr), &opsArray); err == nil {
					return opsArray, nil
				}
			}
		}
	}

	// Try to find any JSON array in the text
	startIdx := strings.Index(result, "[")
	endIdx := strings.LastIndex(result, "]")
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		jsonStr := result[startIdx : endIdx+1]
		var opsArray []map[string]any
		if err := json.Unmarshal([]byte(jsonStr), &opsArray); err == nil {
			return opsArray, nil
		}
	}

	return nil, fmt.Errorf("could not find valid JSON array in LLM result")
}

// markDeviceAsNoAction marks a device as non-operable.
func (t *LLMTool) markDeviceAsNoAction(fromID, from string) *tools.ToolResult {
	if t.deviceStore == nil {
		return &tools.ToolResult{ForLLM: "deviceStore is not initialized", IsError: true}
	}

	// Get all devices to find the target device
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	// Find and update the target device
	for _, device := range devices {
		if device.FromID == fromID && (from == "" || device.From == from) {
			device.Ops = []string{"NoAction"}
			if err := t.deviceStore.Save(device); err != nil {
				return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save device: %v", err), IsError: true}
			}
			return tools.NewToolResult(fmt.Sprintf("device %s from %s marked as NoAction (no operations could be generated)", fromID, device.From))
		}
	}

	return &tools.ToolResult{ForLLM: fmt.Sprintf("device not found: from_id=%s, from=%s", fromID, from), IsError: true}
}

// saveDeviceOperations saves the generated operations to the device operation store.
func (t *LLMTool) saveDeviceOperations(fromID, from string, opsArray []map[string]any) *tools.ToolResult {
	if t.deviceOpStore == nil {
		return &tools.ToolResult{ForLLM: "deviceOpStore is not initialized", IsError: true}
	}

	// Convert ops array to DeviceOp slice
	deviceOps := make([]data.DeviceOp, 0, len(opsArray))
	for _, entry := range opsArray {
		method, _ := entry["method"].(string)
		ops, _ := entry["ops"].(string)
		param := entry["param"]

		if method == "" || ops == "" || param == nil {
			continue
		}

		// Convert param to JSON string
		var paramJSON string
		if paramStr, ok := param.(string); ok {
			paramJSON = paramStr
		} else {
			if paramBytes, err := json.Marshal(param); err == nil {
				paramJSON = string(paramBytes)
			} else {
				continue
			}
		}

		deviceOps = append(deviceOps, data.DeviceOp{
			FromID: fromID,
			From:   from,
			Ops:    ops,
			Method: method,
			Param:  paramJSON,
		})
	}

	if len(deviceOps) == 0 {
		return &tools.ToolResult{ForLLM: "no valid operations to save", IsError: true}
	}

	// Batch save all operations
	if err := t.deviceOpStore.Save(deviceOps...); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to batch save device operations: %v", err), IsError: true}
	}

	return tools.NewToolResult(fmt.Sprintf("successfully saved %d device operations for device %s from %s", len(deviceOps), fromID, from))
}

// execBatchAnalyzeDevices queries all devices with empty operations and batch analyzes them.
// params: {} (no parameters required)
func (t *LLMTool) execBatchAnalyzeDevices(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	if t.deviceStore == nil {
		return &tools.ToolResult{ForLLM: "deviceStore is not initialized", IsError: true}
	}

	// Get all devices
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	// Filter devices with empty ops
	var devicesWithoutOps []data.Device
	for _, device := range devices {
		if len(device.Ops) == 0 {
			devicesWithoutOps = append(devicesWithoutOps, device)
		}
	}

	if len(devicesWithoutOps) == 0 {
		return tools.NewToolResult("all devices already have operations configured")
	}

	logger.Infof("Found %d devices without operations, starting batch analysis", len(devicesWithoutOps))

	// Process each device
	var results []string
	successCount := 0
	failCount := 0

	for _, device := range devicesWithoutOps {
		logger.Infof("Analyzing device: %s (brand: %s)", device.FromID, device.From)

		// Call execAnalyzeDeviceOps for this device
		analyzeParams := map[string]any{
			"brand":   device.From,
			"from_id": device.FromID,
		}

		result := t.execAnalyzeDeviceOps(ctx, llmInst, analyzeParams)

		if result.IsError {
			failCount++
			results = append(results, fmt.Sprintf("FAILED: %s (%s) - %s", device.FromID, device.From, result.ForLLM))
			logger.Warnf("Failed to analyze device %s (%s): %s", device.FromID, device.From, result.ForLLM)
		} else {
			successCount++
			results = append(results, fmt.Sprintf("SUCCESS: %s (%s) - %s", device.FromID, device.From, result.ForLLM))
			logger.Infof("Successfully analyzed device %s (%s)", device.FromID, device.From)
		}
	}

	// Build summary
	summary := fmt.Sprintf("Batch analysis complete: %d succeeded, %d failed out of %d devices\n\nDetails:\n%s",
		successCount, failCount, len(devicesWithoutOps), strings.Join(results, "\n"))

	return tools.NewToolResult(summary)
}

// execAnalyzeDeviceOpsAsync asynchronously analyzes device spec using LLM and saves the generated operations.
// This method starts the analysis in a goroutine and returns immediately.
// params: {"brand": "xiaomi"|"tuya", "from_id": "device_id"}
func (t *LLMTool) execAnalyzeDeviceOpsAsync(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'from_id' in params", IsError: true}
	}

	brand, ok := params["brand"].(string)
	if !ok || brand == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'brand' in params", IsError: true}
	}

	// Create a background context independent of the turn context
	// This ensures the async operation continues even after the tool returns
	backgroundCtx := context.Background()

	// Start goroutine to perform analysis in background
	go func() {
		logger.Infof("Async analyzing device: %s (brand: %s)", fromID, brand)
		result := t.execAnalyzeDeviceOps(backgroundCtx, llmInst, params)
		if result.IsError {
			logger.Warnf("Async analyze device %s (%s) failed: %s", fromID, brand, result.ForLLM)
		} else {
			logger.Infof("Async analyze device %s (%s) succeeded: %s", fromID, brand, result.ForLLM)
		}
	}()

	return tools.NewToolResult(fmt.Sprintf("Device %s analysis started in background", fromID))
}

// execBatchAnalyzeDevicesAsync asynchronously queries all devices with empty operations and batch analyzes them.
// This method starts the batch analysis in a goroutine and returns immediately.
// params: {} (no parameters required)
func (t *LLMTool) execBatchAnalyzeDevicesAsync(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	if t.deviceStore == nil {
		return &tools.ToolResult{ForLLM: "deviceStore is not initialized", IsError: true}
	}

	// Get device count for quick validation
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	// Count devices without ops
	count := 0
	for _, device := range devices {
		if len(device.Ops) == 0 {
			count++
		}
	}

	if count == 0 {
		return tools.NewToolResult("all devices already have operations configured")
	}

	// Create a background context independent of the turn context
	// This ensures the async operation continues even after the tool returns
	backgroundCtx := context.Background()

	// Start goroutine to perform batch analysis in background
	go func() {
		logger.Infof("Async batch analyzing %d devices", count)
		result := t.execBatchAnalyzeDevices(backgroundCtx, llmInst, params)
		if result.IsError {
			logger.Warnf("Async batch analyze failed: %s", result.ForLLM)
		} else {
			logger.Infof("Async batch analyze succeeded: %s", result.ForLLM)
		}
	}()

	return tools.NewToolResult(fmt.Sprintf("Batch analysis started for %d devices in background", count))
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
