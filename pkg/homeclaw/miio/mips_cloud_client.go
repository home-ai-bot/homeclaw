// Package miio 提供小米 MIoT MIPS 云端客户端实现
// 对应 Python 版 miot/miot_mips.py 中的 MipsCloudClient
package miio

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// MipsCloudClient MIPS 云端客户端
type MipsCloudClient struct {
	*MipsClient

	// 云端特定配置
	uuid        string
	cloudServer string
	appID       string

	// 订阅管理
	msgMatcher   map[string]*MipsBroadcast
	msgMatcherMu sync.RWMutex

	// 设备状态订阅
	deviceStateSubs   map[string]*MipsDeviceState
	deviceStateSubsMu sync.RWMutex
}

// NewMipsCloudClient 创建 MIPS 云端客户端
//
// 参数:
//   - uuid: 设备 UUID
//   - cloudServer: 云服务器区域
//   - appID: 应用 ID
//   - token: OAuth 访问令牌（作为 MQTT 密码）
func NewMipsCloudClient(uuid, cloudServer, appID, token string) (*MipsCloudClient, error) {
	host := DefaultCloudBrokerHost
	if cloudServer != "cn" {
		host = cloudServer + "." + DefaultCloudBrokerHost
	}

	config := &MipsClientConfig{
		ClientID: fmt.Sprintf("ha.%s", uuid),
		Host:     host,
		Port:     8883,
		Username: appID,
		Password: token,
	}

	baseClient, err := NewMipsClient(config)
	if err != nil {
		return nil, err
	}

	client := &MipsCloudClient{
		MipsClient:      baseClient,
		uuid:            uuid,
		cloudServer:     cloudServer,
		appID:           appID,
		msgMatcher:      make(map[string]*MipsBroadcast),
		deviceStateSubs: make(map[string]*MipsDeviceState),
	}

	return client, nil
}

// UpdateAccessToken 更新访问令牌
func (c *MipsCloudClient) UpdateAccessToken(token string) {
	c.UpdateMQTTPassword(token)
}

// SubProp 订阅设备属性变更
func (c *MipsCloudClient) SubProp(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, piid *int, ctx interface{}) error {
	topic := fmt.Sprintf("device/%s/property/", did)
	if siid != nil && piid != nil {
		topic = fmt.Sprintf("device/%s/property/%d.%d", did, *siid, *piid)
	} else {
		topic = fmt.Sprintf("device/%s/property/#", did)
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
			c.logDebug("cloud, on properties_changed: %s", payload)
			handler(msg, msgCtx)
		}
	}

	return c.regBroadcast(topic, onPropMsg, ctx)
}

// UnsubProp 取消订阅设备属性变更
func (c *MipsCloudClient) UnsubProp(did string, siid, piid *int) error {
	topic := fmt.Sprintf("device/%s/property/", did)
	if siid != nil && piid != nil {
		topic = fmt.Sprintf("device/%s/property/%d.%d", did, *siid, *piid)
	} else {
		topic = fmt.Sprintf("device/%s/property/#", did)
	}

	return c.unregBroadcast(topic)
}

// SubEvent 订阅设备事件
func (c *MipsCloudClient) SubEvent(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, eiid *int, ctx interface{}) error {
	topic := fmt.Sprintf("device/%s/event/", did)
	if siid != nil && eiid != nil {
		topic = fmt.Sprintf("device/%s/event/%d.%d", did, *siid, *eiid)
	} else {
		topic = fmt.Sprintf("device/%s/event/#", did)
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
			c.logDebug("cloud, on event_occurred: %s", payload)
			handler(msg, msgCtx)
		}
	}

	return c.regBroadcast(topic, onEventMsg, ctx)
}

// UnsubEvent 取消订阅设备事件
func (c *MipsCloudClient) UnsubEvent(did string, siid, eiid *int) error {
	topic := fmt.Sprintf("device/%s/event/", did)
	if siid != nil && eiid != nil {
		topic = fmt.Sprintf("device/%s/event/%d.%d", did, *siid, *eiid)
	} else {
		topic = fmt.Sprintf("device/%s/event/#", did)
	}

	return c.unregBroadcast(topic)
}

// SubDeviceState 订阅设备在线状态
func (c *MipsCloudClient) SubDeviceState(did string, handler func(did string, state MIoTDeviceState, ctx interface{}), ctx interface{}) error {
	// BLE 设备或代理网关子设备不订阅在线状态
	if strings.HasPrefix(did, "blt.") || strings.HasPrefix(did, "proxy.") {
		c.deviceStateSubsMu.Lock()
		c.deviceStateSubs[did] = &MipsDeviceState{
			DID:        did,
			Handler:    handler,
			HandlerCtx: ctx,
		}
		c.deviceStateSubsMu.Unlock()
		return nil
	}

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
			c.logDebug("cloud, device state changed: %s", payload)
			state := MIoTDeviceStateOffline
			if msg["event"].(string) == "online" {
				state = MIoTDeviceStateOnline
			}
			handler(did, state, msgCtx)
		}
	}

	c.deviceStateSubsMu.Lock()
	c.deviceStateSubs[did] = &MipsDeviceState{
		DID:        did,
		Handler:    handler,
		HandlerCtx: ctx,
	}
	c.deviceStateSubsMu.Unlock()

	return c.regBroadcast(topic, onStateMsg, ctx)
}

