// Package miio 提供小米 MIoT MIPS 本地网关客户端实现
// 对应 Python 版 miot/miot_mips.py 中的 MipsLocalClient
package miio

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MipsLocalClient MIPS 本地网关客户端
type MipsLocalClient struct {
	*MipsClient

	// 本地网关特定配置
	did                string
	groupID            string
	homeName           string
	mipsSeedID         uint32
	replyTopic         string
	devListChangeTopic string

	// 请求管理
	requestMap   map[string]*MipsRequest
	requestMapMu sync.RWMutex
	msgIDCounter uint32

	// 广播订阅
	msgMatcher   map[string]*MipsBroadcast
	msgMatcherMu sync.RWMutex

	// 属性获取队列
	getPropQueue map[string][]*getPropItem
	getPropMu    sync.Mutex
	getPropTimer *time.Timer

	// 设备列表变更回调
	onDevListChanged func(client *MipsLocalClient, devList []string)
}

// getPropItem 属性获取队列项
type getPropItem struct {
	Param     string
	Future    chan interface{}
	TimeoutMs int
}

// NewMipsLocalClient 创建 MIPS 本地网关客户端
//
// 参数:
//   - did: 虚拟设备 ID
//   - host: 网关主机地址
//   - groupID: 家庭组 ID
//   - caFile: CA 证书文件路径
//   - certFile: 客户端证书文件路径
//   - keyFile: 客户端密钥文件路径
//   - port: MQTT 端口（默认 8883）
//   - homeName: 家庭名称
func NewMipsLocalClient(did, host, groupID, caFile, certFile, keyFile string, port int, homeName string) (*MipsLocalClient, error) {
	if port == 0 {
		port = 8883
	}

	config := &MipsClientConfig{
		ClientID: did,
		Host:     host,
		Port:     port,
		CAFile:   caFile,
		CertFile: certFile,
		KeyFile:  keyFile,
	}

	baseClient, err := NewMipsClient(config)
	if err != nil {
		return nil, err
	}

	client := &MipsLocalClient{
		MipsClient:         baseClient,
		did:                did,
		groupID:            groupID,
		homeName:           homeName,
		mipsSeedID:         rand.Uint32(),
		replyTopic:         fmt.Sprintf("%s/reply", did),
		devListChangeTopic: fmt.Sprintf("%s/appMsg/devListChange", did),
		requestMap:         make(map[string]*MipsRequest),
		msgMatcher:         make(map[string]*MipsBroadcast),
		getPropQueue:       make(map[string][]*getPropItem),
		msgIDCounter:       0,
	}

	return client, nil
}

// GetGroupID 获取家庭组 ID
func (c *MipsLocalClient) GetGroupID() string {
	return c.groupID
}

// logDebug 记录调试日志（带家庭名前缀）
func (c *MipsLocalClient) logDebug(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Debug(fmt.Sprintf("[%s] ", c.homeName)+msg, args...)
	}
}

// logInfo 记录信息日志（带家庭名前缀）
func (c *MipsLocalClient) logInfo(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Info(fmt.Sprintf("[%s] ", c.homeName)+msg, args...)
	}
}

// logError 记录错误日志（带家庭名前缀）
func (c *MipsLocalClient) logError(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Error(fmt.Sprintf("[%s] ", c.homeName)+msg, args...)
	}
}

// Connect 连接到本地网关
func (c *MipsLocalClient) Connect() error {
	return c.MipsClient.Connect()
}

// Disconnect 断开连接
func (c *MipsLocalClient) Disconnect() error {
	c.requestMapMu.Lock()
	c.requestMap = make(map[string]*MipsRequest)
	c.requestMapMu.Unlock()

	c.msgMatcherMu.Lock()
	c.msgMatcher = make(map[string]*MipsBroadcast)
	c.msgMatcherMu.Unlock()

	return c.MipsClient.Disconnect()
}

// SetOnDevListChanged 设置设备列表变更回调
func (c *MipsLocalClient) SetOnDevListChanged(handler func(client *MipsLocalClient, devList []string)) {
	c.onDevListChanged = handler
}

// genMipsID 生成 MIPS 消息 ID
func (c *MipsLocalClient) genMipsID() int {
	id := atomic.AddUint32(&c.mipsSeedID, 1)
	if id >= 0xFFFFFFFF {
		atomic.StoreUint32(&c.mipsSeedID, 1)
		id = 1
	}
	return int(id)
}

