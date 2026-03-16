// Package miio 提供小米 MIoT 局域网设备控制实现
// 对应 Python 版 miot/miot_lan.py 中的 _MIoTLanDevice
package miio

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// LANDeviceState 表示局域网设备状态
type LANDeviceState int

const (
	// LANDeviceStateFresh 设备刚被发现
	LANDeviceStateFresh LANDeviceState = iota
	// LANDeviceStatePing1 第一次心跳检测
	LANDeviceStatePing1
	// LANDeviceStatePing2 第二次心跳检测
	LANDeviceStatePing2
	// LANDeviceStatePing3 第三次心跳检测
	LANDeviceStatePing3
	// LANDeviceStateDead 设备离线
	LANDeviceStateDead
)

// LAN 协议常量
const (
	// LANOTHeader OT 协议头
	LANOTHeader uint16 = 0x2131
	// LANOTHeaderLen OT 协议头长度
	LANOTHeaderLen = 32
	// LANOTPort OT 协议端口
	LANOTPort = 54321
	// LANOTProbeLen 探测消息长度
	LANOTProbeLen = 32
	// LANOTMsgLen 最大消息长度
	LANOTMsgLen = 1400
	// LANOTSupportWildcardSub 支持通配符订阅
	LANOTSupportWildcardSub = 0xFE

	// LANNetworkUnstableCntTh 网络不稳定检测阈值
	LANNetworkUnstableCntTh = 10
	// LANNetworkUnstableTimeTh 网络不稳定时间阈值（秒）
	LANNetworkUnstableTimeTh = 120
	// LANNetworkUnstableResumeTh 网络恢复时间阈值（秒）
	LANNetworkUnstableResumeTh = 300
	// LANFastPingInterval 快速心跳间隔（秒）
	LANFastPingInterval = 5
	// LANConstructStatePending 构造状态等待时间（秒）
	LANConstructStatePending = 15
	// LANKAIntervalMin 最小保活间隔（秒）
	LANKAIntervalMin = 10
	// LANKAIntervalMax 最大保活间隔（秒）
	LANKAIntervalMax = 50
)

// onlineOfflineRecord 在线/离线历史记录
type onlineOfflineRecord struct {
	TS     int64
	Online bool
}

// LANDevice 表示一个 MIoT 局域网设备
// 对应 Python 版 _MIoTLanDevice
type LANDevice struct {
	// 设备标识
	DID   string
	Token []byte
	IP    string

	// 加密相关
	cipher cipher.Block
	iv     []byte

	// 状态管理
	offset               int
	Subscribed           bool
	SubTS                int64
	SupportedWildcardSub bool

	// 内部状态
	mu                   sync.RWMutex
	ifName               string
	subLocked            bool
	state                LANDeviceState
	online               bool
	onlineOfflineHistory []onlineOfflineRecord
	onlineOfflineTimer   *time.Timer
	kaTimer              *time.Timer
	kaInterval           time.Duration

	// 回调
	manager LANManager
}

// LANManager 定义 LAN 设备管理器接口
type LANManager interface {
	GetVirtualDID() string
	SendToDevice(did string, msg map[string]interface{}, handler func(msg map[string]interface{}, ctx interface{}), ctx interface{}, timeoutMs int)
	BroadcastDeviceState(did string, state map[string]interface{})
	Ping(ifName string, targetIP string)
	GetInternalLoop() chan func()
}

// NewLANDevice 创建新的局域网设备实例
//
// 参数:
//   - manager: LAN 管理器
//   - did: 设备 ID
//   - tokenHex: 设备 token（32字符十六进制字符串）
//   - ip: 设备 IP 地址（可选）
func NewLANDevice(manager LANManager, did, tokenHex, ip string) (*LANDevice, error) {
	token, err := hex.DecodeString(tokenHex)
	if err != nil {
		return nil, fmt.Errorf("invalid token hex: %w", err)
	}
	if len(token) != 16 {
		return nil, fmt.Errorf("invalid token length: expected 16 bytes, got %d", len(token))
	}

	// 派生 AES 密钥和 IV
	aesKey := md5Hash(token)
	aesIV := md5Hash(append(aesKey, token...))

	// 创建 AES 加密器
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	device := &LANDevice{
		DID:                  did,
		Token:                token,
		IP:                   ip,
		cipher:               block,
		iv:                   aesIV,
		offset:               0,
		Subscribed:           false,
		SubTS:                0,
		SupportedWildcardSub: false,
		ifName:               "",
		subLocked:            false,
		state:                LANDeviceStateDead,
		online:               false,
		onlineOfflineHistory: make([]onlineOfflineRecord, 0, LANNetworkUnstableCntTh),
		kaInterval:           LANKAIntervalMin * time.Second,
		manager:              manager,
	}

	// 启动保活定时器
	device.scheduleKeepAliveInit()

	return device, nil
}

