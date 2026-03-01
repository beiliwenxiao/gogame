// Package codec provides message encoding/decoding for the MMRPG game engine.
// It supports both binary (Protobuf-style) and JSON formats, with packet framing
// using the format: [4-byte length (big-endian)][2-byte msgID (big-endian)][body].
package codec

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Serializable defines the interface that all message bodies must implement.
type Serializable interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
}

// Message represents a network message with a type ID and body.
type Message struct {
	ID   uint16
	Body Serializable
}

// CodecType identifies the encoding format.
type CodecType int

const (
	// CodecProtobuf uses binary encoding (encoding/binary for wire format).
	CodecProtobuf CodecType = iota
	// CodecJSON uses JSON text encoding.
	CodecJSON
)

// PacketHeaderSize is the total header size: 4 bytes length + 2 bytes msgID.
const PacketHeaderSize = 6

// MaxPacketBodySize is the maximum allowed message body size (1 MB).
const MaxPacketBodySize = 1 << 20

// Errors returned by the codec.
var (
	ErrUnknownMessageID  = errors.New("codec: unknown message ID")
	ErrBodyNil           = errors.New("codec: message body is nil")
	ErrPacketTooLarge    = errors.New("codec: packet body exceeds maximum size")
	ErrPacketTooSmall    = errors.New("codec: packet too small to contain header")
	ErrInvalidPacketLen  = errors.New("codec: invalid packet length field")
	ErrIncompletePacket  = errors.New("codec: incomplete packet data")
)

// MessageFactory is a function that creates a new zero-value Serializable for a given message type.
type MessageFactory func() Serializable

// MessageRegistry manages the mapping between message IDs and their factory functions.
type MessageRegistry struct {
	mu        sync.RWMutex
	factories map[uint16]MessageFactory
	names     map[uint16]string
}

// NewMessageRegistry creates a new empty MessageRegistry.
func NewMessageRegistry() *MessageRegistry {
	return &MessageRegistry{
		factories: make(map[uint16]MessageFactory),
		names:     make(map[uint16]string),
	}
}

// Register associates a message ID with a factory function and a human-readable name.
func (r *MessageRegistry) Register(id uint16, name string, factory MessageFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[id] = factory
	r.names[id] = name
}

// Create returns a new zero-value Serializable for the given message ID.
func (r *MessageRegistry) Create(id uint16) (Serializable, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[id]
	if !ok {
		return nil, fmt.Errorf("%w: 0x%04X", ErrUnknownMessageID, id)
	}
	return f(), nil
}

// Name returns the human-readable name for a message ID.
func (r *MessageRegistry) Name(id uint16) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n, ok := r.names[id]; ok {
		return n
	}
	return fmt.Sprintf("Unknown(0x%04X)", id)
}

// ---------- MessageCodec interface ----------

// MessageCodec encodes and decodes Message objects.
type MessageCodec interface {
	Encode(msg Message) ([]byte, error)
	Decode(data []byte) (Message, error)
	Format(msg Message) string
}

// ---------- ProtobufCodec (binary) ----------

// ProtobufCodec encodes/decodes messages using binary serialization.
type ProtobufCodec struct {
	registry *MessageRegistry
}

// NewProtobufCodec creates a ProtobufCodec backed by the given registry.
func NewProtobufCodec(registry *MessageRegistry) *ProtobufCodec {
	return &ProtobufCodec{registry: registry}
}

// Encode serializes a Message into bytes: the body is marshalled via Serializable.Marshal().
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

// Decode deserializes bytes into a Message using the registry to create the body.
func (c *ProtobufCodec) Decode(data []byte) (Message, error) {
	// data here is just the body bytes; the caller must provide the msgID separately
	// This method is used after packet framing has extracted the msgID.
	return Message{}, fmt.Errorf("codec: ProtobufCodec.Decode requires msgID; use DecodeWithID")
}

// DecodeWithID deserializes bytes into a Message given the message ID.
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

// Format returns a human-readable representation of the message.
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

