// Package miio 提供小米 MIoT 局域网设备控制客户端实现
// 对应 Python 版 miot/miot_lan.py 中的 MIoTLan
package miio

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// LANClient 控制模式
type LANCtrlMode int

const (
	// LANCtrlModeAuto 自动模式（本地优先）
	LANCtrlModeAuto LANCtrlMode = iota
	// LANCtrlModeCloud 仅云端模式
	LANCtrlModeCloud
)

// LAN 扫描间隔常量
const (
	// LANOTProbeIntervalMin 最小探测间隔（秒）
	LANOTProbeIntervalMin = 5
	// LANOTProbeIntervalMax 最大探测间隔（秒）
	LANOTProbeIntervalMax = 45
)

// LANRequestData 表示一个待处理的请求
type LANRequestData struct {
	MsgID      int
	Handler    func(msg map[string]interface{}, ctx interface{})
	HandlerCtx interface{}
	Timeout    *time.Timer
	CreateTime time.Time
}

// LANBroadcastData 广播订阅数据
type LANBroadcastData struct {
	Key        string
	Handler    func(param map[string]interface{}, ctx interface{})
	HandlerCtx interface{}
}

// LANDeviceStateHandler 设备状态变更回调函数类型
type LANDeviceStateHandler func(did string, state map[string]interface{})

// LANClient MIoT 局域网设备控制客户端
// 对应 Python 版 MIoTLan
type LANClient struct {
	// 配置
	netIFs          []string
	enableSubscribe bool
	virtualDID      string

	// 网络
	network     NetworkMonitor
	mipsService MipsService

	// 设备管理
	devices   map[string]*LANDevice
	devicesMu sync.RWMutex

	// 请求管理
	msgIDCounter    int64
	pendingRequests map[int]*LANRequestData
	requestsMu      sync.RWMutex

	// 订阅管理
	deviceStateHandlers []LANDeviceStateHandler
	handlersMu          sync.RWMutex

	broadcastSubs map[string]*LANBroadcastData
	broadcastMu   sync.RWMutex

	// UDP 连接
	conn        *net.UDPConn
	connMu      sync.RWMutex
	localPort   int
	readBuffer  []byte
	writeBuffer []byte

	// 扫描定时器
	scanTimer    *time.Timer
	scanInterval time.Duration
	scanMu       sync.Mutex

	// 运行状态
	running   bool
	runningMu sync.RWMutex
	stopCh    chan struct{}
	wg        sync.WaitGroup

	// 内部任务队列
	internalLoop chan func()
}

// NetworkMonitor 网络监控接口
type NetworkMonitor interface {
	GetNetworkInfo() map[string]NetworkInterfaceInfo
	SubscribeNetworkInfo(key string, handler func(status InterfaceStatus, info NetworkInterfaceInfo))
	UnsubscribeNetworkInfo(key string)
}

// NetworkInterfaceInfo 网络接口信息
type NetworkInterfaceInfo struct {
	Name string
	IP   string
	IsUp bool
}

// InterfaceStatus 网络接口状态
type InterfaceStatus int

const (
	// InterfaceStatusAdd 接口添加
	InterfaceStatusAdd InterfaceStatus = iota
	// InterfaceStatusRemove 接口移除
	InterfaceStatusRemove
)

// MipsService MIPS 服务接口
type MipsService interface {
	GetServices(groupID string) map[string]MipsServiceInfo
	SubscribeServiceChange(key, groupID string, handler func(groupID string, state MipsServiceState, data map[string]interface{}))
}

// MipsServiceInfo MIPS 服务信息
type MipsServiceInfo struct {
	GroupID   string
	Addresses []string
	Port      int
}

// MipsServiceState MIPS 服务状态
type MipsServiceState int

const (
	// MipsServiceStateOnline 服务在线
	MipsServiceStateOnline MipsServiceState = iota
	// MipsServiceStateOffline 服务离线
	MipsServiceStateOffline
)

