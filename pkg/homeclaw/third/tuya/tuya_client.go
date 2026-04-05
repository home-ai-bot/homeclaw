package tuya

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third"
)

const (
	// BrandTuya is the brand identifier for Tuya platform.
	BrandTuya = "tuya"

	// Cache key constants
	cacheKeyHomes     = "tuya-homes"
	cacheKeyDeviceAll = "tuya-devices-all"
)

// cacheKeyRooms returns the JSONStore key for rooms of a home.
func cacheKeyRooms(homeID string) string { return "tuya-rooms-" + homeID }

// cacheKeyDevices returns the JSONStore key for devices of a home.
func cacheKeyDevices(homeID string) string { return "tuya-devices-" + homeID }

// cacheKeyModel returns the JSONStore key for a device's Thing Model.
func cacheKeyModel(deviceID string) string { return "tuya-model-" + deviceID }

// TuyaClient implements std.Client for Tuya platform using the Open Platform API.
// All query results are persisted via JSONStore for cross-restart caching.
type TuyaClient struct {
	openAPI  *TuyaOpenAPI // Tuya Open Platform API client
	apiKey   string       // Tuya Open Platform API key
	email    string       // Tuya Smart App account email (for RTSP streams)
	password string       // Tuya Smart App account password (for RTSP streams)
	region   string       // Tuya region (for RTSP streams)

	store *data.JSONStore // Persistent cache store
}

// NewTuyaClient creates a new TuyaClient instance with the given store and API key.
// apiKey may be empty when the token has not been configured yet; the client is
// still created so that it can be registered in CLITool and have its key set later
// via SetAPIKey. Any operation that requires the API key will return an error via
// checkAPI() until SetAPIKey is called.
func NewTuyaClient(store *data.JSONStore, apiKey string, email string, password string, region string) (*TuyaClient, error) {
	if store == nil {
		return nil, errors.New("store cannot be nil")
	}
	tc := &TuyaClient{
		store:    store,
		apiKey:   apiKey,
		email:    email,
		password: password,
		region:   region,
	}
	if apiKey != "" {
		tc.openAPI = NewTuyaOpenAPI(apiKey)
	}
	return tc, nil
}

// GetAPIKey returns the configured Tuya Open Platform API key.
func (tc *TuyaClient) GetAPIKey() string {
	return tc.apiKey
}

// SetAPIKey sets the Tuya Open Platform API key.
func (tc *TuyaClient) SetAPIKey(apiKey string) {
	tc.apiKey = apiKey
	tc.openAPI = NewTuyaOpenAPI(apiKey)
}

// SetCredentials sets the Tuya Smart App account credentials (email, password, region).
// These credentials are used for RTSP stream authentication.
func (tc *TuyaClient) SetCredentials(email, password, region string) {
	tc.email = email
	tc.password = password
	tc.region = region
}

// GetEmail returns the configured email.
func (tc *TuyaClient) GetEmail() string {
	return tc.email
}

// GetPassword returns the configured password.
func (tc *TuyaClient) GetPassword() string {
	return tc.password
}

// GetRegion returns the configured region.
func (tc *TuyaClient) GetRegion() string {
	return tc.region
}

// Brand returns the brand identifier.
func (tc *TuyaClient) Brand() string {
	return BrandTuya
}

// checkAPI returns an error if the openAPI client is not initialized.
func (tc *TuyaClient) checkAPI() error {
	if tc.openAPI == nil {
		return errors.New("Must Confirm!API key not configured, please set API key via WithAPIKey")
	}
	return nil
}

// GetHomes returns all homes visible to the authenticated user.
// Results are cached in the JSONStore under key "tuya-homes".
func (tc *TuyaClient) GetHomes() ([]*third.HomeInfo, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	homeList, err := tc.openAPI.GetHomes()
	if err != nil {
		return nil, fmt.Errorf("failed to get home list: %w", err)
	}

	var result []*third.HomeInfo
	if homeList != nil {
		for _, home := range homeList.Homes {
			result = append(result, &third.HomeInfo{
				ID:   home.HomeID,
				Name: home.Name,
			})
		}
	}

	_ = tc.store.Write(cacheKeyHomes, result)
	return result, nil
}

// GetRooms returns all rooms for the given homeID.
// Results are cached in the JSONStore under key "tuya-rooms-{homeID}".
func (tc *TuyaClient) GetRooms(homeID string) ([]*data.Space, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	roomList, err := tc.openAPI.GetRooms(homeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room list: %w", err)
	}

	var result []*data.Space
	for _, room := range roomList {
		result = append(result, &data.Space{
			Name: room.RoomName,
			From: map[string]string{
				BrandTuya: room.RoomID,
			},
		})
	}

	_ = tc.store.Write(cacheKeyRooms(homeID), result)
	return result, nil
}

