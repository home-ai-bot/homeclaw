package event

import "time"

// EventType represents the type of event
type EventType string

// Event type constants
const (
	EventTypeDevice    EventType = "device"
	EventTypeRoom      EventType = "room"
	EventTypeProp      EventType = "prop"
	EventTypeDeviceMsg EventType = "device_msg"
	EventTypeToken     EventType = "token"
	EventTypeNet       EventType = "net"
)

// ==================== Event Data Types ====================

// TokenData represents token update event payload
type TokenData struct {
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token,omitempty"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
}

// NetData represents network status event payload
type NetData struct {
	Kind    string `json:"kind"` // "status" or "interface"
	Online  bool   `json:"online,omitempty"`
	Status  int    `json:"status,omitempty"`
	Name    string `json:"name,omitempty"`
	IP      string `json:"ip,omitempty"`
	Netmask string `json:"netmask,omitempty"`
	NetSeg  string `json:"netseg,omitempty"`
}

// MDNSData represents mDNS discovery event payload
type MDNSData struct {
	State   string              `json:"state"`
	GroupID string              `json:"group_id"`
	Service MipsMDNSServiceData `json:"service"`
}

// ---------- 服务数据 ----------

// MipsMDNSServiceData 从 mDNS 解析的 MIPS 网关服务数据
// 对应 Python 版 MipsServiceData
type MipsMDNSServiceData struct {
	// mDNS 原始字段
	Name      string   // 服务实例名
	Addresses []string // IPv4 地址列表
	Port      int      // 端口
	Type      string   // 服务类型（如 _miot-central._tcp.local.）
	Server    string   // 主机名

	// 从 profile TXT 字段解析
	DID       string // 设备 DID（十进制字符串）
	GroupID   string // 家庭组 ID（十六进制字符串）
	Role      int    // 角色（1 = 主节点）
	SuiteMQTT bool   // 是否支持 MQTT 连接
}

// validService 返回该服务是否为可用的主节点网关
// 对应 Python MipsServiceData.valid_service()
func (d *MipsMDNSServiceData) ValidService() bool {
	return d.Role == 1 && d.SuiteMQTT
}

// toMap 将服务数据转换为 map（用于事件 Data 载荷）
// 对应 Python MipsServiceData.to_dict()
func (d *MipsMDNSServiceData) ToMap() map[string]any {
	return map[string]any{
		"name":       d.Name,
		"addresses":  d.Addresses,
		"port":       d.Port,
		"type":       d.Type,
		"server":     d.Server,
		"did":        d.DID,
		"group_id":   d.GroupID,
		"role":       d.Role,
		"suite_mqtt": d.SuiteMQTT,
	}
}

// ==================== Event Structure ====================

// Event represents a unified event structure
type Event struct {
	Type      EventType // Event type enum
	Source    string    // Who published the event
	Timestamp time.Time // Event timestamp
	Data      any       // Payload: Device, Space, TokenData, NetData, etc.
}

// NewEvent creates a new Event with the given type, source, and data payload
func NewEvent(eventType EventType, source string, data any) Event {
	return Event{
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// IsType checks if the event is of the given type
func (e *Event) IsType(eventType EventType) bool {
	return e.Type == eventType
}

// ==================== Type-Safe Accessors ====================

// TokenData returns the payload as *TokenData, or nil if type mismatch
func (e *Event) TokenData() *TokenData {
	if d, ok := e.Data.(*TokenData); ok {
		return d
	}
	return nil
}

// NetData returns the payload as *NetData, or nil if type mismatch
func (e *Event) NetData() *NetData {
	if d, ok := e.Data.(*NetData); ok {
		return d
	}
	return nil
}

// MDNSData returns the payload as *MDNSData, or nil if type mismatch
func (e *Event) MDNSData() *MDNSData {
	if d, ok := e.Data.(*MDNSData); ok {
		return d
	}
	return nil
}

// MapData returns the payload as map[string]any for backward compatibility
func (e *Event) MapData() map[string]any {
	if d, ok := e.Data.(map[string]any); ok {
		return d
	}
	return nil
}
