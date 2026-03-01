// Package codec 为 MMRPG 游戏引擎提供消息编解码功能。
// 支持二进制（Protobuf 风格）和 JSON 两种格式，数据包帧格式为：
// [4字节长度（大端序）][2字节消息ID（大端序）][消息体]。
package codec

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Serializable 定义了所有消息体必须实现的接口。
type Serializable interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}

// Message 表示一条带有类型 ID 和消息体的网络消息。
type Message struct {
	ID   uint16
	Body Serializable
}

// CodecType 标识编码格式。
type CodecType int

const (
	// CodecProtobuf 使用二进制编码（encoding/binary 线格式）。
	CodecProtobuf CodecType = iota
	// CodecJSON 使用 JSON 文本编码。
	CodecJSON
)

// PacketHeaderSize 是数据包头部总大小：4字节长度 + 2字节消息ID。
const PacketHeaderSize = 6

// MaxPacketBodySize 是消息体允许的最大大小（1 MB）。
const MaxPacketBodySize = 1 << 20

// codec 返回的错误。
var (
	ErrUnknownMessageID  = errors.New("codec: unknown message ID")
	ErrBodyNil           = errors.New("codec: message body is nil")
	ErrPacketTooLarge    = errors.New("codec: packet body exceeds maximum size")
	ErrPacketTooSmall    = errors.New("codec: packet too small to contain header")
	ErrInvalidPacketLen  = errors.New("codec: invalid packet length field")
	ErrIncompletePacket  = errors.New("codec: incomplete packet data")
)

// MessageFactory 是为给定消息类型创建新零值 Serializable 的函数。
type MessageFactory func() Serializable

// MessageRegistry 管理消息 ID 与工厂函数之间的映射。
type MessageRegistry struct {
	mu        sync.RWMutex
	factories map[uint16]MessageFactory
	names     map[uint16]string
}

// NewMessageRegistry 创建一个新的空 MessageRegistry。
func NewMessageRegistry() *MessageRegistry {
	return &MessageRegistry{
		factories: make(map[uint16]MessageFactory),
		names:     make(map[uint16]string),
	}
}

// Register 将消息 ID 与工厂函数及可读名称关联。
func (r *MessageRegistry) Register(id uint16, name string, factory MessageFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[id] = factory
	r.names[id] = name
}

// Create 为给定消息 ID 返回一个新的零值 Serializable。
func (r *MessageRegistry) Create(id uint16) (Serializable, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[id]
	if !ok {
		return nil, fmt.Errorf("%w: 0x%04X", ErrUnknownMessageID, id)
	}
	return f(), nil
}

// Name 返回消息 ID 对应的可读名称。
func (r *MessageRegistry) Name(id uint16) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n, ok := r.names[id]; ok {
		return n
	}
	return fmt.Sprintf("Unknown(0x%04X)", id)
}

// ---------- MessageCodec 接口 ----------

// MessageCodec 对 Message 对象进行编解码。
type MessageCodec interface {
	Encode(msg Message) ([]byte, error)
	Decode(data []byte) (Message, error)
	Format(msg Message) string
}

// ---------- ProtobufCodec（二进制） ----------

// ProtobufCodec 使用二进制序列化对消息进行编解码。
type ProtobufCodec struct {
	registry *MessageRegistry
}

// NewProtobufCodec 创建一个由给定注册表支持的 ProtobufCodec。
func NewProtobufCodec(registry *MessageRegistry) *ProtobufCodec {
	return &ProtobufCodec{registry: registry}
}

// Encode 通过 Serializable.Marshal() 将 Message 序列化为字节。
func (c *ProtobufCodec) Encode(msg Message) ([]byte, error) {
	if msg.Body == nil {
		return nil, ErrBodyNil
	}
	body, err := msg.Body.Marshal()
	if err != nil {
		return nil, fmt.Errorf("codec: protobuf encode body: %w", err)
	}
	return body, nil
}

