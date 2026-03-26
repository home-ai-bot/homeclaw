// Package miio 提供小米 MIoT MIPS (Pub/Sub) 消息协议实现
// 对应 Python 版 miot/miot_mips.py 中的 _MipsMessage
package miio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// MipsMsgType 表示 MIPS 消息类型
type MipsMsgType uint8

const (
	// MipsMsgTypeID 消息 ID
	MipsMsgTypeID MipsMsgType = 0
	// MipsMsgTypeRetTopic 返回主题
	MipsMsgTypeRetTopic MipsMsgType = 1
	// MipsMsgTypePayload 消息体
	MipsMsgTypePayload MipsMsgType = 2
	// MipsMsgTypeFrom 消息来源
	MipsMsgTypeFrom MipsMsgType = 3
)

// MipsMessage 表示 MIPS 协议消息
// 对应 Python 版 _MipsMessage
type MipsMessage struct {
	MID      int    `json:"mid"`
	From     string `json:"from,omitempty"`
	RetTopic string `json:"ret_topic,omitempty"`
	Payload  string `json:"payload,omitempty"`
}

// String 返回消息字符串表示
func (m *MipsMessage) String() string {
	return fmt.Sprintf("%d, %s, %s, %s", m.MID, m.From, m.RetTopic, m.Payload)
}

// UnpackMipsMessage 解包 MIPS 消息
//
// 消息格式:
//   - 4 bytes: 数据长度 (little-endian)
//   - 1 byte: 类型
//   - N bytes: 数据
func UnpackMipsMessage(data []byte) (*MipsMessage, error) {
	msg := &MipsMessage{}
	dataLen := len(data)
	dataStart := 0

	for dataStart < dataLen {
		if dataStart+5 > dataLen {
			break
		}

		// 读取长度和类型
		unpackLen := binary.LittleEndian.Uint32(data[dataStart : dataStart+4])
		unpackType := data[dataStart+4]
		dataEnd := dataStart + 5 + int(unpackLen)

		if dataEnd > dataLen {
			return nil, fmt.Errorf("invalid message length")
		}

		unpackData := data[dataStart+5 : dataEnd]

		// 去除末尾的 \0
		unpackData = bytes.TrimRight(unpackData, "\x00")

		switch MipsMsgType(unpackType) {
		case MipsMsgTypeID:
			if len(unpackData) >= 4 {
				msg.MID = int(binary.LittleEndian.Uint32(unpackData))
			}
		case MipsMsgTypeRetTopic:
			msg.RetTopic = string(unpackData)
		case MipsMsgTypePayload:
			msg.Payload = string(unpackData)
		case MipsMsgTypeFrom:
			msg.From = string(unpackData)
		}

		dataStart = dataEnd
	}

	return msg, nil
}

// PackMipsMessage 打包 MIPS 消息
func PackMipsMessage(mid int, payload string, from string, retTopic string) ([]byte, error) {
	if payload == "" {
		return nil, fmt.Errorf("payload is required")
	}

	var buf bytes.Buffer

	// MID (4 bytes, little-endian)
	midBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(midBytes, uint32(mid))
	writeMipsField(&buf, midBytes, MipsMsgTypeID)

	// From
	if from != "" {
		writeMipsField(&buf, []byte(from), MipsMsgTypeFrom)
	}

	// RetTopic
	if retTopic != "" {
		writeMipsField(&buf, []byte(retTopic), MipsMsgTypeRetTopic)
	}

	// Payload
	writeMipsField(&buf, []byte(payload), MipsMsgTypePayload)

	return buf.Bytes(), nil
}

// writeMipsField 写入 MIPS 字段
func writeMipsField(buf *bytes.Buffer, data []byte, msgType MipsMsgType) {
	// 长度包含末尾的 \0
	length := uint32(len(data) + 1)
	binary.Write(buf, binary.LittleEndian, length)
	buf.WriteByte(byte(msgType))
	buf.Write(data)
	buf.WriteByte(0) // 末尾 \0
}

// MipsRequest 表示 MIPS 请求
type MipsRequest struct {
	MID        int
	OnReply    func(payload string, ctx interface{})
	OnReplyCtx interface{}
	Timer      *time.Timer
	CreateTime time.Time
}

// MipsBroadcast 表示 MIPS 广播订阅
type MipsBroadcast struct {
	Topic      string
	Handler    func(topic string, payload string, ctx interface{})
	HandlerCtx interface{}
}

// MIoTDeviceState 表示设备状态
type MIoTDeviceState int

const (
	// MIoTDeviceStateDisable 设备禁用
	MIoTDeviceStateDisable MIoTDeviceState = iota
	// MIoTDeviceStateOffline 设备离线
	MIoTDeviceStateOffline
	// MIoTDeviceStateOnline 设备在线
	MIoTDeviceStateOnline
)

// MipsDeviceState 表示 MIPS 设备状态回调
type MipsDeviceState struct {
	DID        string
	Handler    func(did string, state MIoTDeviceState, ctx interface{})
	HandlerCtx interface{}
}

// MipsState 表示 MIPS 连接状态回调
type MipsState struct {
	Key     string
	Handler func(key string, connected bool)
}

// MipsError 表示 MIPS 错误
type MipsError struct {
	Message string
	Code    int
}

func (e *MipsError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("MipsError(code=%d): %s", e.Code, e.Message)
	}
	return fmt.Sprintf("MipsError: %s", e.Message)
}

// MipsRPCPayload 表示 RPC 请求体
type MipsRPCPayload struct {
	DID string  `json:"did"`
	RPC MipsRPC `json:"rpc"`
}

// MipsRPC 表示 RPC 内容
type MipsRPC struct {
	ID     int         `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

// MipsRPCResult 表示 RPC 响应结果
type MipsRPCResult struct {
	DID   string      `json:"did"`
	SIID  int         `json:"siid"`
	PIID  int         `json:"piid"`
	Code  int         `json:"code"`
	Value interface{} `json:"value,omitempty"`
}

// MipsDeviceInfo 表示 MIPS 设备信息
type MipsDeviceInfo struct {
	DID           string `json:"did"`
	Name          string `json:"name"`
	URN           string `json:"urn"`
	Model         string `json:"model"`
	Online        bool   `json:"online"`
	SpecV2Access  bool   `json:"specv2_access"`
	PushAvailable bool   `json:"push_available"`
}

// MipsDevListChange 表示设备列表变更消息
type MipsDevListChange struct {
	DevList []string `json:"devList"`
}
