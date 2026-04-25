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
	"time"

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
		logger.Warnf("[DeviceOps] Missing params for analyzeDeviceOps")
		return &tools.ToolResult{ForLLM: "missing 'params' for analyzeDeviceOps", IsError: true}
	}

	brand, ok := params["brand"].(string)
	if !ok || brand == "" {
		logger.Warnf("[DeviceOps] Missing or invalid 'brand' in params")
		return &tools.ToolResult{ForLLM: "missing or invalid 'brand' in params", IsError: true}
	}

	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		logger.Warnf("[DeviceOps] Missing or invalid 'from_id' in params")
		return &tools.ToolResult{ForLLM: "missing or invalid 'from_id' in params", IsError: true}
	}

	logger.Infof("[DeviceOps] Starting analysis for device %s (brand: %s)", fromID, brand)

	// Get client for the brand
	if t.clients == nil {
		logger.Warnf("[DeviceOps] Clients map is not initialized")
		return &tools.ToolResult{ForLLM: "clients map is not initialized", IsError: true}
	}

	client, found := t.clients[brand]
	if !found {
		available := make([]string, 0, len(t.clients))
		for k := range t.clients {
			available = append(available, k)
		}
		logger.Warnf("[DeviceOps] Unknown brand '%s'; registered brands: %v", brand, available)
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown brand '%s'; registered brands: %v", brand, available),
			IsError: true,
		}
	}

	// Lookup device to get URN
	deviceURN, err := t.getDeviceURN(fromID, brand)
	if err != nil {
		logger.Errorf("[DeviceOps] Failed to get device URN for device %s: %v", fromID, err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get device URN: %v", err), IsError: true}
	}

	// Add URN to params for downstream functions
	params["urn"] = deviceURN

	// Use unified analysis flow for all brands
	return t.execUnifiedDeviceOps(ctx, llmInst, client, fromID, deviceURN)
}