// NewLANClient 创建新的局域网控制客户端
//
// 参数:
//   - netIFs: 网络接口列表
//   - network: 网络监控器
//   - mipsService: MIPS 服务
//   - enableSubscribe: 是否启用订阅
func NewLANClient(netIFs []string, network NetworkMonitor, mipsService MipsService, enableSubscribe bool) (*LANClient, error) {
	// 生成虚拟 DID
	virtualDID := generateVirtualDID()

	client := &LANClient{
		netIFs:              netIFs,
		network:             network,
		mipsService:         mipsService,
		enableSubscribe:     enableSubscribe,
		virtualDID:          virtualDID,
		devices:             make(map[string]*LANDevice),
		pendingRequests:     make(map[int]*LANRequestData),
		deviceStateHandlers: make([]LANDeviceStateHandler, 0),
		broadcastSubs:       make(map[string]*LANBroadcastData),
		readBuffer:          make([]byte, LANOTMsgLen),
		writeBuffer:         make([]byte, LANOTMsgLen),
		scanInterval:        LANOTProbeIntervalMin * time.Second,
		stopCh:              make(chan struct{}),
		internalLoop:        make(chan func(), 100),
	}

	return client, nil
}

// generateVirtualDID 生成虚拟 DID
func generateVirtualDID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%d", binary.BigEndian.Uint64(b))
}

// GetVirtualDID 获取虚拟 DID（实现 LANManager 接口）
func (c *LANClient) GetVirtualDID() string {
	return c.virtualDID
}

// GetInternalLoop 获取内部任务队列（实现 LANManager 接口）
func (c *LANClient) GetInternalLoop() chan func() {
	return c.internalLoop
}

// Init 初始化客户端
func (c *LANClient) Init() error {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()

	if c.running {
		return nil
	}

	// 创建 UDP 连接
	addr, err := net.ResolveUDPAddr("udp", ":0")
	if err != nil {
		return fmt.Errorf("failed to resolve udp addr: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen udp: %w", err)
	}

	c.conn = conn
	c.localPort = conn.LocalAddr().(*net.UDPAddr).Port
	c.running = true

	// 启动接收 goroutine
	c.wg.Add(1)
	go c.receiveLoop()

	// 启动内部任务处理 goroutine
	c.wg.Add(1)
	go c.internalLoopHandler()

	// 启动扫描
	c.scheduleScan()

	return nil
}

// Deinit 清理客户端资源
func (c *LANClient) Deinit() {
	c.runningMu.Lock()
	if !c.running {
		c.runningMu.Unlock()
		return
	}
	c.running = false
	c.runningMu.Unlock()

	close(c.stopCh)

	// 取消扫描定时器
	c.scanMu.Lock()
	if c.scanTimer != nil {
		c.scanTimer.Stop()
	}
	c.scanMu.Unlock()

	// 清理所有请求
	c.requestsMu.Lock()
	for _, req := range c.pendingRequests {
		if req.Timeout != nil {
			req.Timeout.Stop()
		}
	}
	c.pendingRequests = make(map[int]*LANRequestData)
	c.requestsMu.Unlock()

	// 清理设备
	c.devicesMu.Lock()
	for _, device := range c.devices {
		device.OnDelete()
	}
	c.devices = make(map[string]*LANDevice)
	c.devicesMu.Unlock()

	// 关闭连接
	c.connMu.Lock()
	if c.conn != nil {
		c.conn.Close()
	}
	c.connMu.Unlock()

	// 等待 goroutine 退出
	c.wg.Wait()
}

// IsRunning 检查客户端是否运行中
func (c *LANClient) IsRunning() bool {
	c.runningMu.RLock()
	defer c.runningMu.RUnlock()
	return c.running
}

