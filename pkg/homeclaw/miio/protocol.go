// Package miio implements the Xiaomi miio local UDP protocol for direct
// device communication on the LAN. It handles packet encoding/decoding,
// AES-128-CBC encryption/decryption, and the handshake sequence required
// to obtain the device stamp before sending commands.
package miio

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

const (
	// miioPort is the default UDP port for miio protocol.
	miioPort = 54321
	// helloPacket is sent to discover devices or initiate a session.
	helloMagic = uint16(0x2131)
	// packetHeaderLen is the fixed header size of a miio packet.
	packetHeaderLen = 32
)

// helloPacketBytes is the broadcast hello packet used for device discovery.
var helloPacketBytes = []byte{
	0x21, 0x31, // magic
	0x00, 0x20, // length = 32 (header only)
	0xff, 0xff, 0xff, 0xff, // unknown
	0xff, 0xff, 0xff, 0xff, // device id
	0xff, 0xff, 0xff, 0xff, // stamp
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // md5 checksum (all 0xff = hello)
}

// Packet represents a miio binary packet.
type Packet struct {
	Magic    uint16
	Length   uint16
	Unknown  uint32
	DeviceID uint32
	Stamp    uint32
	Checksum [16]byte
	Data     []byte
}

// parsePacket parses a raw byte slice into a Packet.
func parsePacket(raw []byte) (*Packet, error) {
	if len(raw) < packetHeaderLen {
		return nil, fmt.Errorf("miio: packet too short (%d bytes)", len(raw))
	}
	p := &Packet{
		Magic:    binary.BigEndian.Uint16(raw[0:2]),
		Length:   binary.BigEndian.Uint16(raw[2:4]),
		Unknown:  binary.BigEndian.Uint32(raw[4:8]),
		DeviceID: binary.BigEndian.Uint32(raw[8:12]),
		Stamp:    binary.BigEndian.Uint32(raw[12:16]),
	}
	copy(p.Checksum[:], raw[16:32])
	if int(p.Length) > len(raw) {
		return nil, fmt.Errorf("miio: declared length %d > actual %d", p.Length, len(raw))
	}
	if p.Length > packetHeaderLen {
		p.Data = raw[packetHeaderLen:p.Length]
	}
	return p, nil
}

// buildPacket serialises a Packet into bytes, computing the MD5 checksum.
func buildPacket(p *Packet) []byte {
	totalLen := uint16(packetHeaderLen + len(p.Data))
	buf := make([]byte, totalLen)
	binary.BigEndian.PutUint16(buf[0:2], p.Magic)
	binary.BigEndian.PutUint16(buf[2:4], totalLen)
	binary.BigEndian.PutUint32(buf[4:8], p.Unknown)
	binary.BigEndian.PutUint32(buf[8:12], p.DeviceID)
	binary.BigEndian.PutUint32(buf[12:16], p.Stamp)
	// Placeholder zeros for checksum field.
	copy(buf[16:32], make([]byte, 16))
	if len(p.Data) > 0 {
		copy(buf[32:], p.Data)
	}
	// Compute MD5 over the full packet with zeroed checksum field.
	sum := md5.Sum(buf)
	copy(buf[16:32], sum[:])
	return buf
}

// ─────────────────────────────────────────────────────────────────────────────
// Crypto helpers
// ─────────────────────────────────────────────────────────────────────────────

// deriveKeys derives the AES key and IV from a miio device token.
//
//	key = MD5(token)
//	iv  = MD5(key + token)
func deriveKeys(token []byte) (key, iv []byte) {
	kHash := md5.Sum(token)
	key = kHash[:]
	ivInput := append(kHash[:], token...)
	ivHash := md5.Sum(ivInput)
	iv = ivHash[:]
	return
}

// encrypt encrypts plaintext with AES-128-CBC using the device token.
func encrypt(token, plaintext []byte) ([]byte, error) {
	key, iv := deriveKeys(token)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	// PKCS7 pad.
	bs := block.BlockSize()
	pad := bs - len(plaintext)%bs
	padded := append(plaintext, bytes.Repeat([]byte{byte(pad)}, pad)...)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	return ct, nil
}

// decrypt decrypts AES-128-CBC ciphertext using the device token.
func decrypt(token, ciphertext []byte) ([]byte, error) {
	key, iv := deriveKeys(token)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%block.BlockSize() != 0 {
		return nil, errors.New("miio: ciphertext length not a multiple of block size")
	}
	pt := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ciphertext)
	// Remove PKCS7 padding.
	if len(pt) == 0 {
		return nil, errors.New("miio: empty plaintext after decryption")
	}
	pad := int(pt[len(pt)-1])
	if pad == 0 || pad > block.BlockSize() {
		return nil, fmt.Errorf("miio: invalid padding %d", pad)
	}
	return pt[:len(pt)-pad], nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Client
// ─────────────────────────────────────────────────────────────────────────────

// Client communicates with a single miio device.
type Client struct {
	ip       string
	token    []byte // 16-byte raw token
	deviceID uint32
	stamp    uint32
	msgID    int
	timeout  time.Duration
}

