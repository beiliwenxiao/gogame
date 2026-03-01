package codec

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// Message IDs for core message types.
const (
	MsgIDLoginRequest    uint16 = 0x0001
	MsgIDMoveRequest     uint16 = 0x0002
	MsgIDSkillCastRequest uint16 = 0x0003
	MsgIDChatMessage     uint16 = 0x0004
)

// ---------- LoginRequest ----------

// LoginRequest represents a client login message.
type LoginRequest struct {
	Token           string `json:"token"`
	ProtocolVersion uint32 `json:"protocol_version"`
}

func (m *LoginRequest) Marshal() ([]byte, error) {
	tokenBytes := []byte(m.Token)
	// Format: [4-byte token length][token bytes][4-byte protocol version]
	buf := make([]byte, 4+len(tokenBytes)+4)
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(tokenBytes)))
	copy(buf[4:4+len(tokenBytes)], tokenBytes)
	binary.BigEndian.PutUint32(buf[4+len(tokenBytes):], m.ProtocolVersion)
	return buf, nil
}

func (m *LoginRequest) Unmarshal(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("LoginRequest: data too short for token length")
	}
	tokenLen := int(binary.BigEndian.Uint32(data[0:4]))
	if len(data) < 4+tokenLen+4 {
		return fmt.Errorf("LoginRequest: data too short for token+version")
	}
	m.Token = string(data[4 : 4+tokenLen])
	m.ProtocolVersion = binary.BigEndian.Uint32(data[4+tokenLen:])
	return nil
}

// ---------- MoveRequest ----------

// MoveRequest represents a movement command.
type MoveRequest struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

func (m *MoveRequest) Marshal() ([]byte, error) {
	buf := make([]byte, 12)
	binary.BigEndian.PutUint32(buf[0:4], math.Float32bits(m.X))
	binary.BigEndian.PutUint32(buf[4:8], math.Float32bits(m.Y))
	binary.BigEndian.PutUint32(buf[8:12], math.Float32bits(m.Z))
	return buf, nil
}

func (m *MoveRequest) Unmarshal(data []byte) error {
	if len(data) < 12 {
		return fmt.Errorf("MoveRequest: data too short, need 12 bytes")
	}
	m.X = math.Float32frombits(binary.BigEndian.Uint32(data[0:4]))
	m.Y = math.Float32frombits(binary.BigEndian.Uint32(data[4:8]))
	m.Z = math.Float32frombits(binary.BigEndian.Uint32(data[8:12]))
	return nil
}

// ---------- SkillCastRequest ----------

// SkillCastRequest represents a skill cast command.
type SkillCastRequest struct {
	SkillID  uint32  `json:"skill_id"`
	TargetID uint64  `json:"target_id"`
	TargetX  float32 `json:"target_x"`
	TargetY  float32 `json:"target_y"`
	TargetZ  float32 `json:"target_z"`
}

func (m *SkillCastRequest) Marshal() ([]byte, error) {
	buf := make([]byte, 24)
	binary.BigEndian.PutUint32(buf[0:4], m.SkillID)
	binary.BigEndian.PutUint64(buf[4:12], m.TargetID)
	binary.BigEndian.PutUint32(buf[12:16], math.Float32bits(m.TargetX))
	binary.BigEndian.PutUint32(buf[16:20], math.Float32bits(m.TargetY))
	binary.BigEndian.PutUint32(buf[20:24], math.Float32bits(m.TargetZ))
	return buf, nil
}

func (m *SkillCastRequest) Unmarshal(data []byte) error {
	if len(data) < 24 {
		return fmt.Errorf("SkillCastRequest: data too short, need 24 bytes")
	}
	m.SkillID = binary.BigEndian.Uint32(data[0:4])
	m.TargetID = binary.BigEndian.Uint64(data[4:12])
	m.TargetX = math.Float32frombits(binary.BigEndian.Uint32(data[12:16]))
	m.TargetY = math.Float32frombits(binary.BigEndian.Uint32(data[16:20]))
	m.TargetZ = math.Float32frombits(binary.BigEndian.Uint32(data[20:24]))
	return nil
}

// ---------- ChatMessage ----------

// ChatMessage represents a chat message.
type ChatMessage struct {
	SenderID uint64 `json:"sender_id"`
	Channel  uint8  `json:"channel"`
	Content  string `json:"content"`
}

func (m *ChatMessage) Marshal() ([]byte, error) {
	contentBytes := []byte(m.Content)
	// Format: [8-byte senderID][1-byte channel][4-byte content length][content bytes]
	buf := make([]byte, 8+1+4+len(contentBytes))
	binary.BigEndian.PutUint64(buf[0:8], m.SenderID)
	buf[8] = m.Channel
	binary.BigEndian.PutUint32(buf[9:13], uint32(len(contentBytes)))
	copy(buf[13:], contentBytes)
	return buf, nil
}

func (m *ChatMessage) Unmarshal(data []byte) error {
	if len(data) < 13 {
		return fmt.Errorf("ChatMessage: data too short for header")
	}
	m.SenderID = binary.BigEndian.Uint64(data[0:8])
	m.Channel = data[8]
	contentLen := int(binary.BigEndian.Uint32(data[9:13]))
	if len(data) < 13+contentLen {
		return fmt.Errorf("ChatMessage: data too short for content")
	}
	m.Content = string(data[13 : 13+contentLen])
	return nil
}

// ---------- Default Registry ----------

// NewDefaultRegistry creates a MessageRegistry pre-populated with all core message types.
func NewDefaultRegistry() *MessageRegistry {
	r := NewMessageRegistry()
	r.Register(MsgIDLoginRequest, "LoginRequest", func() Serializable { return &LoginRequest{} })
	r.Register(MsgIDMoveRequest, "MoveRequest", func() Serializable { return &MoveRequest{} })
	r.Register(MsgIDSkillCastRequest, "SkillCastRequest", func() Serializable { return &SkillCastRequest{} })
	r.Register(MsgIDChatMessage, "ChatMessage", func() Serializable { return &ChatMessage{} })
	return r
}

// ---------- JSON helpers for Serializable ----------

// jsonSerializable is a helper that wraps a Serializable for JSON codec.
// The JSON codec uses json.Marshal/Unmarshal directly on the struct,
// so the Serializable.Marshal/Unmarshal methods are not used for JSON encoding.
// This is by design: JSON codec uses encoding/json, binary codec uses Serializable.

// MarshalJSON implements json.Marshaler for LoginRequest (already handled by struct tags).
// No custom implementation needed since we use struct tags.

// UnmarshalJSON for LoginRequest (already handled by struct tags).
// No custom implementation needed since we use struct tags.

// jsonWrapper is used internally to carry the message type name in JSON format output.
type jsonWrapper struct {
	MsgType string      `json:"msg_type"`
	Data    interface{} `json:"data"`
}

// FormatJSON returns a pretty-printed JSON representation of a message.
func FormatJSON(registry *MessageRegistry, msg Message) string {
	name := registry.Name(msg.ID)
	if msg.Body == nil {
		return fmt.Sprintf("[%s](ID=0x%04X) <nil body>", name, msg.ID)
	}
	w := jsonWrapper{MsgType: name, Data: msg.Body}
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return fmt.Sprintf("[%s](ID=0x%04X) <format error: %v>", name, msg.ID, err)
	}
	return string(b)
}