// UpdateDevices 更新设备列表
func (c *LANClient) UpdateDevices(devices map[string]map[string]interface{}) {
	c.devicesMu.Lock()
	defer c.devicesMu.Unlock()

	for did, info := range devices {
		// 检查 DID 是否为数字
		if !isNumericDID(did) {
			continue
		}

		model, _ := info["model"].(string)
		if model == "" {
			continue
		}

		// 检查设备是否已存在
		if device, exists := c.devices[did]; exists {
			device.UpdateInfo(info)
			continue
		}

		// 创建新设备
		tokenHex, ok := info["token"].(string)
		if !ok || len(tokenHex) != 32 {
			continue
		}

		ip, _ := info["local_ip"].(string)

		device, err := NewLANDevice(c, did, tokenHex, ip)
		if err != nil {
			continue
		}

		c.devices[did] = device
	}
}

// DeleteDevices 删除设备
func (c *LANClient) DeleteDevices(dids []string) {
	c.devicesMu.Lock()
	defer c.devicesMu.Unlock()

	for _, did := range dids {
		if device, exists := c.devices[did]; exists {
			device.OnDelete()
			delete(c.devices, did)
		}
	}
}

// GetDevice 获取设备
func (c *LANClient) GetDevice(did string) (*LANDevice, bool) {
	c.devicesMu.RLock()
	defer c.devicesMu.RUnlock()
	device, exists := c.devices[did]
	return device, exists
}

// SubDeviceState 订阅设备状态变更
func (c *LANClient) SubDeviceState(handler LANDeviceStateHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	c.deviceStateHandlers = append(c.deviceStateHandlers, handler)
}

// UnsubDeviceState 取消订阅设备状态变更
func (c *LANClient) UnsubDeviceState(handler LANDeviceStateHandler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()

	for i, h := range c.deviceStateHandlers {
		if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
			c.deviceStateHandlers = append(c.deviceStateHandlers[:i], c.deviceStateHandlers[i+1:]...)
			break
		}
	}
}

// BroadcastDeviceState 广播设备状态变更（实现 LANManager 接口）
func (c *LANClient) BroadcastDeviceState(did string, state map[string]interface{}) {
	c.handlersMu.RLock()
	handlers := make([]LANDeviceStateHandler, len(c.deviceStateHandlers))
	copy(handlers, c.deviceStateHandlers)
	c.handlersMu.RUnlock()

	for _, handler := range handlers {
		go handler(did, state)
	}
}

// SubProp 订阅属性变更
func (c *LANClient) SubProp(did string, handler func(param map[string]interface{}, ctx interface{}), ctx interface{}, siid, piid *int) {
	key := fmt.Sprintf("%s/p/", did)
	if siid != nil && piid != nil {
		key = fmt.Sprintf("%s/p/%d/%d", did, *siid, *piid)
	} else {
		key = fmt.Sprintf("%s/p/#", did)
	}

	c.broadcastMu.Lock()
	c.broadcastSubs[key] = &LANBroadcastData{
		Key:        key,
		Handler:    handler,
		HandlerCtx: ctx,
	}
	c.broadcastMu.Unlock()
}

// UnsubProp 取消订阅属性变更
func (c *LANClient) UnsubProp(did string, siid, piid *int) {
	key := fmt.Sprintf("%s/p/", did)
	if siid != nil && piid != nil {
		key = fmt.Sprintf("%s/p/%d/%d", did, *siid, *piid)
	} else {
		key = fmt.Sprintf("%s/p/#", did)
	}

	c.broadcastMu.Lock()
	delete(c.broadcastSubs, key)
	c.broadcastMu.Unlock()
}

// GetPropAsync 异步获取设备属性
func (c *LANClient) GetPropAsync(did string, siid, piid int, timeoutMs int) (interface{}, error) {
	if !c.IsRunning() {
		return nil, fmt.Errorf("lan client not running")
	}

	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)

	handler := func(msg map[string]interface{}, ctx interface{}) {
		if msg == nil {
			errCh <- fmt.Errorf("timeout")
			return
		}

		result, ok := msg["result"].([]interface{})
		if !ok || len(result) == 0 {
			errCh <- fmt.Errorf("invalid response")
			return
		}

		firstResult, ok := result[0].(map[string]interface{})
		if !ok {
			errCh <- fmt.Errorf("invalid response format")
			return
		}

		value, exists := firstResult["value"]
		if !exists {
			errCh <- fmt.Errorf("no value in response")
			return
		}

		resultCh <- value
	}

	msg := map[string]interface{}{
		"method": "get_properties",
		"params": []map[string]interface{}{
			{"did": did, "siid": siid, "piid": piid},
		},
	}

	c.SendToDevice(did, msg, handler, nil, timeoutMs)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return nil, fmt.Errorf("timeout")
	}
}