// SubProp 订阅设备属性变更
func (c *MipsLocalClient) SubProp(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, piid *int, ctx interface{}) error {
	topic := fmt.Sprintf("appMsg/notify/iot/%s/property/", did)
	if siid != nil && piid != nil {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/property/%d.%d", did, *siid, *piid)
	} else {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/property/#", did)
	}

	onPropMsg := func(msgTopic string, payload string, msgCtx interface{}) {
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			c.logInfo("unknown prop msg: %s", payload)
			return
		}

		if msg["did"] == nil || msg["siid"] == nil || msg["piid"] == nil || msg["value"] == nil {
			c.logInfo("unknown prop msg: %s", payload)
			return
		}

		if handler != nil {
			c.logDebug("local, on properties_changed: %s", payload)
			handler(msg, msgCtx)
		}
	}

	return c.regBroadcastExternal(topic, onPropMsg, ctx)
}

// UnsubProp 取消订阅设备属性变更
func (c *MipsLocalClient) UnsubProp(did string, siid, piid *int) error {
	topic := fmt.Sprintf("appMsg/notify/iot/%s/property/", did)
	if siid != nil && piid != nil {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/property/%d.%d", did, *siid, *piid)
	} else {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/property/#", did)
	}

	return c.unregBroadcastExternal(topic)
}

// SubEvent 订阅设备事件
func (c *MipsLocalClient) SubEvent(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, eiid *int, ctx interface{}) error {
	topic := fmt.Sprintf("appMsg/notify/iot/%s/event/", did)
	if siid != nil && eiid != nil {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/event/%d.%d", did, *siid, *eiid)
	} else {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/event/#", did)
	}

	onEventMsg := func(msgTopic string, payload string, msgCtx interface{}) {
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			c.logInfo("unknown event msg: %s", payload)
			return
		}

		if msg["did"] == nil || msg["siid"] == nil || msg["eiid"] == nil {
			c.logInfo("unknown event msg: %s", payload)
			return
		}

		if msg["arguments"] == nil {
			c.logInfo("wrong event msg: %s", payload)
			msg["arguments"] = []interface{}{}
		}

		if handler != nil {
			c.logDebug("local, on event_occurred: %s", payload)
			handler(msg, msgCtx)
		}
	}

	return c.regBroadcastExternal(topic, onEventMsg, ctx)
}

// UnsubEvent 取消订阅设备事件
func (c *MipsLocalClient) UnsubEvent(did string, siid, eiid *int) error {
	topic := fmt.Sprintf("appMsg/notify/iot/%s/event/", did)
	if siid != nil && eiid != nil {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/event/%d.%d", did, *siid, *eiid)
	} else {
		topic = fmt.Sprintf("appMsg/notify/iot/%s/event/#", did)
	}

	return c.unregBroadcastExternal(topic)
}

// SubDeviceState 订阅设备在线状态
func (c *MipsLocalClient) SubDeviceState(did string, handler func(did string, state MIoTDeviceState, ctx interface{}), ctx interface{}) error {
	topic := fmt.Sprintf("device/%s/state/#", did)

	onStateMsg := func(msgTopic string, payload string, msgCtx interface{}) {
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			return
		}

		if msg["device_id"] == nil || msg["event"] == nil {
			c.logError("on_state_msg, recv unknown msg: %s", payload)
			return
		}

		if msg["device_id"].(string) != did {
			c.logError("on_state_msg, err msg: %s != %s", msg["device_id"], did)
			return
		}

		if handler != nil {
			c.logDebug("local, device state changed: %s", payload)
			state := MIoTDeviceStateOffline
			if msg["event"].(string) == "online" {
				state = MIoTDeviceStateOnline
			}
			handler(did, state, msgCtx)
		}
	}

	return c.regBroadcastExternal(topic, onStateMsg, ctx)
}

// UnsubDeviceState 取消订阅设备在线状态
func (c *MipsLocalClient) UnsubDeviceState(did string) error {
	topic := fmt.Sprintf("device/%s/state/#", did)
	return c.unregBroadcastExternal(topic)
}

// GetPropAsync 异步获取设备属性
func (c *MipsLocalClient) GetPropAsync(did string, siid, piid int, timeoutMs int) (interface{}, error) {
	payload := map[string]interface{}{
		"did":  did,
		"siid": siid,
		"piid": piid,
	}
	payloadBytes, _ := json.Marshal(payload)

	resultObj, err := c.requestAsync("proxy/get", string(payloadBytes), timeoutMs)
	if err != nil {
		return nil, err
	}

	result, ok := resultObj.(map[string]interface{})
	if !ok {
		return nil, nil
	}

	value, exists := result["value"]
	if !exists {
		return nil, nil
	}

	return value, nil
}