// execUnifiedDeviceOps analyzes device spec using a unified flow for all brands.
// It fetches the spec, validates/compacts JSON, loads brand-specific rules,
// and calls LLM to generate operations.
func (t *LLMTool) execUnifiedDeviceOps(ctx context.Context, llmInst *llm.LLM, client third.Client, fromID, deviceURN string) *tools.ToolResult {
	brand := client.Brand()
	logger.Infof("[DeviceOps] [%s] Starting unified analysis for device %s (URN: %s)", brand, fromID, deviceURN)

	// Check if device already has operations configured
	if t.deviceOpStore != nil {
		existingOps, err := t.deviceOpStore.GetOpsByURN(deviceURN, brand)
		if err == nil && len(existingOps) > 0 {
			logger.Infof("[DeviceOps] [%s] Device %s (URN: %s) already has %d operations configured, skipping analysis", brand, fromID, deviceURN, len(existingOps))
			return &tools.ToolResult{ForLLM: fmt.Sprintf("device %s (URN: %s) already has %d operations configured: %v", fromID, deviceURN, len(existingOps), existingOps)}
		}
	}

	// Fetch spec from client
	specInfo, err := client.GetSpec(fromID)
	if err != nil {
		logger.Errorf("[DeviceOps] [%s] Failed to get spec for device %s: %v", brand, fromID, err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get spec for device %s: %v", fromID, err), IsError: true}
	}

	// If spec is empty, mark device as NoAction
	if specInfo == nil || specInfo.Raw == "" || specInfo.Raw == "{}" {
		logger.Infof("[DeviceOps] [%s] Empty spec for device %s, marking as NoAction", brand, fromID)
		return t.markDeviceAsNoAction(fromID, brand)
	}

	logger.Infof("[DeviceOps] [%s] Fetching spec for device %s (length: %d bytes)", brand, fromID, len(specInfo.Raw))

	// Validate and compact JSON
	specRaw := specInfo.Raw
	var jsonData any
	if err := json.Unmarshal([]byte(specRaw), &jsonData); err == nil {
		// Valid JSON - compact it
		compacted, err := json.Marshal(jsonData)
		if err != nil {
			logger.Warnf("[DeviceOps] [%s] Failed to compact JSON: %v, using raw spec", brand, err)
		} else {
			specRaw = string(compacted)
			logger.Infof("[DeviceOps] [%s] Spec validated and compacted (%d bytes)", brand, len(specRaw))
		}
	} else {
		logger.Warnf("[DeviceOps] [%s] Spec is not valid JSON, using as-is", brand)
	}

	// Load brand-specific parsing rules
	logger.Infof("[DeviceOps] [%s] Loading parsing rules for brand '%s'", brand, brand)
	parsingRules, err := t.loadBrandParsingRules(brand)
	if err != nil {
		logger.Errorf("[DeviceOps] [%s] Failed to load parsing rules for brand '%s': %v", brand, brand, err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to load parsing rules for brand '%s': %v", brand, err), IsError: true}
	}
	logger.Infof("[DeviceOps] [%s] Successfully loaded parsing rules for brand '%s' (length: %d bytes)", brand, brand, len(parsingRules))

	// Load supported operations reference
	logger.Infof("[DeviceOps] [%s] Loading ops reference", brand)
	opsReference, err := t.loadOpsReference()
	if err != nil {
		logger.Warnf("[DeviceOps] [%s] Failed to load ops reference: %v (continuing without it)", brand, err)
		opsReference = ""
	} else {
		logger.Infof("[DeviceOps] [%s] Successfully loaded ops reference (length: %d bytes)", brand, len(opsReference))
	}

	// Build unified prompt
	prompt := fmt.Sprintf(`You are analyzing %s smart home device specifications.

## Brand Parsing Rules:
%s

## Supported Operations:
%s

## Device Specification (compacted JSON):
%s

## Task:
Analyze the spec according to brand rules and return a JSON array of operations.
Each operation must include: ops, param_type, param_value, method, method_param.
Return ONLY valid JSON array. No explanations or markdown.`, brand, parsingRules, opsReference, specRaw)

	logger.Infof("[DeviceOps] [%s] Calling LLM to analyze device %s (prompt length: %d chars)", brand, fromID, len(prompt))

	// Call LLM to analyze spec
	startTime := time.Now()
	result, err := llmInst.Chat(ctx, fmt.Sprintf("You are a %s smart home device specification analyzer.", brand), prompt)
	elapsed := time.Since(startTime)
	if err != nil {
		logger.Errorf("[DeviceOps] [%s] LLM analysis failed for device %s after %v: %v", brand, fromID, elapsed, err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to analyze device spec: %v", err), IsError: true}
	}

	logger.Infof("[DeviceOps] [%s] LLM analysis completed for device %s in %v (result length: %d chars)", brand, fromID, elapsed, len(result))

	// Parse the JSON array from LLM response
	logger.Infof("[DeviceOps] [%s] Parsing operations array from LLM result", brand)
	opsArray, err := t.parseOpsArrayFromLLMResult(result)
	if err != nil {
		logger.Errorf("[DeviceOps] [%s] Failed to parse LLM result for device %s: %v", brand, fromID, err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse LLM result: %v\n\nRaw result: %s", err, result), IsError: true}
	}

	logger.Infof("[DeviceOps] [%s] Successfully parsed %d operations from LLM result for device %s", brand, len(opsArray), fromID)

	if len(opsArray) == 0 {
		logger.Infof("[DeviceOps] [%s] No operations generated for device %s, marking as NoAction", brand, fromID)
		return t.markDeviceAsNoAction(fromID, brand)
	}

	// Save operations with URN
	logger.Infof("[DeviceOps] [%s] Saving %d operations for URN %s", brand, len(opsArray), deviceURN)
	return t.saveDeviceOperations(deviceURN, brand, opsArray)
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
	logger.Infof("[DeviceOps] Marking device %s (from: %s) as NoAction", fromID, from)
	if t.deviceStore == nil {
		logger.Warnf("[DeviceOps] deviceStore is not initialized")
		return &tools.ToolResult{ForLLM: "deviceStore is not initialized", IsError: true}
	}

	// Get all devices to find the target device
	logger.Infof("[DeviceOps] Retrieving all devices from store")
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		logger.Errorf("[DeviceOps] Failed to get devices: %v", err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}
	logger.Infof("[DeviceOps] Retrieved %d devices from store, searching for device %s", len(devices), fromID)

	// Find and update the target device
	for _, device := range devices {
		if device.FromID == fromID && (from == "" || device.From == from) {
			logger.Infof("[DeviceOps] Found device %s (from: %s), setting Ops to [NoAction]", fromID, device.From)
			device.Ops = []string{"NoAction"}
			if err := t.deviceStore.Save(device); err != nil {
				logger.Errorf("[DeviceOps] Failed to save device %s: %v", fromID, err)
				return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save device: %v", err), IsError: true}
			}
			logger.Infof("[DeviceOps] Successfully marked device %s (from: %s) as NoAction", fromID, device.From)
			return tools.NewToolResult(fmt.Sprintf("device %s from %s marked as NoAction (no operations could be generated)", fromID, device.From))
		}
	}

	logger.Warnf("[DeviceOps] Device not found: from_id=%s, from=%s", fromID, from)
	return &tools.ToolResult{ForLLM: fmt.Sprintf("device not found: from_id=%s, from=%s", fromID, from), IsError: true}
}

// saveDeviceOperations saves the generated operations to the device operation store.
// Uses URN-based storage - operations are saved per device type, not per device instance.
func (t *LLMTool) saveDeviceOperations(urn, from string, opsArray []map[string]any) *tools.ToolResult {
	logger.Infof("[DeviceOps] Saving operations for URN %s (from: %s), received %d operations", urn, from, len(opsArray))
	if t.deviceOpStore == nil {
		logger.Warnf("[DeviceOps] deviceOpStore is not initialized")
		return &tools.ToolResult{ForLLM: "deviceOpStore is not initialized", IsError: true}
	}

	// Convert ops array to DeviceOp slice
	deviceOps := make([]data.DeviceOp, 0, len(opsArray))
	validCount := 0
	invalidCount := 0
	for i, entry := range opsArray {
		method, _ := entry["method"].(string)
		ops, _ := entry["ops"].(string)
		paramType, _ := entry["param_type"].(string)
		paramValue := entry["param_value"]
		methodParam := entry["method_param"]

		if method == "" || ops == "" || methodParam == nil {
			logger.Warnf("[DeviceOps] Operation %d is invalid (method: '%s', ops: '%s', method_param: %v)", i+1, method, ops, methodParam != nil)
			invalidCount++
			continue
		}

		// Convert method_param to JSON string (Go template format)
		var methodParamJSON string
		if methodParamStr, ok := methodParam.(string); ok {
			methodParamJSON = methodParamStr
		} else {
			if methodParamBytes, err := json.Marshal(methodParam); err == nil {
				methodParamJSON = string(methodParamBytes)
			} else {
				logger.Warnf("[DeviceOps] Failed to marshal method_param for operation %d: %v", i+1, err)
				invalidCount++
				continue
			}
		}

		deviceOps = append(deviceOps, data.DeviceOp{
			URN:         urn,
			From:        from,
			Ops:         ops,
			ParamType:   paramType,
			ParamValue:  paramValue,
			Method:      method,
			MethodParam: methodParamJSON,
		})
		validCount++
		logger.Infof("[DeviceOps] Prepared operation %d: method='%s', ops='%s', param_type='%s'", i+1, method, ops, paramType)
	}

	logger.Infof("[DeviceOps] Operations validation complete: %d valid, %d invalid out of %d total", validCount, invalidCount, len(opsArray))

	if len(deviceOps) == 0 {
		logger.Warnf("[DeviceOps] No valid operations to save for URN %s", urn)
		return &tools.ToolResult{ForLLM: "no valid operations to save", IsError: true}
	}

	// Batch save all operations
	logger.Infof("[DeviceOps] Batch saving %d operations for URN %s (from: %s)", len(deviceOps), urn, from)
	if err := t.deviceOpStore.Save(deviceOps...); err != nil {
		logger.Errorf("[DeviceOps] Failed to batch save device operations for URN %s: %v", urn, err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to batch save device operations: %v", err), IsError: true}
	}

	logger.Infof("[DeviceOps] Successfully saved %d device operations for URN %s (from: %s)", len(deviceOps), urn, from)
	return tools.NewToolResult(fmt.Sprintf("successfully saved %d device operations for URN %s from %s", len(deviceOps), urn, from))
}

// execBatchAnalyzeDevices queries all devices with empty operations and batch analyzes them.
// params: {} (no parameters required)
func (t *LLMTool) execBatchAnalyzeDevices(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	logger.Infof("[DeviceOps] Starting batch analysis of devices")
	if t.deviceStore == nil {
		logger.Warnf("[DeviceOps] deviceStore is not initialized")
		return &tools.ToolResult{ForLLM: "deviceStore is not initialized", IsError: true}
	}

	// Get all devices
	logger.Infof("[DeviceOps] Retrieving all devices from store")
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		logger.Errorf("[DeviceOps] Failed to get devices: %v", err)
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}
	logger.Infof("[DeviceOps] Retrieved %d devices from store", len(devices))

	// Filter devices with empty ops
	var devicesWithoutOps []data.Device
	for _, device := range devices {
		if len(device.Ops) == 0 {
			devicesWithoutOps = append(devicesWithoutOps, device)
		}
	}

	if len(devicesWithoutOps) == 0 {
		logger.Infof("[DeviceOps] All devices already have operations configured")
		return tools.NewToolResult("all devices already have operations configured")
	}

	logger.Infof("[DeviceOps] Found %d devices without operations, starting batch analysis", len(devicesWithoutOps))
	for i, device := range devicesWithoutOps {
		logger.Infof("[DeviceOps] Device %d/%d to analyze: %s (brand: %s)", i+1, len(devicesWithoutOps), device.FromID, device.From)
	}

	// Process each device
	var results []string
	successCount := 0
	failCount := 0

	for i, device := range devicesWithoutOps {
		logger.Infof("[DeviceOps] === Processing device %d/%d: %s (brand: %s) ===", i+1, len(devicesWithoutOps), device.FromID, device.From)

		// Call execAnalyzeDeviceOps for this device
		analyzeParams := map[string]any{
			"brand":   device.From,
			"from_id": device.FromID,
		}

		result := t.execAnalyzeDeviceOps(ctx, llmInst, analyzeParams)

		if result.IsError {
			failCount++
			results = append(results, fmt.Sprintf("FAILED: %s (%s) - %s", device.FromID, device.From, result.ForLLM))
			logger.Warnf("[DeviceOps] Failed to analyze device %d/%d: %s (%s): %s", i+1, len(devicesWithoutOps), device.FromID, device.From, result.ForLLM)
		} else {
			successCount++
			results = append(results, fmt.Sprintf("SUCCESS: %s (%s) - %s", device.FromID, device.From, result.ForLLM))
			logger.Infof("[DeviceOps] Successfully analyzed device %d/%d: %s (%s)", i+1, len(devicesWithoutOps), device.FromID, device.From)
		}
	}

	// Build summary
	logger.Infof("[DeviceOps] === Batch analysis summary ===")
	logger.Infof("[DeviceOps] Total devices processed: %d", len(devicesWithoutOps))
	logger.Infof("[DeviceOps] Success: %d, Failed: %d", successCount, failCount)
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
		logger.Warnf("[DeviceOps] Missing or invalid 'from_id' in async params")
		return &tools.ToolResult{ForLLM: "missing or invalid 'from_id' in params", IsError: true}
	}

	brand, ok := params["brand"].(string)
	if !ok || brand == "" {
		logger.Warnf("[DeviceOps] Missing or invalid 'brand' in async params")
		return &tools.ToolResult{ForLLM: "missing or invalid 'brand' in params", IsError: true}
	}

	logger.Infof("[DeviceOps] Async analysis requested for device %s (brand: %s)", fromID, brand)

	// Create a background context independent of the turn context
	// This ensures the async operation continues even after the tool returns
	backgroundCtx := context.Background()

	// Start goroutine to perform analysis in background
	go func() {
		logger.Infof("[DeviceOps] === Starting async analysis for device %s (brand: %s) ===", fromID, brand)
		result := t.execAnalyzeDeviceOps(backgroundCtx, llmInst, params)
		if result.IsError {
			logger.Warnf("[DeviceOps] === Async analysis FAILED for device %s (%s): %s ===", fromID, brand, result.ForLLM)
		} else {
			logger.Infof("[DeviceOps] === Async analysis SUCCEEDED for device %s (%s): %s ===", fromID, brand, result.ForLLM)
		}
	}()

	logger.Infof("[DeviceOps] Async analysis dispatched for device %s, returning immediately", fromID)
	return tools.NewToolResult(fmt.Sprintf("Device %s analysis started in background", fromID))
}

// execBatchAnalyzeDevicesAsync asynchronously queries all devices with empty operations and batch analyzes them.
// This method starts the batch analysis in a goroutine and returns immediately.
// params: {} (no parameters required)
func (t *LLMTool) execBatchAnalyzeDevicesAsync(ctx context.Context, llmInst *llm.LLM, params map[string]any) *tools.ToolResult {
	logger.Infof("[DeviceOps] Async batch analysis requested")
	if t.deviceStore == nil {
		logger.Warnf("[DeviceOps] deviceStore is not initialized for async batch analysis")
		return &tools.ToolResult{ForLLM: "deviceStore is not initialized", IsError: true}
	}

	// Get device count for quick validation
	logger.Infof("[DeviceOps] Retrieving devices for async batch analysis")
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		logger.Errorf("[DeviceOps] Failed to get devices for async batch analysis: %v", err)
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
		logger.Infof("[DeviceOps] All devices already have operations configured for async batch analysis")
		return tools.NewToolResult("all devices already have operations configured")
	}

	logger.Infof("[DeviceOps] Found %d devices without operations for async batch analysis", count)

	// Create a background context independent of the turn context
	// This ensures the async operation continues even after the tool returns
	backgroundCtx := context.Background()

	// Start goroutine to perform batch analysis in background
	go func() {
		logger.Infof("[DeviceOps] === Starting async batch analysis for %d devices ===", count)
		result := t.execBatchAnalyzeDevices(backgroundCtx, llmInst, params)
		if result.IsError {
			logger.Warnf("[DeviceOps] === Async batch analysis FAILED: %s ===", result.ForLLM)
		} else {
			logger.Infof("[DeviceOps] === Async batch analysis SUCCEEDED: %s ===", result.ForLLM)
		}
	}()

	logger.Infof("[DeviceOps] Async batch analysis dispatched for %d devices, returning immediately", count)
	return tools.NewToolResult(fmt.Sprintf("Batch analysis started for %d devices in background", count))
}

// getDeviceURN looks up a device by from_id and from, and returns its URN.
// Returns an error if the device is not found or has no URN.
func (t *LLMTool) getDeviceURN(fromID, from string) (string, error) {
	if t.deviceStore == nil {
		return "", fmt.Errorf("deviceStore is not initialized")
	}

	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return "", fmt.Errorf("failed to get devices: %w", err)
	}

	for _, device := range devices {
		if device.FromID == fromID && device.From == from {
			if device.URN == "" {
				return "", fmt.Errorf("device %s (from: %s) has no URN", fromID, from)
			}
			return device.URN, nil
		}
	}

	return "", fmt.Errorf("device not found: from_id=%s, from=%s", fromID, from)
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
