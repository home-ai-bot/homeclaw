package intent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// deviceID generates a simple time-based ID for a new device.
func deviceID() string {
	return fmt.Sprintf("device-%d", time.Now().UnixNano())
}

// DeviceMgmtIntent handles device management intents (device.add, device.scan,
// device.remove, device.rename, device.move, device.query.status).
type DeviceMgmtIntent struct {
	deviceStore data.DeviceStore
	spaceStore  data.SpaceStore
}

// NewDeviceMgmtIntent creates a DeviceMgmtIntent backed by the given stores.
// If either store is nil the handler falls through to the large model.
func NewDeviceMgmtIntent(deviceStore data.DeviceStore, spaceStore data.SpaceStore) *DeviceMgmtIntent {
	return &DeviceMgmtIntent{deviceStore: deviceStore, spaceStore: spaceStore}
}

// Types implements Intent.
func (d *DeviceMgmtIntent) Types() []IntentType {
	return []IntentType{
		IntentDeviceAdd,
		IntentDeviceScan,
		IntentDeviceRemove,
		IntentDeviceRename,
		IntentDeviceMove,
		IntentDeviceQueryStatus,
	}
}

// Run executes the device management operation and returns a direct reply.
func (d *DeviceMgmtIntent) Run(_ context.Context, ictx IntentContext) IntentResponse {
	if d.deviceStore == nil {
		return IntentResponse{Handled: false}
	}

	switch ictx.Result.Type {
	case IntentDeviceAdd:
		return d.handleAdd(ictx)
	case IntentDeviceScan:
		return d.handleScan()
	case IntentDeviceRemove:
		return d.handleRemove(ictx)
	case IntentDeviceRename:
		return d.handleRename(ictx)
	case IntentDeviceMove:
		return d.handleMove(ictx)
	case IntentDeviceQueryStatus:
		return d.handleQueryStatus(ictx)
	default:
		return IntentResponse{Handled: false}
	}
}

func (d *DeviceMgmtIntent) handleAdd(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "device_name")
	if name == "" {
		return IntentResponse{Handled: false}
	}

	spaceName := entityString(ictx.Result.Entities, "space_name")
	spaceID := ""
	if spaceName != "" && d.spaceStore != nil {
		if sp, err := d.spaceStore.FindByName(spaceName); err == nil {
			spaceID = sp.ID
		}
	}

	device := data.Device{
		ID:      deviceID(),
		Name:    name,
		SpaceID: spaceID,
		Props:   map[string]string{},
		AddedAt: time.Now(),
	}
	if err := d.deviceStore.Save(device); err != nil {
		return errResponse(fmt.Sprintf("添加设备「%s」失败：%s", name, err.Error()), err)
	}
	msg := fmt.Sprintf("已添加设备「%s」。", name)
	if spaceName != "" && spaceID == "" {
		msg += fmt.Sprintf("（未找到空间「%s」，设备未分配房间）", spaceName)
	}
	return IntentResponse{Handled: true, Response: msg}
}

func (d *DeviceMgmtIntent) handleScan() IntentResponse {
	// Actual network scanning requires hardware integration; report back to the
	// large model so it can guide the user through the pairing wizard.
	return IntentResponse{Handled: false}
}

func (d *DeviceMgmtIntent) handleRemove(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "device_name")
	if name == "" {
		return IntentResponse{Handled: false}
	}

	devices, err := d.deviceStore.GetAll()
	if err != nil {
		return errResponse(fmt.Sprintf("查询设备失败：%s", err.Error()), err)
	}
	for _, dev := range devices {
		if strings.EqualFold(dev.Name, name) {
			if err := d.deviceStore.Delete(dev.ID); err != nil {
				return errResponse(fmt.Sprintf("删除设备「%s」失败：%s", name, err.Error()), err)
			}
			return IntentResponse{Handled: true, Response: fmt.Sprintf("已删除设备「%s」。", name)}
		}
	}
	return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到设备「%s」。", name)}
}