// SetPropAsync 异步设置设备属性
func (c *MipsLocalClient) SetPropAsync(did string, siid, piid int, value interface{}, timeoutMs int) (map[string]interface{}, error) {
	payloadObj := map[string]interface{}{
		"did": did,
		"rpc": map[string]interface{}{
			"id":     c.genMipsID(),
			"method": "set_properties",
			"params": []map[string]interface{}{
				{
					"did":   did,
					"siid":  siid,
					"piid":  piid,
					"value": value,
				},
			},
		},
	}
	payloadBytes, _ := json.Marshal(payloadObj)

	resultObj, err := c.requestAsync("proxy/rpcReq", string(payloadBytes), timeoutMs)
	if err != nil {
		return nil, err
	}

	result, ok := resultObj.(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"code":    -1,
			"message": "Invalid result",
		}, nil
	}

	// 检查结果格式
	if resultArr, ok := result["result"].([]interface{}); ok && len(resultArr) > 0 {
		if resultItem, ok := resultArr[0].(map[string]interface{}); ok {
			if resultItem["did"] == did && resultItem["code"] != nil {
				return resultItem, nil
			}
		}
	}

	if errObj, ok := result["error"].(map[string]interface{}); ok {
		return errObj, nil
	}

	return map[string]interface{}{
		"code":    -1,
		"message": "Invalid result",
	}, nil
}

// ActionAsync 异步执行设备动作
func (c *MipsLocalClient) ActionAsync(did string, siid, aiid int, inList []interface{}, timeoutMs int) (map[string]interface{}, error) {
	payloadObj := map[string]interface{}{
		"did": did,
		"rpc": map[string]interface{}{
			"id":     c.genMipsID(),
			"method": "action",
			"params": map[string]interface{}{
				"did":  did,
				"siid": siid,
				"aiid": aiid,
				"in":   inList,
			},
		},
	}
	payloadBytes, _ := json.Marshal(payloadObj)

	resultObj, err := c.requestAsync("proxy/rpcReq", string(payloadBytes), timeoutMs)
	if err != nil {
		return nil, err
	}

	result, ok := resultObj.(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"code":    -1,
			"message": "Invalid result",
		}, nil
	}

	// 检查结果格式
	if resultItem, ok := result["result"].(map[string]interface{}); ok {
		if resultItem["code"] != nil {
			return resultItem, nil
		}
	}

	if errObj, ok := result["error"].(map[string]interface{}); ok {
		return errObj, nil
	}

	return map[string]interface{}{
		"code":    -1,
		"message": "Invalid result",
	}, nil
}

// GetDevListAsync 异步获取设备列表
func (c *MipsLocalClient) GetDevListAsync(payload string, timeoutMs int) (map[string]*MipsDeviceInfo, error) {
	if payload == "" {
		payload = "{}"
	}

	resultObj, err := c.requestAsync("proxy/getDevList", payload, timeoutMs)
	if err != nil {
		return nil, err
	}

	result, ok := resultObj.(map[string]interface{})
	if !ok {
		return nil, &MipsError{Message: "invalid result"}
	}

	devList, ok := result["devList"].(map[string]interface{})
	if !ok {
		return nil, &MipsError{Message: "invalid result: no devList"}
	}

	deviceList := make(map[string]*MipsDeviceInfo)
	for did, info := range devList {
		infoMap, ok := info.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := infoMap["name"].(string)
		urn, _ := infoMap["urn"].(string)
		model, _ := infoMap["model"].(string)

		if name == "" || urn == "" || model == "" {
			c.logError("invalid device info: %s, %v", did, info)
			continue
		}

		// 检查不支持的型号
		if isUnsupportedModel(model) {
			c.logInfo("unsupported model: %s, %s", model, did)
			continue
		}

		deviceList[did] = &MipsDeviceInfo{
			DID:           did,
			Name:          name,
			URN:           urn,
			Model:         model,
			Online:        getBool(infoMap, "online"),
			SpecV2Access:  getBool(infoMap, "specV2Access"),
			PushAvailable: getBool(infoMap, "pushAvailable"),
		}
	}

	return deviceList, nil
}

// GetActionGroupListAsync 获取米家动作组列表
func (c *MipsLocalClient) GetActionGroupListAsync(timeoutMs int) ([]string, error) {
	resultObj, err := c.requestAsync("proxy/getMijiaActionGroupList", "{}", timeoutMs)
	if err != nil {
		return nil, err
	}

	result, ok := resultObj.(map[string]interface{})
	if !ok {
		return nil, &MipsError{Message: "invalid result"}
	}

	resultArr, ok := result["result"].([]interface{})
	if !ok {
		return nil, &MipsError{Message: "invalid result: no result"}
	}

	var actionGroups []string
	for _, item := range resultArr {
		if str, ok := item.(string); ok {
			actionGroups = append(actionGroups, str)
		}
	}

	return actionGroups, nil
}