// Decode 将字节反序列化为 Message，使用注册表创建消息体。
// 此方法在数据包帧提取出 msgID 后由调用方使用。
func (c *ProtobufCodec) Decode(data []byte) (Message, error) {
	return Message{}, fmt.Errorf("codec: ProtobufCodec.Decode requires msgID; use DecodeWithID")
}

// DecodeWithID 根据消息 ID 将字节反序列化为 Message。
func (c *ProtobufCodec) DecodeWithID(id uint16, data []byte) (Message, error) {
	body, err := c.registry.Create(id)
	if err != nil {
		return Message{}, err
	}
	if err := body.Unmarshal(data); err != nil {
		return Message{}, fmt.Errorf("codec: protobuf decode body: %w", err)
	}
	return Message{ID: id, Body: body}, nil
}

// Format 返回消息的可读表示。
func (c *ProtobufCodec) Format(msg Message) string {
	name := c.registry.Name(msg.ID)
	if msg.Body == nil {
		return fmt.Sprintf("[%s](ID=0x%04X) <nil body>", name, msg.ID)
	}
	body, err := json.MarshalIndent(msg.Body, "", "  ")
	if err != nil {
		return fmt.Sprintf("[%s](ID=0x%04X) <format error: %v>", name, msg.ID, err)
	}
	return fmt.Sprintf("[%s](ID=0x%04X) %s", name, msg.ID, string(body))
}

// ---------- JSONCodec ----------

// JSONCodec 使用 JSON 序列化对消息进行编解码。
type JSONCodec struct {
	registry *MessageRegistry
}

// NewJSONCodec 创建一个由给定注册表支持的 JSONCodec。
func NewJSONCodec(registry *MessageRegistry) *JSONCodec {
	return &JSONCodec{registry: registry}
}

// Encode 将 Message 消息体序列化为 JSON 字节。
func (c *JSONCodec) Encode(msg Message) ([]byte, error) {
	if msg.Body == nil {
		return nil, ErrBodyNil
	}
	body, err := json.Marshal(msg.Body)
	if err != nil {
		return nil, fmt.Errorf("codec: json encode body: %w", err)
	}
	return body, nil
}

// Decode 不直接使用；数据包帧处理后请使用 DecodeWithID。
func (c *JSONCodec) Decode(data []byte) (Message, error) {
	return Message{}, fmt.Errorf("codec: JSONCodec.Decode requires msgID; use DecodeWithID")
}

// DecodeWithID 根据消息 ID 将 JSON 字节反序列化为 Message。
func (c *JSONCodec) DecodeWithID(id uint16, data []byte) (Message, error) {
	body, err := c.registry.Create(id)
	if err != nil {
		return Message{}, err
	}
	if err := json.Unmarshal(data, body); err != nil {
		return Message{}, fmt.Errorf("codec: json decode body: %w", err)
	}
	return Message{ID: id, Body: body}, nil
}

// Format 返回消息的可读表示。
func (c *JSONCodec) Format(msg Message) string {
	name := c.registry.Name(msg.ID)
	if msg.Body == nil {
		return fmt.Sprintf("[%s](ID=0x%04X) <nil body>", name, msg.ID)
	}
	body, err := json.MarshalIndent(msg.Body, "", "  ")
	if err != nil {
		return fmt.Sprintf("[%s](ID=0x%04X) <format error: %v>", name, msg.ID, err)
	}
	return fmt.Sprintf("[%s](ID=0x%04X) %s", name, msg.ID, string(body))
}

// ---------- Codec（统一接口） ----------

// Codec 封装了支持 DecodeWithID 的 MessageCodec。
type Codec interface {
	Encode(msg Message) ([]byte, error)
	DecodeWithID(id uint16, data []byte) (Message, error)
	Format(msg Message) string
}

// ---------- PacketWriter ----------

// PacketWriter 将 Message 封装为线格式：
// [4字节 body+2 长度（大端序）][2字节消息ID（大端序）][消息体]
// 4字节长度字段存储（2字节消息ID + 消息体）的大小。
type PacketWriter struct {
	codec Codec
}

