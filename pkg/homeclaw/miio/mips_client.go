// Package miio 提供小米 MIoT MIPS (Pub/Sub) 客户端基类实现
// 对应 Python 版 miot/miot_mips.py 中的 _MipsClient
package miio

import (
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// MipsClientConfig 表示 MIPS 客户端配置
type MipsClientConfig struct {
	ClientID string
	Host     string
	Port     int
	Username string
	Password string
	CAFile   string
	CertFile string
	KeyFile  string
}

// MipsClient MIPS 客户端基类
type MipsClient struct {
	config *MipsClientConfig

	// MQTT 客户端
	mqtt        mqtt.Client
	mqttOpts    *mqtt.ClientOptions
	mqttMu      sync.RWMutex
	isConnected bool

	// 状态管理
	stateHandlers   map[string]*MipsState
	stateHandlersMu sync.RWMutex

	// 重连管理
	reconnectInterval time.Duration
	reconnectTimer    *time.Timer
	reconnectMu       sync.Mutex

	// 订阅管理
	subPendingMap   map[string]int
	subPendingMu    sync.Mutex
	subPendingTimer *time.Timer

	// 日志
	logger Logger
}

// Logger 日志接口
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// MipsClientInterface 定义 MIPS 客户端接口
type MipsClientInterface interface {
	Connect() error
	Disconnect() error
	IsConnected() bool
	SubMipsState(key string, handler func(key string, connected bool)) error
	UnsubMipsState(key string) error
	SubProp(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, piid *int, ctx interface{}) error
	UnsubProp(did string, siid, piid *int) error
	SubEvent(did string, handler func(msg map[string]interface{}, ctx interface{}), siid, eiid *int, ctx interface{}) error
	UnsubEvent(did string, siid, eiid *int) error
	SubDeviceState(did string, handler func(did string, state MIoTDeviceState, ctx interface{}), ctx interface{}) error
	UnsubDeviceState(did string) error
	GetPropAsync(did string, siid, piid int, timeoutMs int) (interface{}, error)
	SetPropAsync(did string, siid, piid int, value interface{}, timeoutMs int) (map[string]interface{}, error)
	ActionAsync(did string, siid, aiid int, inList []interface{}, timeoutMs int) (map[string]interface{}, error)
	GetDevListAsync(payload string, timeoutMs int) (map[string]*MipsDeviceInfo, error)
}

// NewMipsClient 创建 MIPS 客户端基类
func NewMipsClient(config *MipsClientConfig) (*MipsClient, error) {
	if config.ClientID == "" || config.Host == "" || config.Port == 0 {
		return nil, &MipsError{Message: "invalid config: client_id, host, port are required"}
	}

	client := &MipsClient{
		config:            config,
		stateHandlers:     make(map[string]*MipsState),
		subPendingMap:     make(map[string]int),
		reconnectInterval: 10 * time.Second,
	}

	return client, nil
}

// EnableLogger 启用日志
func (c *MipsClient) EnableLogger(logger Logger) {
	c.logger = logger
}

// logDebug 记录调试日志
func (c *MipsClient) logDebug(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Debug(fmt.Sprintf("[%s] ", c.config.ClientID)+msg, args...)
	}
}

// logInfo 记录信息日志
func (c *MipsClient) logInfo(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Info(fmt.Sprintf("[%s] ", c.config.ClientID)+msg, args...)
	}
}

// logError 记录错误日志
func (c *MipsClient) logError(msg string, args ...interface{}) {
	if c.logger != nil {
		c.logger.Error(fmt.Sprintf("[%s] ", c.config.ClientID)+msg, args...)
	}
}

// Connect 连接到 MQTT Broker
func (c *MipsClient) Connect() error {
	c.mqttMu.Lock()
	defer c.mqttMu.Unlock()

	if c.mqtt != nil && c.mqtt.IsConnected() {
		return nil
	}

	// 创建 MQTT 客户端选项
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("ssl://%s:%d", c.config.Host, c.config.Port))
	opts.SetClientID(c.config.ClientID)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetConnectTimeout(30 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(10 * time.Minute)

	if c.config.Username != "" {
		opts.SetUsername(c.config.Username)
	}
	if c.config.Password != "" {
		opts.SetPassword(c.config.Password)
	}

	// TLS 配置
	if c.config.CAFile != "" || c.config.CertFile != "" {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
		}

		if c.config.CertFile != "" && c.config.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(c.config.CertFile, c.config.KeyFile)
			if err != nil {
				return fmt.Errorf("failed to load client cert: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}

		opts.SetTLSConfig(tlsConfig)
	}

	// 设置回调
	opts.SetOnConnectHandler(c.onMQTTConnect)
	opts.SetConnectionLostHandler(c.onMQTTDisconnect)
	opts.SetDefaultPublishHandler(c.onMQTTMessage)

	c.mqttOpts = opts
	c.mqtt = mqtt.NewClient(opts)

	// 连接
	if token := c.mqtt.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect: %w", token.Error())
	}

	c.isConnected = true
	c.logInfo("connected to %s:%d", c.config.Host, c.config.Port)

	return nil
}

// Disconnect 断开连接
func (c *MipsClient) Disconnect() error {
	c.mqttMu.Lock()
	defer c.mqttMu.Unlock()

	if c.mqtt == nil || !c.mqtt.IsConnected() {
		return nil
	}

	c.mqtt.Disconnect(250)
	c.isConnected = false
	c.logInfo("disconnected from %s:%d", c.config.Host, c.config.Port)

	return nil
}