// SetPropAsync 异步设置设备属性
func (c *LANClient) SetPropAsync(did string, siid, piid int, value interface{}, timeoutMs int) (map[string]interface{}, error) {
	if !c.IsRunning() {
		return nil, fmt.Errorf("lan client not running")
	}

	resultCh := make(chan map[string]interface{}, 1)
	errCh := make(chan error, 1)

	handler := func(msg map[string]interface{}, ctx interface{}) {
		if msg == nil {
			errCh <- fmt.Errorf("timeout")
			return
		}

		resultCh <- msg
	}

	msg := map[string]interface{}{
		"method": "set_properties",
		"params": []map[string]interface{}{
			{"did": did, "siid": siid, "piid": piid, "value": value},
		},
	}

	c.SendToDevice(did, msg, handler, nil, timeoutMs)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return nil, fmt.Errorf("timeout")
	}
}

// ActionAsync 异步执行设备动作
func (c *LANClient) ActionAsync(did string, siid, aiid int, inList []interface{}, timeoutMs int) (map[string]interface{}, error) {
	if !c.IsRunning() {
		return nil, fmt.Errorf("lan client not running")
	}

	resultCh := make(chan map[string]interface{}, 1)
	errCh := make(chan error, 1)

	handler := func(msg map[string]interface{}, ctx interface{}) {
		if msg == nil {
			errCh <- fmt.Errorf("timeout")
			return
		}

		resultCh <- msg
	}

	msg := map[string]interface{}{
		"method": "action",
		"params": map[string]interface{}{
			"did":  did,
			"siid": siid,
			"aiid": aiid,
			"in":   inList,
		},
	}

	c.SendToDevice(did, msg, handler, nil, timeoutMs)

	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return nil, fmt.Errorf("timeout")
	}
}

// SendToDevice 发送消息到设备（实现 LANManager 接口）
func (c *LANClient) SendToDevice(did string, msg map[string]interface{}, handler func(msg map[string]interface{}, ctx interface{}), ctx interface{}, timeoutMs int) {
	c.devicesMu.RLock()
	device, exists := c.devices[did]
	c.devicesMu.RUnlock()

	if !exists {
		if handler != nil {
			handler(nil, ctx)
		}
		return
	}

	ifName := device.GetIfName()
	if ifName == "" || device.IP == "" {
		if handler != nil {
			handler(nil, ctx)
		}
		return
	}

	// 生成消息 ID
	msgID := c.genMsgID()
	msg["id"] = msgID

	// 计算时间偏移
	offset := int(time.Now().Unix()) - device.offset

	// 生成数据包
	packet, err := device.GenPacket(msg, did, offset)
	if err != nil {
		if handler != nil {
			handler(nil, ctx)
		}
		return
	}

	// 注册请求
	if handler != nil && timeoutMs > 0 {
		c.makeRequest(msgID, packet, ifName, device.IP, handler, ctx, timeoutMs)
	} else {
		// 直接发送，不等待响应
		c.sendPacket(ifName, packet, device.IP)
	}
}

// makeRequest 创建请求并等待响应
func (c *LANClient) makeRequest(msgID int, packet []byte, ifName, ip string, handler func(msg map[string]interface{}, ctx interface{}), ctx interface{}, timeoutMs int) {
	req := &LANRequestData{
		MsgID:      msgID,
		Handler:    handler,
		HandlerCtx: ctx,
		CreateTime: time.Now(),
	}

	// 设置超时
	req.Timeout = time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, func() {
		c.requestsMu.Lock()
		delete(c.pendingRequests, msgID)
		c.requestsMu.Unlock()

		if handler != nil {
			handler(nil, ctx)
		}
	})

	c.requestsMu.Lock()
	c.pendingRequests[msgID] = req
	c.requestsMu.Unlock()

	// 发送数据包
	c.sendPacket(ifName, packet, ip)
}