// NewPacketWriter 使用给定 Codec 创建 PacketWriter。
func NewPacketWriter(codec Codec) *PacketWriter {
	return &PacketWriter{codec: codec}
}

// WritePacket 对 Message 进行编码并封装为完整数据包。
func (pw *PacketWriter) WritePacket(msg Message) ([]byte, error) {
	body, err := pw.codec.Encode(msg)
	if err != nil {
		return nil, err
	}
	if len(body) > MaxPacketBodySize {
		return nil, ErrPacketTooLarge
	}
	// length = 2（消息ID）+ len(body)
	payloadLen := 2 + len(body)
	pkt := make([]byte, 4+payloadLen)
	binary.BigEndian.PutUint32(pkt[0:4], uint32(payloadLen))
	binary.BigEndian.PutUint16(pkt[4:6], msg.ID)
	copy(pkt[6:], body)
	return pkt, nil
}

// ---------- PacketReader ----------

// PacketReader 从字节流缓冲区中读取完整数据包。
type PacketReader struct {
	codec Codec
	buf   []byte
}

// NewPacketReader 使用给定 Codec 创建 PacketReader。
func NewPacketReader(codec Codec) *PacketReader {
	return &PacketReader{codec: codec}
}

// Append 将原始字节追加到内部缓冲区。
func (pr *PacketReader) Append(data []byte) {
	pr.buf = append(pr.buf, data...)
}

// ReadPacket 尝试从缓冲区中提取一个完整数据包。
// 若数据不足则返回 io.ErrUnexpectedEOF。
// 无效数据包返回带客户端信息的错误。
func (pr *PacketReader) ReadPacket() (Message, error) {
	if len(pr.buf) < 4 {
		return Message{}, io.ErrUnexpectedEOF
	}
	payloadLen := int(binary.BigEndian.Uint32(pr.buf[0:4]))
	if payloadLen < 2 {
		// 无效：payload 至少需要包含 2 字节消息ID。
		pr.buf = pr.buf[4:] // 丢弃错误的长度头
		return Message{}, ErrInvalidPacketLen
	}
	if payloadLen-2 > MaxPacketBodySize {
		pr.buf = pr.buf[4:]
		return Message{}, ErrPacketTooLarge
	}
	totalLen := 4 + payloadLen
	if len(pr.buf) < totalLen {
		return Message{}, io.ErrUnexpectedEOF
	}
	msgID := binary.BigEndian.Uint16(pr.buf[4:6])
	body := pr.buf[6:totalLen]

	msg, err := pr.codec.DecodeWithID(msgID, body)
	// 无论解码是否成功，都从缓冲区消费该数据包。
	pr.buf = pr.buf[totalLen:]
	if err != nil {
		return Message{}, err
	}
	return msg, nil
}

// ReadAllPackets 提取缓冲区中所有完整数据包。
func (pr *PacketReader) ReadAllPackets() ([]Message, []error) {
	var msgs []Message
	var errs []error
	for {
		msg, err := pr.ReadPacket()
		if err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		msgs = append(msgs, msg)
	}
	return msgs, errs
}

// Buffered 返回缓冲区中未处理的字节数。
func (pr *PacketReader) Buffered() int {
	return len(pr.buf)
}

// ---------- InvalidMessageError ----------

// InvalidMessageError 将解码错误与客户端标识信息封装在一起。
type InvalidMessageError struct {
	ClientID string
	Err      error
}

func (e *InvalidMessageError) Error() string {
	return fmt.Sprintf("codec: invalid message from client %q: %v", e.ClientID, e.Err)
}

func (e *InvalidMessageError) Unwrap() error {
	return e.Err
}

// WrapWithClientID 将错误与客户端标识封装，用于日志记录。
func WrapWithClientID(clientID string, err error) error {
	if err == nil {
		return nil
	}
	return &InvalidMessageError{ClientID: clientID, Err: err}
}
