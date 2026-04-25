package tuya

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
)

// API key prefix to Open Platform base URL mapping
var prefixToBaseURL = map[string]string{
	"AY": "https://openapi.tuyacn.com",      // China Data Center
	"AZ": "https://openapi.tuyaus.com",      // US West Data Center
	"EU": "https://openapi.tuyaeu.com",      // Central Europe Data Center
	"IN": "https://openapi.tuyain.com",      // India Data Center
	"UE": "https://openapi-ueaz.tuyaus.com", // US East Data Center
	"WE": "https://openapi-weaz.tuyaeu.com", // Western Europe Data Center
	"SG": "https://openapi-sg.iotbing.com",  // Singapore Data Center
}

// TuyaOpenAPI implements the Tuya Open Platform 2C end-user API client.
// Authentication is via API key (Authorization: Bearer {api-key}).
type TuyaOpenAPI struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// TuyaOpenAPIError represents an error from the Tuya Open Platform API.
type TuyaOpenAPIError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (e *TuyaOpenAPIError) Error() string {
	return fmt.Sprintf("Tuya API error %d: %s", e.Code, e.Msg)
}

// NewTuyaOpenAPI creates a new Tuya Open Platform API client.
// The base URL is auto-detected from the API key prefix.
func NewTuyaOpenAPI(apiKey string) *TuyaOpenAPI {
	baseURL := resolveBaseURLFromAPIKey(apiKey)
	return &TuyaOpenAPI{
		apiKey:  apiKey,
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// resolveBaseURLFromAPIKey extracts the data center base URL from the API key prefix.
func resolveBaseURLFromAPIKey(apiKey string) string {
	key := apiKey
	if len(key) > 3 && key[:3] == "sk-" {
		key = key[3:]
	}
	if len(key) >= 2 {
		prefix := key[:2]
		if url, ok := prefixToBaseURL[prefix]; ok {
			return url
		}
	}
	// Default to China if prefix not recognized
	return "https://openapi.tuyacn.com"
}

// SetBaseURL sets a custom base URL (for testing or custom regions).
func (api *TuyaOpenAPI) SetBaseURL(baseURL string) {
	api.baseURL = baseURL
}

// doRequest performs an HTTP request and returns the result.
func (api *TuyaOpenAPI) doRequest(method, path string, body any) (map[string]any, error) {
	logger.Infof("[Tuya OpenAPI] doRequest - Method: %s, Path: %s", method, path)

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			logger.Errorf("[Tuya OpenAPI] doRequest - Failed to marshal request body: %v", err)
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
		logger.Infof("[Tuya OpenAPI] doRequest - Request body JSON: %s", string(jsonBody))
	}

	url := api.baseURL + path
	logger.Infof("[Tuya OpenAPI] doRequest - Full URL: %s", url)

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		logger.Errorf("[Tuya OpenAPI] doRequest - Failed to create request: %v", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+api.apiKey)
	req.Header.Set("Content-Type", "application/json")

	logger.Infof("[Tuya OpenAPI] doRequest - Sending request...")
	resp, err := api.client.Do(req)
	if err != nil {
		logger.Errorf("[Tuya OpenAPI] doRequest - Request failed: %v", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	logger.Infof("[Tuya OpenAPI] doRequest - Response status: %s", resp.Status)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Errorf("[Tuya OpenAPI] doRequest - Failed to read response: %v", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	logger.Infof("[Tuya OpenAPI] doRequest - Response body: %s", string(respBody))

	var result struct {
		Success bool            `json:"success"`
		Code    int             `json:"code"`
		Msg     string          `json:"msg"`
		Result  json.RawMessage `json:"result"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		logger.Errorf("[Tuya OpenAPI] doRequest - Failed to parse response: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.Success {
		logger.Errorf("[Tuya OpenAPI] doRequest - API error: code=%d, msg=%s", result.Code, result.Msg)
		return nil, &TuyaOpenAPIError{Code: result.Code, Msg: result.Msg}
	}

	logger.Infof("[Tuya OpenAPI] doRequest - API success, parsing result...")

	var data map[string]any
	if len(result.Result) > 0 {
		if err := json.Unmarshal(result.Result, &data); err != nil {
			// Result might not be a map, try raw message
			logger.Warnf("[Tuya OpenAPI] doRequest - Result is not a map, returning raw string")
			return map[string]any{"raw": string(result.Result)}, nil
		}
	}
	logger.Infof("[Tuya OpenAPI] doRequest - Returning data: %+v", data)
	return data, nil
}

// doGet performs a GET request.
func (api *TuyaOpenAPI) doGet(path string) (map[string]any, error) {
	return api.doRequest(http.MethodGet, path, nil)
}

// doPost performs a POST request.
func (api *TuyaOpenAPI) doPost(path string, body any) (map[string]any, error) {
	return api.doRequest(http.MethodPost, path, body)
}

// ─── Device Query ───

// GetDeviceDetail returns detailed information of a device including current property states.
func (api *TuyaOpenAPI) GetDeviceDetail(deviceID string) (*DeviceDetail, error) {
	result, err := api.doGet("/v1.0/end-user/devices/" + deviceID + "/detail")
	if err != nil {
		return nil, err
	}

	// Log raw response to check for product_id field
	logger.Infof("[Tuya OpenAPI] GetDeviceDetail raw response for device %s: %+v", deviceID, result)

	if result == nil {
		return nil, nil
	}

	// Re-marshal and unmarshal to get typed result
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var detail DeviceDetail
	if err := json.Unmarshal(data, &detail); err != nil {
		return nil, err
	}

	logger.Infof("[Tuya OpenAPI] GetDeviceDetail parsed: DeviceID=%s, Category=%s, ProductName=%s",
		detail.DeviceID, detail.Category, detail.ProductName)

	return &detail, nil
}

// ─── Device Control ───

// GetDeviceModel returns the Thing Model of a device.
func (api *TuyaOpenAPI) GetDeviceModel(deviceID string) (*ThingModel, error) {
	result, err := api.doGet("/v1.0/end-user/devices/" + deviceID + "/model")
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	modelRaw, ok := result["model"]
	if !ok {
		return nil, nil
	}

	// model is a JSON string that needs to be parsed again
	modelStr, ok := modelRaw.(string)
	if !ok {
		return nil, errors.New("model is not a string")
	}

	var model ThingModel
	if err := json.Unmarshal([]byte(modelStr), &model); err != nil {
		return nil, fmt.Errorf("failed to parse model: %w", err)
	}

	return &model, nil
}

// IssueProperties sends control commands to a device.
func (api *TuyaOpenAPI) IssueProperties(deviceID string, properties map[string]any) error {
	// Log the input parameters
	logger.Infof("[Tuya OpenAPI] IssueProperties START - DeviceID: %s, Properties: %+v", deviceID, properties)

	// Properties must be serialized to a JSON string
	propsJSON, err := json.Marshal(properties)
	if err != nil {
		logger.Errorf("[Tuya OpenAPI] IssueProperties FAILED - Failed to serialize properties: %v", err)
		return fmt.Errorf("failed to serialize properties: %w", err)
	}

	logger.Infof("[Tuya OpenAPI] IssueProperties - Serialized properties JSON: %s", string(propsJSON))

	body := map[string]any{
		"properties": string(propsJSON),
	}

	logger.Infof("[Tuya OpenAPI] IssueProperties - Request body: %+v", body)

	result, err := api.doPost("/v1.0/end-user/devices/"+deviceID+"/shadow/properties/issue", body)
	if err != nil {
		logger.Errorf("[Tuya OpenAPI] IssueProperties FAILED - DeviceID: %s, Error: %v", deviceID, err)
		return err
	}

	logger.Infof("[Tuya OpenAPI] IssueProperties SUCCESS - DeviceID: %s, Response: %+v", deviceID, result)
	return nil
}

// RenameDevice renames a device.
func (api *TuyaOpenAPI) RenameDevice(deviceID, name string) error {
	body := map[string]any{"name": name}
	_, err := api.doPost("/v1.0/end-user/devices/"+deviceID+"/attribute", body)
	return err
}

// ─── Home Management ───

// GetHomes returns all homes for the user.
func (api *TuyaOpenAPI) GetHomes() (*HomeListResult, error) {
	result, err := api.doGet("/v1.0/end-user/homes/all")
	if err != nil {
		return nil, err
	}
	logger.Debugf("GetHomes result: %v", result)
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var homeList HomeListResult
	if err := json.Unmarshal(data, &homeList); err != nil {
		return nil, err
	}
	return &homeList, nil
}

// GetRooms returns all rooms in a home.
func (api *TuyaOpenAPI) GetRooms(homeID string) ([]*RoomInfo, error) {
	result, err := api.doGet("/v1.0/end-user/homes/" + homeID + "/rooms")
	logger.Debugf("GetRooms result: %v", result["rooms"])
	if err != nil {
		return nil, err
	}

	roomsRaw, ok := result["rooms"]
	if !ok {
		return nil, nil
	}

	data, err := json.Marshal(roomsRaw)
	if err != nil {
		return nil, err
	}

	var rooms []*RoomInfo
	if err := json.Unmarshal(data, &rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

// GetHomeDevices returns all devices in a home.
func (api *TuyaOpenAPI) GetHomeDevices(homeID string) ([]*DeviceInfo, error) {
	result, err := api.doGet("/v1.0/end-user/homes/" + homeID + "/devices")
	if err != nil {
		return nil, err
	}

	// Log raw API response for debugging
	logger.Infof("[Tuya OpenAPI] GetHomeDevices raw response for home %s: %+v", homeID, result)

	devicesRaw, ok := result["devices"]
	if !ok {
		logger.Warnf("[Tuya OpenAPI] GetHomeDevices: no 'devices' field in response")
		return nil, nil
	}

	data, err := json.Marshal(devicesRaw)
	if err != nil {
		logger.Errorf("[Tuya OpenAPI] GetHomeDevices: failed to marshal devices: %v", err)
		return nil, err
	}

	// Log raw JSON before unmarshaling
	logger.Infof("[Tuya OpenAPI] GetHomeDevices raw devices JSON: %s", string(data))

	var devices []*DeviceInfo
	if err := json.Unmarshal(data, &devices); err != nil {
		logger.Errorf("[Tuya OpenAPI] GetHomeDevices: failed to unmarshal devices: %v", err)
		return nil, err
	}

	// Log parsed devices with key fields
	logger.Infof("[Tuya OpenAPI] GetHomeDevices: parsed %d devices", len(devices))
	for i, dev := range devices {
		if dev != nil {
			logger.Infof("[Tuya OpenAPI] Device[%d]: ID=%s, Name=%s, Category=%s, ProductID=%s",
				i, dev.DeviceID, dev.Name, dev.Category, dev.ProductID)
		}
	}

	return devices, nil
}

// GetRoomDevices returns all devices in a room.
func (api *TuyaOpenAPI) GetRoomDevices(roomID string) ([]*DeviceInfo, error) {
	result, err := api.doGet("/v1.0/end-user/homes/room/" + roomID + "/devices")
	if err != nil {
		return nil, err
	}

	devicesRaw, ok := result["devices"]
	if !ok {
		return nil, nil
	}

	data, err := json.Marshal(devicesRaw)
	if err != nil {
		return nil, err
	}

	var devices []*DeviceInfo
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, err
	}
	return devices, nil
}

// ─── Weather Service ───

// GetWeather returns weather information for a location.
// codes: weather attribute list, defaults to temperature, humidity, condition, hourly forecast
func (api *TuyaOpenAPI) GetWeather(lat, lon string, codes []string) (map[string]any, error) {
	if len(codes) == 0 {
		codes = []string{"w.temp", "w.humidity", "w.condition", "w.hour.7"}
	}

	codesJSON, _ := json.Marshal(codes)
	path := fmt.Sprintf("/v1.0/end-user/services/weather/recent?lat=%s&lon=%s&codes=%s",
		lat, lon, string(codesJSON))

	return api.doGet(path)
}

// ─── Notifications ───

// SendSMS sends an SMS message to the current user.
func (api *TuyaOpenAPI) SendSMS(message string) error {
	_, err := api.doPost("/v1.0/end-user/services/sms/self-send", map[string]any{"message": message})
	return err
}

// SendVoice sends a voice notification to the current user.
func (api *TuyaOpenAPI) SendVoice(message string) error {
	_, err := api.doPost("/v1.0/end-user/services/voice/self-send", map[string]any{"message": message})
	return err
}

// SendMail sends an email to the current user.
func (api *TuyaOpenAPI) SendMail(subject, content string) error {
	_, err := api.doPost("/v1.0/end-user/services/mail/self-send", map[string]any{
		"subject": subject,
		"content": content,
	})
	return err
}

// SendPush sends an App push notification to the current user.
func (api *TuyaOpenAPI) SendPush(subject, content string) error {
	_, err := api.doPost("/v1.0/end-user/services/push/self-send", map[string]any{
		"subject": subject,
		"content": content,
	})
	return err
}

// ─── Data Statistics ───

// GetStatisticsConfig returns hourly statistics configuration for all user devices.
func (api *TuyaOpenAPI) GetStatisticsConfig() (map[string]any, error) {
	return api.doGet("/v1.0/end-user/statistics/hour/config")
}

// GetStatisticsData returns hourly statistics values for a device.
// startTime/endTime format: yyyyMMddHH (e.g., 2024010100)
// statisticType: SUM, COUNT, MAX, MIN, MINUS
// dpCode: data point code (e.g., ele_usage for electricity)
func (api *TuyaOpenAPI) GetStatisticsData(deviceID, dpCode, statisticType, startTime, endTime string) (map[string]any, error) {
	path := fmt.Sprintf("/v1.0/end-user/statistics/hour/data?dev_id=%s&dp_code=%s&statistic_type=%s&start_time=%s&end_time=%s",
		deviceID, dpCode, statisticType, startTime, endTime)
	return api.doGet(path)
}

// ─── IPC Cloud Capture ───

// IPCCaptureAllocate allocates a cloud capture (snapshot or short video).
// captureType: "PIC" for snapshot, "VIDEO" for short video
func (api *TuyaOpenAPI) IPCCaptureAllocate(deviceID, captureType string, picCount, videoDurationSeconds int, homeID string) (map[string]any, error) {
	captureParams := map[string]any{
		"device_id":    deviceID,
		"capture_type": captureType,
	}
	if picCount > 0 {
		captureParams["pic_count"] = picCount
	}
	if videoDurationSeconds > 0 {
		captureParams["video_duration_seconds"] = videoDurationSeconds
	}
	if homeID != "" {
		captureParams["home_id"] = homeID
	}

	captureJSON, _ := json.Marshal(captureParams)
	return api.doPost("/v1.0/end-user/ipc/"+deviceID+"/capture/allocate", map[string]any{
		"capture_json": string(captureJSON),
	})
}

// IPCCaptureResolve resolves capture access URL.
// userPrivacyConsentAccepted: true for decrypted URLs
func (api *TuyaOpenAPI) IPCCaptureResolve(deviceID, captureType, bucket string, params map[string]any) (map[string]any, error) {
	resolveParams := map[string]any{
		"device_id":    deviceID,
		"capture_type": captureType,
		"bucket":       bucket,
	}
	for k, v := range params {
		resolveParams[k] = v
	}

	resolveJSON, _ := json.Marshal(resolveParams)
	return api.doPost("/v1.0/end-user/ipc/"+deviceID+"/capture/resolve", map[string]any{
		"resolve_json": string(resolveJSON),
	})
}

// ─── Data Types ───

// HomeInfo represents a home information.
type HomeInfo struct {
	HomeID     string   `json:"home_id"`
	Name       string   `json:"name"`
	CreateTime int64    `json:"create_time,omitempty"`
	Admin      bool     `json:"admin"`
	Latitude   *float64 `json:"latitude,omitempty"`
	Longitude  *float64 `json:"longitude,omitempty"`
	GeoName    string   `json:"geo_name,omitempty"`
	Role       string   `json:"role"`
	Status     bool     `json:"status"`
}

// HomeListResult represents the result of GetHomes API.
type HomeListResult struct {
	Homes []*HomeInfo `json:"homes"`
	Total int         `json:"total"`
}

// RoomInfo represents a room information.
type RoomInfo struct {
	RoomID      string `json:"room_id"`
	Name        string `json:"name"`
	DeviceCount int    `json:"device_count"`
}

// DeviceInfo represents basic device information.
type DeviceInfo struct {
	DeviceID     string `json:"device_id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	CategoryName string `json:"category_name"`
	ProductID    string `json:"product_id"`
	Online       bool   `json:"online"`
	RoomID       string `json:"room_id,omitempty"`
}

// DeviceDetail represents detailed device information including current property states.
type DeviceDetail struct {
	DeviceID                string         `json:"device_id"`
	Name                    string         `json:"name"`
	Category                string         `json:"category"`
	CategoryName            string         `json:"category_name"`
	ProductName             string         `json:"product_name"`
	Online                  bool           `json:"online"`
	FirmwareVersion         string         `json:"firmware_version"`
	FirmwareUpdateAvailable bool           `json:"firmware_update_available"`
	Properties              map[string]any `json:"properties"`
}

// ThingModel represents the capability specification of a device.
type ThingModel struct {
	ModelID  string    `json:"modelId"`
	Services []Service `json:"services"`
}

// Service represents a service in the Thing Model.
type Service struct {
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Properties  []Property `json:"properties"`
}

// Property represents a property in a service.
type Property struct {
	AbilityID   int      `json:"abilityId"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	AccessMode  string   `json:"accessMode"` // ro, wr, rw
	TypeSpec    TypeSpec `json:"typeSpec"`
}

// TypeSpec represents the data type specification of a property.
type TypeSpec struct {
	Type   string   `json:"type"`             // bool, value, enum, string, etc.
	Min    int      `json:"min,omitempty"`    // For value type
	Max    int      `json:"max,omitempty"`    // For value type
	Step   int      `json:"step,omitempty"`   // For value type
	Unit   string   `json:"unit,omitempty"`   // For value type
	Scale  int      `json:"scale,omitempty"`  // For value type
	MaxLen int      `json:"maxlen,omitempty"` // For string type
	Range  []string `json:"range,omitempty"`  // For enum type
}