// UnsubDeviceState 取消订阅设备在线状态
func (c *MipsCloudClient) UnsubDeviceState(did string) error {
	c.deviceStateSubsMu.Lock()
	delete(c.deviceStateSubs, did)
	c.deviceStateSubsMu.Unlock()

	topic := fmt.Sprintf("device/%s/state/#", did)
	return c.unregBroadcast(topic)
}

// GetPropAsync 异步获取设备属性（云端不支持，需通过 HTTP）
func (c *MipsCloudClient) GetPropAsync(did string, siid, piid int, timeoutMs int) (interface{}, error) {
	return nil, &MipsError{Message: "please call in http client"}
}

// SetPropAsync 异步设置设备属性（云端不支持，需通过 HTTP）
func (c *MipsCloudClient) SetPropAsync(did string, siid, piid int, value interface{}, timeoutMs int) (map[string]interface{}, error) {
	return nil, &MipsError{Message: "please call in http client"}
}

// ActionAsync 异步执行设备动作（云端不支持，需通过 HTTP）
func (c *MipsCloudClient) ActionAsync(did string, siid, aiid int, inList []interface{}, timeoutMs int) (map[string]interface{}, error) {
	return nil, &MipsError{Message: "please call in http client"}
}

// GetDevListAsync 异步获取设备列表（云端不支持，需通过 HTTP）
func (c *MipsCloudClient) GetDevListAsync(payload string, timeoutMs int) (map[string]*MipsDeviceInfo, error) {
	return nil, &MipsError{Message: "please call in http client"}
}

// onMipsConnect MIPS 连接回调
func (c *MipsCloudClient) onMipsConnect(rc int, props map[string]interface{}) {
	c.logDebug("on_mips_connect")

	// 重新订阅所有主题
	c.msgMatcherMu.RLock()
	topics := make([]string, 0, len(c.msgMatcher))
	for topic := range c.msgMatcher {
		topics = append(topics, topic)
	}
	c.msgMatcherMu.RUnlock()

	for _, topic := range topics {
		c.mipsSubscribe(topic, 2)
	}
}

// onMipsDisconnect MIPS 断开连接回调
func (c *MipsCloudClient) onMipsDisconnect(rc int, props map[string]interface{}) {
	// 清理工作
}

// onMipsMessage MIPS 消息回调
func (c *MipsCloudClient) onMipsMessage(topic string, payload []byte) {
	// 云端消息不打包，直接是 JSON
	payloadStr := string(payload)

	// 查找匹配的订阅
	c.msgMatcherMu.RLock()
	var matchedHandlers []*MipsBroadcast
	for pattern, broadcast := range c.msgMatcher {
		if matchTopicCloud(pattern, topic) {
			matchedHandlers = append(matchedHandlers, broadcast)
		}
	}
	c.msgMatcherMu.RUnlock()

	if len(matchedHandlers) == 0 {
		return
	}

	c.logDebug("on broadcast: %s", topic)
	for _, item := range matchedHandlers {
		if item.Handler != nil {
			go item.Handler(topic, payloadStr, item.HandlerCtx)
		}
	}
}

// regBroadcast 注册广播订阅
func (c *MipsCloudClient) regBroadcast(topic string, handler func(topic string, payload string, ctx interface{}), ctx interface{}) error {
	c.msgMatcherMu.Lock()
	c.msgMatcher[topic] = &MipsBroadcast{
		Topic:      topic,
		Handler:    handler,
		HandlerCtx: ctx,
	}
	c.msgMatcherMu.Unlock()

	return c.mipsSubscribe(topic, 2)
}

// unregBroadcast 取消注册广播订阅
func (c *MipsCloudClient) unregBroadcast(topic string) error {
	c.msgMatcherMu.Lock()
	delete(c.msgMatcher, topic)
	c.msgMatcherMu.Unlock()

	return c.mipsUnsubscribe(topic)
}

// matchTopicCloud 检查云端主题是否匹配模式
func matchTopicCloud(pattern, topic string) bool {
	// 支持 # 多级通配符和 + 单级通配符
	if strings.HasSuffix(pattern, "/#") {
		prefix := strings.TrimSuffix(pattern, "/#")
		return strings.HasPrefix(topic, prefix+"/") || topic == prefix
	}

	if strings.Contains(pattern, "+") {
		// 将 + 转换为正则表达式匹配单级
		regexPattern := strings.ReplaceAll(regexp.QuoteMeta(pattern), `\+`, `[^/]+`)
		regexPattern = "^" + regexPattern + "$"
		matched, _ := regexp.MatchString(regexPattern, topic)
		return matched
	}

	return pattern == topic
}
