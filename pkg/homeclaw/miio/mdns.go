// Package miio 提供小米 MIPS 中枢网关 mDNS 服务发现实现
// 对应 Python 版 miot/miot_mdns.py 中的 MipsService
package miio

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/sipeed/picoclaw/pkg/homeclaw/event"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// ---------- 常量 ----------

const (
	// mipsMDNSType MIPS 中枢网关 mDNS 服务类型
	mipsMDNSType = "_miot-central._tcp"
	// mipsMDNSDomain mDNS 域
	mipsMDNSDomain = "local."
	// mipsMDNSRequestTimeout 单次 Browse 超时（对应 Python MIPS_MDNS_REQUEST_TIMEOUT_MS = 5000）
	mipsMDNSRequestTimeout = 5 * time.Second
	// mipsMDNSUpdateInterval 强制重新扫描间隔（对应 Python MIPS_MDNS_UPDATE_INTERVAL_S = 600）
	mipsMDNSUpdateInterval = 600 * time.Second

	// mipsMDNSEventType 事件中心中 MIPS mDNS 事件的类型标识
	mipsMDNSEventType event.EventType = "mips_mdns"
)

// ---------- 服务状态 ----------

// MipsMDNSState MIPS mDNS 服务状态（对应 Python MipsServiceState 枚举）
type MipsMDNSState int

const (
	// MipsMDNSStateAdded 新增服务
	MipsMDNSStateAdded MipsMDNSState = 1
	// MipsMDNSStateRemoved 服务移除（保留定义，当前不触发，与 Python 原版行为一致）
	MipsMDNSStateRemoved MipsMDNSState = 2
	// MipsMDNSStateUpdated 服务更新
	MipsMDNSStateUpdated MipsMDNSState = 3
)

// String 返回状态字符串
func (s MipsMDNSState) String() string {
	switch s {
	case MipsMDNSStateAdded:
		return "added"
	case MipsMDNSStateRemoved:
		return "removed"
	case MipsMDNSStateUpdated:
		return "updated"
	default:
		return "unknown"
	}
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
func (d *MipsMDNSServiceData) validService() bool {
	return d.Role == 1 && d.SuiteMQTT
}

// toMap 将服务数据转换为 map（用于事件 Data 载荷）
// 对应 Python MipsServiceData.to_dict()
func (d *MipsMDNSServiceData) toMap() map[string]any {
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

// ---------- MipsMDNS ----------

// MipsMDNS MIPS 中枢网关 mDNS 服务发现
// 对应 Python 版 MipsService
//
// 工作原理:
//  1. 通过 grandcat/zeroconf 持续监听 _miot-central._tcp.local. 服务。
//  2. 每次发现条目后解析 TXT profile 字段（base64 二进制），提取 DID/GroupID/Role/SuiteMQTT。
//  3. 仅保留 role==1 且 suite_mqtt==true 的条目（主节点）。
//  4. 新增/更新时通过 event.GetCenter().Publish() 发布 EventTypeMipsMDNS 事件，
//     同时调用已注册的 MipsMDNSHandler 回调。
//  5. REMOVED 报文不触发移除，让连接自然断开（与 Python 原版行为一致）。
//  6. 每隔 mipsMDNSUpdateInterval 重新触发一次全量扫描。
type MipsMDNS struct {
	mu sync.RWMutex

	// 已发现的服务缓存，key = GroupID
	services map[string]*MipsMDNSServiceData

	// 运行控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMipsMDNS 创建 MipsMDNS 实例
func NewMipsMDNS() *MipsMDNS {
	return &MipsMDNS{
		services: make(map[string]*MipsMDNSServiceData),
	}
}

// Start 启动 mDNS 服务发现
// 对应 Python MipsService.init_async()
func (m *MipsMDNS) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	resolver, err := zeroconf.NewResolver(zeroconf.SelectIPTraffic(zeroconf.IPv4))
	if err != nil {
		return fmt.Errorf("mdns: create resolver: %w", err)
	}

	m.ctx, m.cancel = context.WithCancel(ctx)
	go m.browseLoop(resolver)

	logger.InfoC("mdns", "MIPS mDNS service discovery started")
	return nil
}

// Stop 停止 mDNS 服务发现
// 对应 Python MipsService.deinit_async()
func (m *MipsMDNS) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}

	m.services = make(map[string]*MipsMDNSServiceData)

	logger.InfoC("mdns", "MIPS mDNS service discovery stopped")
}