// GetDevices returns all devices for the given homeID.
// If homeID is empty, returns devices from all homes.
// Results are cached in the JSONStore.
func (tc *TuyaClient) GetDevices(homeID string) ([]*data.Device, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}

	if homeID == "" {
		return nil, errors.New("please specify homeID")
	}
	key := cacheKeyDevices(homeID)

	var devices []*DeviceInfo
	var err error

	devices, err = tc.openAPI.GetHomeDevices(homeID)

	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	var result []*data.Device
	for _, device := range devices {
		result = append(result, &data.Device{
			FromID: device.DeviceID,
			From:   BrandTuya,
			Name:   device.Name,
			Type:   device.Category,
			URN:    device.ProductID,
		})
	}

	_ = tc.store.Write(key, result)
	return result, nil
}

// GetSpec fetches the capability specification for a device (Thing Model).
// Results are cached in the JSONStore under key "tuya-model-{deviceID}".
func (tc *TuyaClient) GetSpec(deviceID string) (*third.SpecInfo, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}

	// Try cache
	var cached ThingModel
	if err := tc.store.Read(cacheKeyModel(deviceID), &cached); err == nil && cached.ModelID != "" {
		return modelToSpecInfo(deviceID, &cached), nil
	}

	model, err := tc.openAPI.GetDeviceModel(deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device model: %w", err)
	}

	if model != nil {
		_ = tc.store.Write(cacheKeyModel(deviceID), model)
	}

	return modelToSpecInfo(deviceID, model), nil
}

// modelToSpecInfo converts a ThingModel to SpecInfo.
func modelToSpecInfo(deviceID string, model *ThingModel) *third.SpecInfo {
	if model == nil {
		return &third.SpecInfo{
			DeviceID: deviceID,
			Model:    "",
			Raw:      "{}",
			Extra:    map[string]any{"platform": BrandTuya},
		}
	}

	raw, _ := json.Marshal(model)

	return &third.SpecInfo{
		DeviceID: deviceID,
		Model:    model.ModelID,
		Raw:      string(raw),
		Extra: map[string]any{
			"platform": BrandTuya,
			"services": model.Services,
		},
	}
}

// Execute sends an action command to a device.
func (tc *TuyaClient) Execute(params map[string]any) (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}

	deviceID, ok := params["device_id"].(string)
	if !ok || deviceID == "" {
		return nil, errors.New("device_id is required")
	}

	props := make(map[string]any)
	for k, v := range params {
		if k != "device_id" && k != "action" {
			props[k] = v
		}
	}

	if len(props) == 0 {
		return nil, errors.New("no properties to set")
	}

	if err := tc.openAPI.IssueProperties(deviceID, props); err != nil {
		return nil, fmt.Errorf("failed to execute command: %w", err)
	}
	return map[string]any{"success": true}, nil
}

// GetProps reads property values from a device.
// Results are cached in the JSONStore under key "tuya-detail-{deviceID}".
func (tc *TuyaClient) GetProps(params map[string]any) (any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}

	deviceID, ok := params["device_id"].(string)
	if !ok || deviceID == "" {
		return nil, errors.New("device_id is required")
	}

	detail, err := tc.openAPI.GetDeviceDetail(deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device detail: %w", err)
	}
	if detail == nil {
		return nil, errors.New("device not found")
	}

	return detail.Properties, nil
}

// SetProps sets property values on a device.
func (tc *TuyaClient) SetProps(params map[string]any) (any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}

	deviceID, ok := params["device_id"].(string)
	if !ok || deviceID == "" {
		return nil, errors.New("device_id is required")
	}

	props := make(map[string]any)
	for k, v := range params {
		if k != "device_id" {
			props[k] = v
		}
	}

	if len(props) == 0 {
		return nil, errors.New("no properties to set")
	}

	if err := tc.openAPI.IssueProperties(deviceID, props); err != nil {
		return nil, fmt.Errorf("failed to set properties: %w", err)
	}

	return map[string]any{"success": true}, nil
}

// EnableEvent starts event subscription for a device.
func (tc *TuyaClient) EnableEvent(params map[string]any) error {
	return errors.New("event subscription not implemented for Tuya")
}

// DisableEvent stops event subscription for a device.
func (tc *TuyaClient) DisableEvent(params map[string]any) error {
	return errors.New("event subscription not implemented for Tuya")
}

// GetRtspStr returns the go2rtc-compatible RTSP stream URL for the given deviceID.
// Returns an empty string and no error if email or password is not configured.
// URL format: tuya://protect-{region}.ismartlife.me?device_id={deviceID}&email={email}&password={password}
func (tc *TuyaClient) GetRtspStr(deviceID string) (string, error) {
	// Return empty string if email or password is not configured
	if tc.email == "" || tc.password == "" {
		return "", nil
	}

	// Determine the host based on region
	host := "protect-eu.ismartlife.me"
	if tc.region != "" {
		switch tc.region {
		case "us":
			host = "protect-us.ismartlife.me"
		case "cn":
			host = "protect-cn.ismartlife.me"
		case "eu":
			host = "protect-eu.ismartlife.me"
		default:
			host = "protect-" + tc.region + ".ismartlife.me"
		}
	}

	// Construct the RTSP URL
	url := fmt.Sprintf("tuya://%s?device_id=%s&email=%s&password=%s",
		host, deviceID, tc.email, tc.password)

	return url, nil
}