// ExecActionGroupAsync 执行米家动作组
func (c *MipsLocalClient) ExecActionGroupAsync(agID string, timeoutMs int) (map[string]interface{}, error) {
	payload := fmt.Sprintf(`{"id":"%s"}`, agID)

	resultObj, err := c.requestAsync("proxy/execMijiaActionGroup", payload, timeoutMs)
	if err != nil {
		return nil, err
	}

	result, ok := resultObj.(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"code":    -1,
			"message": "invalid result",
		}, nil
	}

	if resultItem, ok := result["result"].(map[string]interface{}); ok {
		return resultItem, nil
	}

	if errObj, ok := result["error"].(map[string]interface{}); ok {
		return errObj, nil
	}

	return map[string]interface{}{
		"code":    -1,
		"message": "invalid result",
	}, nil
}

// onMipsConnect MIPS 连接回调
func (c *MipsLocalClient) onMipsConnect(rc int, props map[string]interface{}) {
	c.logDebug("on_mips_connect")

	// 订阅 did/# 包含回复主题
	c.mipsSubscribe(fmt.Sprintf("%s/#", c.did), 2)

	// 订阅设备列表变更
	c.mipsSubscribe("master/appMsg/devListChange", 2)

	// 订阅已注册的广播主题
	c.msgMatcherMu.RLock()
	for topic := range c.msgMatcher {
		// 将 did 前缀替换为 master
		masterTopic := regexp.MustCompile(`^`+c.did).ReplaceAllString(topic, "master")
		c.mipsSubscribe(masterTopic, 2)
	}
	c.msgMatcherMu.RUnlock()
}

// onMipsDisconnect MIPS 断开连接回调
func (c *MipsLocalClient) onMipsDisconnect(rc int, props map[string]interface{}) {
	// 清理工作
}

// onMipsMessage MIPS 消息回调
func (c *MipsLocalClient) onMipsMessage(topic string, payload []byte) {
	mipsMsg, err := UnpackMipsMessage(payload)
	if err != nil {
		c.logError("failed to unpack mips message: %v", err)
		return
	}

	// 处理回复消息
	if topic == c.replyTopic {
		c.logDebug("on request reply: %s", mipsMsg.String())

		c.requestMapMu.Lock()
		req, exists := c.requestMap[strconv.Itoa(mipsMsg.MID)]
		if exists {
			delete(c.requestMap, strconv.Itoa(mipsMsg.MID))
		}
		c.requestMapMu.Unlock()

		if exists {
			if req.Timer != nil {
				req.Timer.Stop()
			}
			if req.OnReply != nil {
				go req.OnReply(mipsMsg.Payload, req.OnReplyCtx)
			}
		}
		return
	}

	// 处理广播消息
	c.msgMatcherMu.RLock()
	var matchedHandlers []*MipsBroadcast
	for pattern, broadcast := range c.msgMatcher {
		if matchTopic(pattern, topic) {
			matchedHandlers = append(matchedHandlers, broadcast)
		}
	}
	c.msgMatcherMu.RUnlock()

	if len(matchedHandlers) > 0 {
		c.logDebug("on broadcast: %s, %s", topic, mipsMsg.String())
		for _, item := range matchedHandlers {
			if item.Handler != nil {
				// 提取子主题（去掉 did/ 前缀）
				subTopic := topic
				if idx := strings.Index(topic, "/"); idx != -1 {
					subTopic = topic[idx+1:]
				}
				go item.Handler(subTopic, mipsMsg.Payload, item.HandlerCtx)
			}
		}
		return
	}

	// 处理设备列表变更
	if topic == c.devListChangeTopic {
		if mipsMsg.Payload == "" {
			c.logError("devListChange msg is nil")
			return
		}

		var payloadObj map[string]interface{}
		if err := json.Unmarshal([]byte(mipsMsg.Payload), &payloadObj); err != nil {
			c.logError("unknown devListChange msg: %s", mipsMsg.Payload)
			return
		}

		devList, ok := payloadObj["devList"].([]interface{})
		if !ok || len(devList) == 0 {
			c.logError("unknown devListChange msg: %s", mipsMsg.Payload)
			return
		}

		var devIDs []string
		for _, dev := range devList {
			if str, ok := dev.(string); ok {
				devIDs = append(devIDs, str)
			}
		}

		if c.onDevListChanged != nil {
			go c.onDevListChanged(c, devIDs)
		}
		return
	}

	c.logDebug("mips local client, recv unknown msg: %s -> %s", topic, mipsMsg.String())
}