// md5Hash 计算 MD5 哈希
func md5Hash(data []byte) []byte {
	hash := md5.Sum(data)
	return hash[:]
}

// scheduleKeepAliveInit 调度初始保活
func (d *LANDevice) scheduleKeepAliveInit() {
	delay := randomizeDuration(LANConstructStatePending*time.Second, 0.5)
	d.kaTimer = time.AfterFunc(delay, func() {
		d.mu.Lock()
		d.kaInterval = LANKAIntervalMin * time.Second
		d.mu.Unlock()
		d.updateKeepAlive(LANDeviceStateDead)
	})
}

// KeepAlive 更新设备保活状态
func (d *LANDevice) KeepAlive(ip, ifName string) {
	d.mu.Lock()
	d.IP = ip
	if d.ifName != ifName {
		d.ifName = ifName
	}
	d.mu.Unlock()
	d.updateKeepAlive(LANDeviceStateFresh)
}

// GetIfName 获取网络接口名称
func (d *LANDevice) GetIfName() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.ifName
}

// IsOnline 获取设备在线状态
func (d *LANDevice) IsOnline() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.online
}

// setOnline 设置设备在线状态（内部使用）
func (d *LANDevice) setOnline(online bool) {
	d.mu.Lock()
	if d.online == online {
		d.mu.Unlock()
		return
	}
	d.online = online
	d.mu.Unlock()

	d.manager.BroadcastDeviceState(d.DID, map[string]interface{}{
		"online":         online,
		"push_available": d.Subscribed,
	})
}

// updateKeepAlive 更新保活状态机
func (d *LANDevice) updateKeepAlive(state LANDeviceState) {
	d.mu.Lock()
	lastState := d.state
	d.state = state

	// 取消现有定时器
	if d.kaTimer != nil {
		d.kaTimer.Stop()
		d.kaTimer = nil
	}
	d.mu.Unlock()

	switch state {
	case LANDeviceStateFresh:
		if lastState == LANDeviceStateDead {
			d.mu.Lock()
			d.kaInterval = LANKAIntervalMin * time.Second
			d.mu.Unlock()
			d.changeOnline(true)
		}
		d.scheduleNextKeepAlive()

	case LANDeviceStatePing1, LANDeviceStatePing2, LANDeviceStatePing3:
		// 快速心跳检测
		d.mu.RLock()
		ifName := d.ifName
		ip := d.IP
		d.mu.RUnlock()

		if ifName == "" {
			return
		}
		if ip == "" {
			return
		}
		d.manager.Ping(ifName, ip)

		// 调度下一次状态
		nextState := state + 1
		d.kaTimer = time.AfterFunc(LANFastPingInterval*time.Second, func() {
			d.updateKeepAlive(nextState)
		})

	case LANDeviceStateDead:
		if lastState == LANDeviceStatePing3 {
			d.mu.Lock()
			d.kaInterval = LANKAIntervalMin * time.Second
			d.mu.Unlock()
			d.changeOnline(false)
		}
	}
}

// scheduleNextKeepAlive 调度下一次保活
func (d *LANDevice) scheduleNextKeepAlive() {
	d.mu.Lock()
	d.kaInterval = minDuration(d.kaInterval*2, LANKAIntervalMax*time.Second)
	delay := randomizeDuration(d.kaInterval, 0.1)
	d.mu.Unlock()

	d.kaTimer = time.AfterFunc(delay, func() {
		d.updateKeepAlive(LANDeviceStatePing1)
	})
}