// ClearCache removes all cached data from the JSONStore.
func (tc *TuyaClient) ClearCache() {
	keys, err := tc.store.List()
	if err != nil {
		return
	}
	for _, key := range keys {
		if len(key) >= 5 && key[:5] == "tuya-" {
			_ = tc.store.Remove(key)
		}
	}
}

// GetDeviceModel returns the Thing Model of a device.
// Results are cached in the JSONStore under key "tuya-model-{deviceID}".
func (tc *TuyaClient) GetDeviceModel(deviceID string) (*ThingModel, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}

	// Try cache
	var cached ThingModel
	if err := tc.store.Read(cacheKeyModel(deviceID), &cached); err == nil && cached.ModelID != "" {
		return &cached, nil
	}

	model, err := tc.openAPI.GetDeviceModel(deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device model: %w", err)
	}

	if model != nil {
		_ = tc.store.Write(cacheKeyModel(deviceID), model)
	}

	return model, nil
}

// IssueProperties sends control commands to a device.
func (tc *TuyaClient) IssueProperties(deviceID string, properties map[string]any) error {
	if err := tc.checkAPI(); err != nil {
		return err
	}

	if err := tc.openAPI.IssueProperties(deviceID, properties); err != nil {
		return fmt.Errorf("failed to issue properties: %w", err)
	}

	return nil
}

// IsOnline checks if a device is online.
func (tc *TuyaClient) IsOnline(deviceID string) (bool, error) {
	// Always fetch fresh detail for online status
	detail, err := tc.openAPI.GetDeviceDetail(deviceID)
	if err != nil {
		return false, fmt.Errorf("failed to get device detail: %w", err)
	}
	if detail == nil {
		return false, errors.New("device not found")
	}
	return detail.Online, nil
}

// RenameDevice renames a device.
func (tc *TuyaClient) RenameDevice(deviceID, name string) error {
	if err := tc.checkAPI(); err != nil {
		return err
	}
	return tc.openAPI.RenameDevice(deviceID, name)
}

// GetWeather returns weather information for a location.
func (tc *TuyaClient) GetWeather(lat, lon string, codes []string) (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	return tc.openAPI.GetWeather(lat, lon, codes)
}

// SendSMS sends an SMS message to the current user.
func (tc *TuyaClient) SendSMS(message string) error {
	if err := tc.checkAPI(); err != nil {
		return err
	}
	return tc.openAPI.SendSMS(message)
}

// SendVoice sends a voice notification to the current user.
func (tc *TuyaClient) SendVoice(message string) error {
	if err := tc.checkAPI(); err != nil {
		return err
	}
	return tc.openAPI.SendVoice(message)
}

// SendMail sends an email to the current user.
func (tc *TuyaClient) SendMail(subject, content string) error {
	if err := tc.checkAPI(); err != nil {
		return err
	}
	return tc.openAPI.SendMail(subject, content)
}

// SendPush sends an App push notification to the current user.
func (tc *TuyaClient) SendPush(subject, content string) error {
	if err := tc.checkAPI(); err != nil {
		return err
	}
	return tc.openAPI.SendPush(subject, content)
}

// GetStatisticsConfig returns hourly statistics configuration for all user devices.
func (tc *TuyaClient) GetStatisticsConfig() (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	return tc.openAPI.GetStatisticsConfig()
}

// GetStatisticsData returns hourly statistics values for a device.
func (tc *TuyaClient) GetStatisticsData(deviceID, dpCode, statisticType, startTime, endTime string) (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	return tc.openAPI.GetStatisticsData(deviceID, dpCode, statisticType, startTime, endTime)
}

// IPCCaptureAllocate allocates a cloud capture (snapshot or short video).
func (tc *TuyaClient) IPCCaptureAllocate(deviceID, captureType string, picCount, videoDurationSeconds int, homeID string) (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	return tc.openAPI.IPCCaptureAllocate(deviceID, captureType, picCount, videoDurationSeconds, homeID)
}

// IPCCaptureResolve resolves capture access URL.
func (tc *TuyaClient) IPCCaptureResolve(deviceID, captureType, bucket string, params map[string]any) (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	return tc.openAPI.IPCCaptureResolve(deviceID, captureType, bucket, params)
}

// IPCCapturePic captures a snapshot from an IPC camera.
func (tc *TuyaClient) IPCCapturePic(deviceID string, picCount int, homeID string) (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	return tc.openAPI.IPCCaptureAllocate(deviceID, "PIC", picCount, 0, homeID)
}

// IPCCaptureVideo captures a short video from an IPC camera.
func (tc *TuyaClient) IPCCaptureVideo(deviceID string, durationSeconds int, homeID string) (map[string]any, error) {
	if err := tc.checkAPI(); err != nil {
		return nil, err
	}
	return tc.openAPI.IPCCaptureAllocate(deviceID, "VIDEO", 0, durationSeconds, homeID)
}