// IsConnected 检查连接状态
func (c *MipsClient) IsConnected() bool {
	c.mqttMu.RLock()
	defer c.mqttMu.RUnlock()
	return c.isConnected && c.mqtt != nil && c.mqtt.IsConnected()
}

// onMQTTConnect MQTT 连接回调
func (c *MipsClient) onMQTTConnect(client mqtt.Client) {
	c.mqttMu.Lock()
	c.isConnected = true
	c.mqttMu.Unlock()

	c.logDebug("mqtt connected")
	c.onMipsConnect(0, nil)

	// 通知状态处理器
	c.stateHandlersMu.RLock()
	handlers := make(map[string]*MipsState)
	for k, v := range c.stateHandlers {
		handlers[k] = v
	}
	c.stateHandlersMu.RUnlock()

	for _, state := range handlers {
		if state.Handler != nil {
			go state.Handler(state.Key, true)
		}
	}
}

// onMQTTDisconnect MQTT 断开连接回调
func (c *MipsClient) onMQTTDisconnect(client mqtt.Client, err error) {
	c.mqttMu.Lock()
	c.isConnected = false
	c.mqttMu.Unlock()

	c.logError("mqtt disconnected: %v", err)
	c.onMipsDisconnect(0, nil)

	// 通知状态处理器
	c.stateHandlersMu.RLock()
	handlers := make(map[string]*MipsState)
	for k, v := range c.stateHandlers {
		handlers[k] = v
	}
	c.stateHandlersMu.RUnlock()

	for _, state := range handlers {
		if state.Handler != nil {
			go state.Handler(state.Key, false)
		}
	}
}

// onMQTTMessage MQTT 消息回调
func (c *MipsClient) onMQTTMessage(client mqtt.Client, msg mqtt.Message) {
	c.onMipsMessage(msg.Topic(), msg.Payload())
}

// onMipsConnect MIPS 连接回调（子类可重写）
func (c *MipsClient) onMipsConnect(rc int, props map[string]interface{}) {
	// 子类实现
}

// onMipsDisconnect MIPS 断开连接回调（子类可重写）
func (c *MipsClient) onMipsDisconnect(rc int, props map[string]interface{}) {
	// 子类实现
}

// onMipsMessage MIPS 消息回调（子类可重写）
func (c *MipsClient) onMipsMessage(topic string, payload []byte) {
	// 子类实现
}

// SubMipsState 订阅 MIPS 连接状态
func (c *MipsClient) SubMipsState(key string, handler func(key string, connected bool)) error {
	if key == "" || handler == nil {
		return &MipsError{Message: "invalid params"}
	}

	c.stateHandlersMu.Lock()
	c.stateHandlers[key] = &MipsState{
		Key:     key,
		Handler: handler,
	}
	c.stateHandlersMu.Unlock()

	c.logDebug("registered mips state: %s", key)
	return nil
}

// UnsubMipsState 取消订阅 MIPS 连接状态
func (c *MipsClient) UnsubMipsState(key string) error {
	c.stateHandlersMu.Lock()
	delete(c.stateHandlers, key)
	c.stateHandlersMu.Unlock()

	c.logDebug("unregistered mips state: %s", key)
	return nil
}

// mipsSubscribe 内部订阅主题
func (c *MipsClient) mipsSubscribe(topic string, qos byte) error {
	c.mqttMu.RLock()
	client := c.mqtt
	c.mqttMu.RUnlock()

	if client == nil || !client.IsConnected() {
		return &MipsError{Message: "mqtt not connected"}
	}

	if token := client.Subscribe(topic, qos, nil); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	c.logDebug("subscribed to: %s", topic)
	return nil
}

// mipsUnsubscribe 内部取消订阅主题
func (c *MipsClient) mipsUnsubscribe(topic string) error {
	c.mqttMu.RLock()
	client := c.mqtt
	c.mqttMu.RUnlock()

	if client == nil || !client.IsConnected() {
		return &MipsError{Message: "mqtt not connected"}
	}

	if token := client.Unsubscribe(topic); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	c.logDebug("unsubscribed from: %s", topic)
	return nil
}

// mipsPublish 内部发布消息
func (c *MipsClient) mipsPublish(topic string, payload []byte, qos byte, retained bool) error {
	c.mqttMu.RLock()
	client := c.mqtt
	c.mqttMu.RUnlock()

	if client == nil || !client.IsConnected() {
		return &MipsError{Message: "mqtt not connected"}
	}

	if token := client.Publish(topic, qos, retained, payload); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	return nil
}

// UpdateMQTTPassword 更新 MQTT 密码
func (c *MipsClient) UpdateMQTTPassword(password string) {
	c.config.Password = password
	c.mqttMu.Lock()
	if c.mqttOpts != nil {
		c.mqttOpts.SetPassword(password)
	}
	c.mqttMu.Unlock()
}

// GetClientID 获取客户端 ID
func (c *MipsClient) GetClientID() string {
	return c.config.ClientID
}

// GetHost 获取主机地址
func (c *MipsClient) GetHost() string {
	return c.config.Host
}

// GetPort 获取端口
func (c *MipsClient) GetPort() int {
	return c.config.Port
}