// sendPacket 发送数据包
func (c *LANClient) sendPacket(ifName string, packet []byte, ip string) {
	c.connMu.RLock()
	conn := c.conn
	c.connMu.RUnlock()

	if conn == nil {
		return
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", ip, LANOTPort))
	if err != nil {
		return
	}

	conn.WriteToUDP(packet, addr)
}

// Ping 发送探测消息（实现 LANManager 接口）
func (c *LANClient) Ping(ifName, targetIP string) {
	probeMsg := c.genProbeMsg()

	if ifName == "" {
		// 广播到所有接口
		c.sendPacket("", probeMsg, "255.255.255.255")
	} else {
		c.sendPacket(ifName, probeMsg, targetIP)
	}
}

// genProbeMsg 生成探测消息
func (c *LANClient) genProbeMsg() []byte {
	probe := make([]byte, LANOTProbeLen)

	// 固定头部
	copy(probe[0:4], []byte{0x21, 0x31, 0x00, 0x20})

	// 填充 0xFF
	for i := 4; i < 12; i++ {
		probe[i] = 0xFF
	}

	// MDID
	copy(probe[12:16], []byte("MDID"))

	// Virtual DID (uint64, big-endian)
	var didNum uint64
	fmt.Sscanf(c.virtualDID, "%d", &didNum)
	binary.BigEndian.PutUint64(probe[16:24], didNum)

	// 填充 0x00
	for i := 24; i < 32; i++ {
		probe[i] = 0x00
	}

	return probe
}

// genMsgID 生成消息 ID
func (c *LANClient) genMsgID() int {
	id := atomic.AddInt64(&c.msgIDCounter, 1)
	if id > 0x80000000 {
		atomic.StoreInt64(&c.msgIDCounter, 1)
		id = 1
	}
	return int(id)
}

// receiveLoop 接收循环
func (c *LANClient) receiveLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		c.connMu.RLock()
		conn := c.conn
		c.connMu.RUnlock()

		if conn == nil {
			return
		}

		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, addr, err := conn.ReadFromUDP(c.readBuffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}

		if addr.Port != LANOTPort {
			continue
		}

		go c.handleRawMessage(c.readBuffer[:n], addr.IP.String())
	}
}

// handleRawMessage 处理原始消息
func (c *LANClient) handleRawMessage(data []byte, ip string) {
	if len(data) < 2 {
		return
	}

	// 检查协议头
	header := binary.BigEndian.Uint16(data[0:2])
	if header != LANOTHeader {
		return
	}

	// 解析 DID
	if len(data) < 12 {
		return
	}
	didNum := binary.BigEndian.Uint64(data[4:12])
	did := fmt.Sprintf("%d", didNum)

	c.devicesMu.RLock()
	device, exists := c.devices[did]
	c.devicesMu.RUnlock()

	if !exists {
		return
	}

	// 解析时间戳偏移
	if len(data) >= 16 {
		timestamp := binary.BigEndian.Uint32(data[12:16])
		device.offset = int(time.Now().Unix()) - int(timestamp)
	}

	// 保活
	dataLen := binary.BigEndian.Uint16(data[2:4])
	if int(dataLen) == LANOTProbeLen || device.Subscribed {
		device.KeepAlive(ip, "")
	}

	// 处理订阅消息
	if len(data) >= 28 && c.enableSubscribe {
		if bytes.Equal(data[16:20], []byte("MSUB")) &&
			bytes.Equal(data[24:27], []byte("PUB")) {
			device.SupportedWildcardSub = data[28] == LANOTSupportWildcardSub
			subTS := binary.BigEndian.Uint32(data[20:24])
			subType := data[27]

			if device.SupportedWildcardSub && (subType == 0 || subType == 1 || subType == 4) &&
				int64(subTS) != device.SubTS {
				device.Subscribed = false
				device.Subscribe()
			}
		}
	}

	// 处理数据消息
	if dataLen > LANOTProbeLen && len(data) >= int(dataLen) {
		decrypted, err := device.DecryptPacket(data[:dataLen])
		if err != nil {
			return
		}

		c.handleMessage(did, decrypted)
	}
}