// changeOnline 改变在线状态（带网络不稳定检测）
func (d *LANDevice) changeOnline(online bool) {
	tsNow := time.Now().Unix()

	d.mu.Lock()
	d.onlineOfflineHistory = append(d.onlineOfflineHistory, onlineOfflineRecord{
		TS:     tsNow,
		Online: online,
	})
	if len(d.onlineOfflineHistory) > LANNetworkUnstableCntTh {
		d.onlineOfflineHistory = d.onlineOfflineHistory[1:]
	}

	if d.onlineOfflineTimer != nil {
		d.onlineOfflineTimer.Stop()
		d.onlineOfflineTimer = nil
	}
	d.mu.Unlock()

	if !online {
		d.setOnline(false)
		return
	}

	d.mu.RLock()
	historyLen := len(d.onlineOfflineHistory)
	var firstTS int64
	if historyLen > 0 {
		firstTS = d.onlineOfflineHistory[0].TS
	}
	d.mu.RUnlock()

	if historyLen < LANNetworkUnstableCntTh || (tsNow-firstTS) > LANNetworkUnstableTimeTh {
		d.setOnline(true)
	} else {
		// 网络不稳定，延迟恢复
		d.onlineOfflineTimer = time.AfterFunc(LANNetworkUnstableResumeTh*time.Second, func() {
			d.setOnline(true)
		})
	}
}

// GenPacket 生成加密数据包
//
// 参数:
//   - clearData: 明文 JSON 数据
//   - did: 设备 ID
//   - offset: 时间偏移
//
// 返回: (加密后的数据包, 错误)
func (d *LANDevice) GenPacket(clearData map[string]interface{}, did string, offset int) ([]byte, error) {
	// 序列化 JSON
	clearBytes, err := json.Marshal(clearData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal json: %w", err)
	}

	// PKCS7 填充
	paddedData := pkcs7Pad(clearBytes, aes.BlockSize)

	// 加密
	encryptedData := make([]byte, len(paddedData))
	mode := cipher.NewCBCEncrypter(d.cipher, d.iv)
	mode.CryptBlocks(encryptedData, paddedData)

	// 构建数据包
	dataLen := len(encryptedData) + LANOTHeaderLen
	if dataLen > LANOTMsgLen {
		return nil, fmt.Errorf("rpc too long: %d > %d", dataLen, LANOTMsgLen)
	}

	packet := make([]byte, dataLen)

	// 写入头部 (big-endian)
	binary.BigEndian.PutUint16(packet[0:2], LANOTHeader)
	binary.BigEndian.PutUint16(packet[2:4], uint16(dataLen))

	// DID (uint64)
	didNum := parseDID(did)
	binary.BigEndian.PutUint64(packet[4:12], didNum)

	// Offset (uint32)
	binary.BigEndian.PutUint32(packet[12:16], uint32(offset))

	// Token (16 bytes)
	copy(packet[16:32], d.Token)

	// 加密数据
	copy(packet[32:], encryptedData)

	// 计算 MD5 (覆盖 packet[16:32])
	msgMD5 := md5Hash(packet[0:dataLen])
	copy(packet[16:32], msgMD5)

	return packet, nil
}

// DecryptPacket 解密数据包
//
// 参数:
//   - encryptedData: 加密的数据包
//
// 返回: (解密后的 JSON 数据, 错误)
func (d *LANDevice) DecryptPacket(encryptedData []byte) (map[string]interface{}, error) {
	if len(encryptedData) < LANOTHeaderLen {
		return nil, fmt.Errorf("packet too short")
	}

	// 解析数据长度
	dataLen := binary.BigEndian.Uint16(encryptedData[2:4])
	if int(dataLen) > len(encryptedData) {
		return nil, fmt.Errorf("invalid data length")
	}

	// 验证 MD5
	md5Orig := make([]byte, 16)
	copy(md5Orig, encryptedData[16:32])

	// 临时替换为 token 计算 MD5
	tempPacket := make([]byte, dataLen)
	copy(tempPacket, encryptedData[0:dataLen])
	copy(tempPacket[16:32], d.Token)

	md5Calc := md5Hash(tempPacket[0:dataLen])
	if !bytes.Equal(md5Orig, md5Calc) {
		return nil, fmt.Errorf("invalid md5")
	}

	// 解密
	encryptedPayload := encryptedData[32:dataLen]
	decryptedData := make([]byte, len(encryptedPayload))
	mode := cipher.NewCBCDecrypter(d.cipher, d.iv)
	mode.CryptBlocks(decryptedData, encryptedPayload)

	// PKCS7 去填充
	decryptedData, err := pkcs7Unpad(decryptedData, aes.BlockSize)
	if err != nil {
		return nil, fmt.Errorf("failed to unpad: %w", err)
	}

	// 去除末尾的 \0
	decryptedData = bytes.TrimRight(decryptedData, "\x00")

	// 解析 JSON
	var result map[string]interface{}
	if err := json.Unmarshal(decryptedData, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal json: %w", err)
	}

	return result, nil
}