// JSONCodec encodes/decodes messages using JSON serialization.
type JSONCodec struct {
	registry *MessageRegistry
}

// NewJSONCodec creates a JSONCodec backed by the given registry.
func NewJSONCodec(registry *MessageRegistry) *JSONCodec {
	return &JSONCodec{registry: registry}
}

// Encode serializes a Message body to JSON bytes.
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

// Decode is not used directly; use DecodeWithID after packet framing.
func (c *JSONCodec) Decode(data []byte) (Message, error) {
	return Message{}, fmt.Errorf("codec: JSONCodec.Decode requires msgID; use DecodeWithID")
}

// DecodeWithID deserializes JSON bytes into a Message given the message ID.
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

// Format returns a human-readable representation of the message.
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

// ---------- Codec (unified) ----------

// Codec wraps a MessageCodec with DecodeWithID support.
type Codec interface {
	Encode(msg Message) ([]byte, error)
	DecodeWithID(id uint16, data []byte) (Message, error)
	Format(msg Message) string
}

// ---------- PacketWriter ----------

// PacketWriter frames a Message into the wire format:
// [4-byte body+2 length (big-endian)][2-byte msgID (big-endian)][body]
// The 4-byte length field stores the size of (2-byte msgID + body).
type PacketWriter struct {
	codec Codec
}

// NewPacketWriter creates a PacketWriter using the given Codec.
func NewPacketWriter(codec Codec) *PacketWriter {
	return &PacketWriter{codec: codec}
}

// WritePacket encodes a Message and frames it into a complete packet.
func (pw *PacketWriter) WritePacket(msg Message) ([]byte, error) {
	body, err := pw.codec.Encode(msg)
	if err != nil {
		return nil, err
	}
	if len(body) > MaxPacketBodySize {
		return nil, ErrPacketTooLarge
	}
	// length = 2 (msgID) + len(body)
	payloadLen := 2 + len(body)
	pkt := make([]byte, 4+payloadLen)
	binary.BigEndian.PutUint32(pkt[0:4], uint32(payloadLen))
	binary.BigEndian.PutUint16(pkt[4:6], msg.ID)
	copy(pkt[6:], body)
	return pkt, nil
}

// ---------- PacketReader ----------

// PacketReader reads complete packets from a byte stream buffer.
type PacketReader struct {
	codec Codec
	buf   []byte
}

// NewPacketReader creates a PacketReader using the given Codec.
func NewPacketReader(codec Codec) *PacketReader {
	return &PacketReader{codec: codec}
}

// Append adds raw bytes to the internal buffer.
func (pr *PacketReader) Append(data []byte) {
	pr.buf = append(pr.buf, data...)
}

// ReadPacket attempts to extract one complete packet from the buffer.
// Returns io.ErrUnexpectedEOF if not enough data is available yet.
// Returns an error with client info wrapper for invalid packets.
func (pr *PacketReader) ReadPacket() (Message, error) {
	if len(pr.buf) < 4 {
		return Message{}, io.ErrUnexpectedEOF
	}
	payloadLen := int(binary.BigEndian.Uint32(pr.buf[0:4]))
	if payloadLen < 2 {
		// Invalid: payload must contain at least the 2-byte msgID.
		pr.buf = pr.buf[4:] // discard the bad length header
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
	// Consume the packet from the buffer regardless of decode success.
	pr.buf = pr.buf[totalLen:]
	if err != nil {
		return Message{}, err
	}
	return msg, nil
}

// ReadAllPackets extracts all complete packets currently in the buffer.
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

// Buffered returns the number of unprocessed bytes in the buffer.
func (pr *PacketReader) Buffered() int {
	return len(pr.buf)
}

// ---------- InvalidMessageError ----------

// InvalidMessageError wraps a decode error with client identification info.
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

// WrapWithClientID wraps an error with client identification for logging.
func WrapWithClientID(clientID string, err error) error {
	if err == nil {
		return nil
	}
	return &InvalidMessageError{ClientID: clientID, Err: err}
}