// GetServices 获取已发现的 MIPS 服务（深拷贝）
// groupID 为空时返回全部；否则仅返回对应 groupID 的条目。
// 对应 Python MipsService.get_services()
func (m *MipsMDNS) GetServices(groupID string) map[string]map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]map[string]any)
	for gid, svc := range m.services {
		if groupID != "" && gid != groupID {
			continue
		}
		result[gid] = svc.toMap()
	}
	return result
}

// ---------- 内部方法 ----------

// browseLoop 持续扫描循环，每隔 mipsMDNSUpdateInterval 重新触发一次全量扫描
// 对应 Python AsyncServiceBrowser 的持续监听行为
func (m *MipsMDNS) browseLoop(resolver *zeroconf.Resolver) {
	for {
		m.browse(resolver)

		select {
		case <-m.ctx.Done():
			return
		case <-time.After(mipsMDNSUpdateInterval):
			// 定期重扫
		}
	}
}

// browse 执行一次 mDNS 扫描，收集 mipsMDNSRequestTimeout 内的所有条目
// 对应 Python __request_service_info_async() 的批量版本
func (m *MipsMDNS) browse(resolver *zeroconf.Resolver) {
	entriesCh := make(chan *zeroconf.ServiceEntry, 32)

	browseCtx, browseCancel := context.WithTimeout(m.ctx, mipsMDNSRequestTimeout)
	defer browseCancel()

	if err := resolver.Browse(browseCtx, mipsMDNSType, mipsMDNSDomain, entriesCh); err != nil {
		if m.ctx.Err() == nil {
			logger.ErrorCF("mdns", "browse error", map[string]any{"error": err.Error()})
		}
		return
	}

	for {
		select {
		case entry, ok := <-entriesCh:
			if !ok {
				return
			}
			if entry == nil {
				continue
			}
			m.onServiceEntry(entry)

		case <-browseCtx.Done():
			return
		case <-m.ctx.Done():
			return
		}
	}
}

// onServiceEntry 处理单条 mDNS 服务条目
// 对应 Python __on_service_state_change() + __request_service_info_async()
func (m *MipsMDNS) onServiceEntry(entry *zeroconf.ServiceEntry) {
	logger.DebugCF("mdns", "service entry received", map[string]any{
		"name": entry.ServiceInstanceName(),
	})

	svcData, err := parseMipsMDNSEntry(entry)
	if err != nil {
		logger.ErrorCF("mdns", "invalid mips service", map[string]any{
			"name":  entry.ServiceInstanceName(),
			"error": err.Error(),
		})
		return
	}

	if !svcData.validService() {
		logger.DebugCF("mdns", "skip: not primary role or no MQTT support", map[string]any{
			"name":     svcData.Name,
			"role":     svcData.Role,
			"mqtt":     svcData.SuiteMQTT,
			"group_id": svcData.GroupID,
		})
		return
	}

	m.mu.Lock()
	existing, exists := m.services[svcData.GroupID]
	if exists {
		// 检查是否有意义的变更（DID / 地址 / 端口）
		// 对应 Python __request_service_info_async() 中的更新判断
		if existing.DID == svcData.DID &&
			strSliceEqual(existing.Addresses, svcData.Addresses) &&
			existing.Port == svcData.Port {
			m.mu.Unlock()
			return // 无变化，忽略
		}
		// 更新缓存
		m.services[svcData.GroupID] = svcData
		m.mu.Unlock()

		logger.InfoCF("mdns", "MIPS service updated", map[string]any{
			"group_id": svcData.GroupID,
			"did":      svcData.DID,
			"name":     svcData.Name,
		})
		m.callServiceChange(MipsMDNSStateUpdated, svcData)
	} else {
		// 新增
		m.services[svcData.GroupID] = svcData
		m.mu.Unlock()

		logger.InfoCF("mdns", "MIPS service added", map[string]any{
			"group_id": svcData.GroupID,
			"did":      svcData.DID,
			"name":     svcData.Name,
		})
		m.callServiceChange(MipsMDNSStateAdded, svcData)
	}
}