// Subscribe 订阅设备状态推送
func (d *LANDevice) Subscribe() {
	d.mu.Lock()
	if d.subLocked {
		d.mu.Unlock()
		return
	}
	d.subLocked = true
	d.mu.Unlock()

	defer func() {
		d.mu.Lock()
		d.subLocked = false
		d.mu.Unlock()
	}()

	subTS := time.Now().Unix()
	d.manager.SendToDevice(d.DID, map[string]interface{}{
		"method": "miIO.sub",
		"params": map[string]interface{}{
			"version":    "2.0",
			"did":        d.manager.GetVirtualDID(),
			"update_ts":  subTS,
			"sub_method": ".",
		},
	}, d.handleSubscribeResponse, subTS, 5000)
}

// handleSubscribeResponse 处理订阅响应
func (d *LANDevice) handleSubscribeResponse(msg map[string]interface{}, ctx interface{}) {
	subTS, _ := ctx.(int64)

	result, ok := msg["result"].(map[string]interface{})
	if !ok {
		return
	}
	code, _ := result["code"].(float64)
	if code != 0 {
		return
	}

	d.Subscribed = true
	d.SubTS = subTS

	d.manager.BroadcastDeviceState(d.DID, map[string]interface{}{
		"online":         d.IsOnline(),
		"push_available": d.Subscribed,
	})
}

// Unsubscribe 取消订阅设备状态推送
func (d *LANDevice) Unsubscribe() {
	if !d.Subscribed {
		return
	}

	d.manager.SendToDevice(d.DID, map[string]interface{}{
		"method": "miIO.unsub",
		"params": map[string]interface{}{
			"version":    "2.0",
			"did":        d.manager.GetVirtualDID(),
			"update_ts":  d.SubTS,
			"sub_method": ".",
		},
	}, d.handleUnsubscribeResponse, nil, 5000)

	d.Subscribed = false
	d.manager.BroadcastDeviceState(d.DID, map[string]interface{}{
		"online":         d.IsOnline(),
		"push_available": d.Subscribed,
	})
}

// handleUnsubscribeResponse 处理取消订阅响应
func (d *LANDevice) handleUnsubscribeResponse(msg map[string]interface{}, ctx interface{}) {
	// 日志记录或处理响应
}

// OnDelete 设备被删除时的清理
func (d *LANDevice) OnDelete() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.kaTimer != nil {
		d.kaTimer.Stop()
		d.kaTimer = nil
	}
	if d.onlineOfflineTimer != nil {
		d.onlineOfflineTimer.Stop()
		d.onlineOfflineTimer = nil
	}
}

// UpdateInfo 更新设备信息
func (d *LANDevice) UpdateInfo(info map[string]interface{}) {
	tokenHex, ok := info["token"].(string)
	if !ok || len(tokenHex) != 32 {
		return
	}

	token, err := hex.DecodeString(tokenHex)
	if err != nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if string(token) == string(d.Token) {
		return
	}

	// 更新 token 和密钥
	d.Token = token
	aesKey := md5Hash(d.Token)
	d.iv = md5Hash(append(aesKey, d.Token...))

	newCipher, err := aes.NewCipher(aesKey)
	if err != nil {
		return
	}
	d.cipher = newCipher
}

// pkcs7Pad PKCS7 填充
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// pkcs7Unpad PKCS7 去填充
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid data length")
	}
	padding := int(data[len(data)-1])
	if padding > blockSize || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	for i := 0; i < padding; i++ {
		if data[len(data)-1-i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

// parseDID 解析 DID 字符串为 uint64
func parseDID(did string) uint64 {
	var num uint64
	fmt.Sscanf(did, "%d", &num)
	return num
}

// randomizeDuration 随机化持续时间
func randomizeDuration(base time.Duration, factor float64) time.Duration {
	// 简化实现，实际应使用随机数
	return base
}

// minDuration 返回较小的 duration
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