func (d *DeviceMgmtIntent) handleRename(ictx IntentContext) IntentResponse {
	oldName := entityString(ictx.Result.Entities, "device_name")
	newName := entityString(ictx.Result.Entities, "new_name")
	if oldName == "" || newName == "" {
		return IntentResponse{Handled: false}
	}

	devices, err := d.deviceStore.GetAll()
	if err != nil {
		return errResponse(fmt.Sprintf("查询设备失败：%s", err.Error()), err)
	}
	for _, dev := range devices {
		if strings.EqualFold(dev.Name, oldName) {
			dev.Name = newName
			if err := d.deviceStore.Save(dev); err != nil {
				return errResponse(fmt.Sprintf("重命名设备失败：%s", err.Error()), err)
			}
			return IntentResponse{Handled: true, Response: fmt.Sprintf("已将设备「%s」重命名为「%s」。", oldName, newName)}
		}
	}
	return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到设备「%s」。", oldName)}
}

func (d *DeviceMgmtIntent) handleMove(ictx IntentContext) IntentResponse {
	deviceName := entityString(ictx.Result.Entities, "device_name")
	spaceName := entityString(ictx.Result.Entities, "space_name")
	if deviceName == "" || spaceName == "" {
		return IntentResponse{Handled: false}
	}

	// Resolve space.
	var spaceID string
	if d.spaceStore != nil {
		sp, err := d.spaceStore.FindByName(spaceName)
		if err != nil {
			if err == data.ErrRecordNotFound {
				return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到空间「%s」。", spaceName)}
			}
			return errResponse(fmt.Sprintf("查询空间失败：%s", err.Error()), err)
		}
		spaceID = sp.ID
	}

	devices, err := d.deviceStore.GetAll()
	if err != nil {
		return errResponse(fmt.Sprintf("查询设备失败：%s", err.Error()), err)
	}
	for _, dev := range devices {
		if strings.EqualFold(dev.Name, deviceName) {
			dev.SpaceID = spaceID
			if err := d.deviceStore.Save(dev); err != nil {
				return errResponse(fmt.Sprintf("移动设备失败：%s", err.Error()), err)
			}
			return IntentResponse{Handled: true, Response: fmt.Sprintf("已将设备「%s」移动到「%s」。", deviceName, spaceName)}
		}
	}
	return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到设备「%s」。", deviceName)}
}

func (d *DeviceMgmtIntent) handleQueryStatus(ictx IntentContext) IntentResponse {
	name := entityString(ictx.Result.Entities, "device_name")

	// Query a specific device.
	if name != "" {
		devices, err := d.deviceStore.GetAll()
		if err != nil {
			return errResponse(fmt.Sprintf("查询设备失败：%s", err.Error()), err)
		}
		for _, dev := range devices {
			if strings.EqualFold(dev.Name, name) {
				return IntentResponse{
					Handled:  true,
					Response: formatDeviceStatus(dev),
				}
			}
		}
		return IntentResponse{Handled: true, Response: fmt.Sprintf("未找到设备「%s」。", name)}
	}

	// Query all devices.
	devices, err := d.deviceStore.GetAll()
	if err != nil {
		return errResponse(fmt.Sprintf("查询设备列表失败：%s", err.Error()), err)
	}
	if len(devices) == 0 {
		return IntentResponse{Handled: true, Response: "当前没有任何设备。"}
	}
	lines := make([]string, 0, len(devices))
	for _, dev := range devices {
		lines = append(lines, fmt.Sprintf("「%s」（%s）", dev.Name, dev.Brand))
	}
	return IntentResponse{
		Handled:  true,
		Response: fmt.Sprintf("共有 %d 台设备：%s。", len(devices), strings.Join(lines, "、")),
	}
}

func formatDeviceStatus(dev data.Device) string {
	sb := &strings.Builder{}
	fmt.Fprintf(sb, "设备「%s」", dev.Name)
	if dev.Brand != "" {
		fmt.Fprintf(sb, "（%s）", dev.Brand)
	}
	if len(dev.Props) == 0 {
		sb.WriteString("，状态未知。")
		return sb.String()
	}
	parts := make([]string, 0, len(dev.Props))
	for k, v := range dev.Props {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	fmt.Fprintf(sb, "，当前状态：%s。", strings.Join(parts, "，"))
	return sb.String()
}