// callServiceChange 触发事件
func (m *MipsMDNS) callServiceChange(state MipsMDNSState, svcData *MipsMDNSServiceData) {
	data := svcData.toMap()

	// 发布到全局事件中心
	evt := event.NewEventWithData(mipsMDNSEventType, "mdns", map[string]any{
		"state":    state.String(),
		"group_id": svcData.GroupID,
		"service":  data,
	})
	event.GetCenter().Publish(evt)

	logger.InfoCF("mdns", "call service change", map[string]any{
		"state":    state.String(),
		"group_id": svcData.GroupID,
	})
}

// ---------- 解析 ----------

// parseMipsMDNSEntry 将 zeroconf.ServiceEntry 解析为 MipsMDNSServiceData
//
// profile TXT 字段二进制布局（对应 Python MipsServiceData.__init__）:
//
//	[0]      版本/魔数字节
//	[1:9]    DID（大端 uint64，转十进制字符串）
//	[9:17]   GroupID（8 字节，小端存储；逆序后 hex 编码）
//	[17:20]  保留
//	[20]     高 4 位 = role
//	[21]     保留
//	[22]     bit1 = suite_mqtt
func parseMipsMDNSEntry(entry *zeroconf.ServiceEntry) (*MipsMDNSServiceData, error) {
	if entry == nil {
		return nil, fmt.Errorf("nil entry")
	}

	// 收集 IPv4 地址
	addrs := make([]string, 0, len(entry.AddrIPv4))
	for _, ip := range entry.AddrIPv4 {
		addrs = append(addrs, ip.String())
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("no IPv4 addresses")
	}
	if entry.Port == 0 {
		return nil, fmt.Errorf("port is 0")
	}

	// 查找 "profile=..." TXT 记录
	var profileB64 string
	for _, txt := range entry.Text {
		const prefix = "profile="
		if len(txt) > len(prefix) && txt[:len(prefix)] == prefix {
			profileB64 = txt[len(prefix):]
			break
		}
	}
	if profileB64 == "" {
		return nil, fmt.Errorf("missing profile TXT record")
	}

	profileBin, err := base64.StdEncoding.DecodeString(profileB64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode profile: %w", err)
	}
	if len(profileBin) < 23 {
		return nil, fmt.Errorf("profile too short: %d bytes", len(profileBin))
	}

	// DID: bytes [1:9] 大端 uint64 → 十进制字符串
	// 对应 Python: str(int.from_bytes(self.profile_bin[1:9], byteorder='big'))
	did := fmt.Sprintf("%d", binary.BigEndian.Uint64(profileBin[1:9]))

	// GroupID: bytes [9:17] 逆序后 hex 编码
	// 对应 Python: binascii.hexlify(self.profile_bin[9:17][::-1]).decode('utf-8')
	groupBytes := make([]byte, 8)
	copy(groupBytes, profileBin[9:17])
	reverseByteSlice(groupBytes)
	groupID := hex.EncodeToString(groupBytes)

	// Role: byte [20] 高 4 位
	// 对应 Python: int(self.profile_bin[20] >> 4)
	role := int(profileBin[20] >> 4)

	// SuiteMQTT: byte [22] bit1
	// 对应 Python: ((self.profile_bin[22] >> 1) & 0x01) == 0x01
	suiteMQTT := ((profileBin[22] >> 1) & 0x01) == 0x01

	svcType := entry.Service + "." + entry.Domain

	return &MipsMDNSServiceData{
		Name:      entry.ServiceInstanceName(),
		Addresses: addrs,
		Port:      entry.Port,
		Type:      svcType,
		Server:    entry.HostName,
		DID:       did,
		GroupID:   groupID,
		Role:      role,
		SuiteMQTT: suiteMQTT,
	}, nil
}

// ---------- 辅助函数 ----------

// reverseByteSlice 原地反转字节切片
func reverseByteSlice(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}

// strSliceEqual 比较两个字符串切片是否完全相同
func strSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