// NewClient creates a miio Client for the device at ip with the given hex token.
// tokenHex is the 32-character hex string (16 bytes) from Mi Home / cloud login.
func NewClient(ip, tokenHex string) (*Client, error) {
	token, err := hexToBytes(tokenHex, 16)
	if err != nil {
		return nil, fmt.Errorf("miio: invalid token %q: %w", tokenHex, err)
	}
	return &Client{
		ip:      ip,
		token:   token,
		timeout: 5 * time.Second,
	}, nil
}

// Handshake performs the miio hello handshake to fetch the device stamp.
// Must be called before the first command.
func (c *Client) Handshake() error {
	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", c.ip, miioPort), c.timeout)
	if err != nil {
		return fmt.Errorf("miio handshake dial: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	if _, err := conn.Write(helloPacketBytes); err != nil {
		return fmt.Errorf("miio handshake write: %w", err)
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("miio handshake read: %w", err)
	}
	pkt, err := parsePacket(buf[:n])
	if err != nil {
		return err
	}
	c.deviceID = pkt.DeviceID
	c.stamp = pkt.Stamp
	return nil
}

// rpcRequest is the JSON-RPC payload sent to a device.
type rpcRequest struct {
	ID     int    `json:"id"`
	Method string `json:"method"`
	Params []any  `json:"params"`
}

// rpcResponse is the JSON-RPC response from a device.
type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Send sends a JSON-RPC command to the device and returns the raw result JSON.
func (c *Client) Send(method string, params []any) (json.RawMessage, error) {
	if c.stamp == 0 {
		if err := c.Handshake(); err != nil {
			return nil, err
		}
	}
	c.msgID++
	req := rpcRequest{ID: c.msgID, Method: method, Params: params}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	encrypted, err := encrypt(c.token, payload)
	if err != nil {
		return nil, err
	}

	pkt := &Packet{
		Magic:    helloMagic,
		Unknown:  0,
		DeviceID: c.deviceID,
		Stamp:    c.stamp,
		Data:     encrypted,
	}
	raw := buildPacket(pkt)

	conn, err := net.DialTimeout("udp", fmt.Sprintf("%s:%d", c.ip, miioPort), c.timeout)
	if err != nil {
		return nil, fmt.Errorf("miio send dial: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write(raw); err != nil {
		return nil, fmt.Errorf("miio send write: %w", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("miio send read: %w", err)
	}
	respPkt, err := parsePacket(buf[:n])
	if err != nil {
		return nil, err
	}
	c.stamp = respPkt.Stamp // keep stamp in sync

	if len(respPkt.Data) == 0 {
		return nil, errors.New("miio: empty response data")
	}
	plaintext, err := decrypt(c.token, respPkt.Data)
	if err != nil {
		return nil, err
	}
	var rpcResp rpcResponse
	if err := json.Unmarshal(plaintext, &rpcResp); err != nil {
		return nil, fmt.Errorf("miio: response parse error: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("miio rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// GetProperties fetches a list of miio properties (MIoT spec format).
// props is a slice of {did, siid, piid} objects.
func (c *Client) GetProperties(props []map[string]any) (json.RawMessage, error) {
	params := make([]any, len(props))
	for i, p := range props {
		params[i] = p
	}
	return c.Send("get_properties", params)
}

// SetProperties sets a list of miio properties.
// props is a slice of {did, siid, piid, value} objects.
func (c *Client) SetProperties(props []map[string]any) (json.RawMessage, error) {
	params := make([]any, len(props))
	for i, p := range props {
		params[i] = p
	}
	return c.Send("set_properties", params)
}

// ─────────────────────────────────────────────────────────────────────────────
// Discovery
// ─────────────────────────────────────────────────────────────────────────────

// DiscoveredDevice holds information about a device discovered via LAN broadcast.
type DiscoveredDevice struct {
	IP       string `json:"ip"`
	DeviceID uint32 `json:"device_id"`
	Stamp    uint32 `json:"stamp"`
}

// Discover sends a broadcast hello and collects responding devices within timeout.
func Discover(timeout time.Duration) ([]DiscoveredDevice, error) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("255.255.255.255:%d", miioPort))
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		return nil, fmt.Errorf("miio discover listen: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.WriteToUDP(helloPacketBytes, addr); err != nil {
		return nil, fmt.Errorf("miio discover broadcast: %w", err)
	}

	var found []DiscoveredDevice
	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Timeout or error — stop collecting.
			break
		}
		pkt, pErr := parsePacket(buf[:n])
		if pErr != nil {
			continue
		}
		found = append(found, DiscoveredDevice{
			IP:       remoteAddr.IP.String(),
			DeviceID: pkt.DeviceID,
			Stamp:    pkt.Stamp,
		})
	}
	return found, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// hex helper
// ─────────────────────────────────────────────────────────────────────────────

func hexToBytes(s string, expectedLen int) ([]byte, error) {
	if len(s) != expectedLen*2 {
		return nil, fmt.Errorf("expected %d hex chars, got %d", expectedLen*2, len(s))
	}
	b := make([]byte, expectedLen)
	for i := 0; i < expectedLen; i++ {
		var v byte
		_, err := fmt.Sscanf(s[i*2:i*2+2], "%02x", &v)
		if err != nil {
			return nil, fmt.Errorf("invalid hex at position %d: %w", i*2, err)
		}
		b[i] = v
	}
	return b, nil
}