// request 发送请求
func (c *MipsLocalClient) request(topic, payload string, onReply func(payload string, ctx interface{}), ctx interface{}, timeoutMs int) {
	mid := c.genMipsID()
	pubTopic := fmt.Sprintf("master/%s", topic)

	req := &MipsRequest{
		MID:        mid,
		OnReply:    onReply,
		OnReplyCtx: ctx,
		CreateTime: time.Now(),
	}

	// 打包消息
	mipsPayload, err := PackMipsMessage(mid, payload, "local", c.replyTopic)
	if err != nil {
		c.logError("failed to pack mips message: %v", err)
		return
	}

	// 发布消息
	if err := c.mipsPublish(pubTopic, mipsPayload, 2, false); err != nil {
		c.logError("failed to publish: %v", err)
		return
	}

	c.logDebug("mips local call api: %d, %s, %s", mid, pubTopic, payload)

	// 设置超时
	req.Timer = time.AfterFunc(time.Duration(timeoutMs)*time.Millisecond, func() {
		c.requestMapMu.Lock()
		delete(c.requestMap, strconv.Itoa(mid))
		c.requestMapMu.Unlock()

		c.logError("on mips request timeout: %d, %s, %s", mid, pubTopic, payload)

		if onReply != nil {
			onReply(`{"error":{"code":-10006, "message":"timeout"}}`, ctx)
		}
	})

	c.requestMapMu.Lock()
	c.requestMap[strconv.Itoa(mid)] = req
	c.requestMapMu.Unlock()
}

// requestAsync 异步发送请求
func (c *MipsLocalClient) requestAsync(topic, payload string, timeoutMs int) (interface{}, error) {
	resultCh := make(chan string, 1)

	onReply := func(result string, ctx interface{}) {
		select {
		case resultCh <- result:
		default:
		}
	}

	c.request(topic, payload, onReply, nil, timeoutMs)

	select {
	case result := <-resultCh:
		var resultObj interface{}
		if err := json.Unmarshal([]byte(result), &resultObj); err != nil {
			return map[string]interface{}{
				"code":    -1,
				"message": fmt.Sprintf("Error: %s", result),
			}, nil
		}
		return resultObj, nil
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return nil, &MipsError{Message: "timeout", Code: -10006}
	}
}

// regBroadcastExternal 外部注册广播
func (c *MipsLocalClient) regBroadcastExternal(topic string, handler func(topic string, payload string, ctx interface{}), ctx interface{}) error {
	subTopic := fmt.Sprintf("%s/%s", c.did, topic)

	c.msgMatcherMu.Lock()
	c.msgMatcher[subTopic] = &MipsBroadcast{
		Topic:      subTopic,
		Handler:    handler,
		HandlerCtx: ctx,
	}
	c.msgMatcherMu.Unlock()

	// 订阅 master 主题
	masterTopic := regexp.MustCompile(`^`+c.did).ReplaceAllString(subTopic, "master")
	return c.mipsSubscribe(masterTopic, 2)
}

// unregBroadcastExternal 外部取消注册广播
func (c *MipsLocalClient) unregBroadcastExternal(topic string) error {
	subTopic := fmt.Sprintf("%s/%s", c.did, topic)

	c.msgMatcherMu.Lock()
	delete(c.msgMatcher, subTopic)
	c.msgMatcherMu.Unlock()

	// 取消订阅 master 主题
	masterTopic := regexp.MustCompile(`^`+c.did).ReplaceAllString(subTopic, "master")
	return c.mipsUnsubscribe(masterTopic)
}

// matchTopic 检查主题是否匹配模式（支持 # 通配符）
func matchTopic(pattern, topic string) bool {
	// 简单实现：支持 # 作为多级通配符
	if strings.HasSuffix(pattern, "/#") {
		prefix := strings.TrimSuffix(pattern, "/#")
		return strings.HasPrefix(topic, prefix+"/") || topic == prefix
	}
	return pattern == topic
}

// isUnsupportedModel 检查是否为不支持的型号
func isUnsupportedModel(model string) bool {
	for _, m := range UnsupportedModels {
		if m == model {
			return true
		}
	}
	return false
}

// getBool 从 map 中获取 bool 值
func getBool(m map[string]interface{}, key string) bool {
	v, ok := m[key].(bool)
	return ok && v
}