// handleMessage 处理解密后的消息
func (c *LANClient) handleMessage(did string, msg map[string]interface{}) {
	msgID, ok := msg["id"].(float64)
	if !ok {
		return
	}

	// 检查是否是响应
	c.requestsMu.Lock()
	req, exists := c.pendingRequests[int(msgID)]
	if exists {
		delete(c.pendingRequests, int(msgID))
	}
	c.requestsMu.Unlock()

	if exists {
		if req.Timeout != nil {
			req.Timeout.Stop()
		}
		if req.Handler != nil {
			go req.Handler(msg, req.HandlerCtx)
		}
		return
	}

	// 处理上行消息（属性变更/事件）
	method, _ := msg["method"].(string)
	params, _ := msg["params"].(map[string]interface{})

	if method == "" || params == nil {
		return
	}

	switch method {
	case "properties_changed":
		c.handlePropertiesChanged(did, params)
	case "event_occured":
		c.handleEventOccurred(did, params)
	}

	// 发送确认响应
	c.SendToDevice(did, map[string]interface{}{
		"id":     int(msgID),
		"result": map[string]interface{}{"code": 0},
	}, nil, nil, 0)
}

// handlePropertiesChanged 处理属性变更通知
func (c *LANClient) handlePropertiesChanged(did string, params map[string]interface{}) {
	siid, _ := params["siid"].(float64)
	piid, _ := params["piid"].(float64)

	key := fmt.Sprintf("%s/p/%d/%d", did, int(siid), int(piid))
	wildcardKey := fmt.Sprintf("%s/p/#", did)

	c.broadcastMu.RLock()
	subs := make([]*LANBroadcastData, 0)
	if sub, exists := c.broadcastSubs[key]; exists {
		subs = append(subs, sub)
	}
	if sub, exists := c.broadcastSubs[wildcardKey]; exists {
		subs = append(subs, sub)
	}
	c.broadcastMu.RUnlock()

	for _, sub := range subs {
		go sub.Handler(params, sub.HandlerCtx)
	}
}

// handleEventOccurred 处理事件通知
func (c *LANClient) handleEventOccurred(did string, params map[string]interface{}) {
	siid, _ := params["siid"].(float64)
	eiid, _ := params["eiid"].(float64)

	key := fmt.Sprintf("%s/e/%d/%d", did, int(siid), int(eiid))

	c.broadcastMu.RLock()
	sub, exists := c.broadcastSubs[key]
	c.broadcastMu.RUnlock()

	if exists {
		go sub.Handler(params, sub.HandlerCtx)
	}
}

// scheduleScan 调度设备扫描
func (c *LANClient) scheduleScan() {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()

	if c.scanTimer != nil {
		c.scanTimer.Stop()
	}

	c.scanTimer = time.AfterFunc(c.scanInterval, func() {
		c.scanDevices()
	})
}

// scanDevices 扫描设备
func (c *LANClient) scanDevices() {
	select {
	case <-c.stopCh:
		return
	default:
	}

	// 发送广播探测
	c.Ping("", "255.255.255.255")

	// 更新扫描间隔
	c.scanMu.Lock()
	c.scanInterval = minDuration(c.scanInterval*2, LANOTProbeIntervalMax*time.Second)
	c.scanMu.Unlock()

	c.scheduleScan()
}

// internalLoopHandler 内部任务队列处理
func (c *LANClient) internalLoopHandler() {
	defer c.wg.Done()

	for {
		select {
		case <-c.stopCh:
			return
		case task := <-c.internalLoop:
			if task != nil {
				task()
			}
		}
	}
}

// isNumericDID 检查 DID 是否为数字
func isNumericDID(did string) bool {
	if did == "" {
		return false
	}
	for _, ch := range did {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}
