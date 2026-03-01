package codec

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// 核心消息类型的消息 ID。
const (
	MsgIDLoginRequest    uint16 = 0x0001
	MsgIDMoveRequest     uint16 = 0x0002
	MsgIDSkillCastRequest uint16 = 0x0003
	MsgIDChatMessage     uint16 = 0x0004
)

// ---------- LoginRequest ----------

// LoginRequest 表示客户端登录消息。
type LoginRequest struct {
	Token           string `json:"token"`
	ProtocolVersion uint32 `json:"protocol_version"`
}

func (m *LoginRequest) Marshal() ([]byte, error) {
	tokenBytes := []byte(m.Token)
	// 格式：[4字节 token 长度][token 字节][4字节协议版本]
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

// MoveRequest 表示移动指令。
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

// SkillCastRequest 表示技能释放指令。
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

// ChatMessage 表示聊天消息。
type ChatMessage struct {
	SenderID uint64 `json:"sender_id"`
	Channel  uint8  `json:"channel"`
	Content  string `json:"content"`
}

func (m *ChatMessage) Marshal() ([]byte, error) {
	contentBytes := []byte(m.Content)
	// 格式：[8字节发送者ID][1字节频道][4字节内容长度][内容字节]
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

// ---------- 默认注册表 ----------

// NewDefaultRegistry 创建预填充了所有核心消息类型的 MessageRegistry。
func NewDefaultRegistry() *MessageRegistry {
	r := NewMessageRegistry()
	r.Register(MsgIDLoginRequest, "LoginRequest", func() Serializable { return &LoginRequest{} })
	r.Register(MsgIDMoveRequest, "MoveRequest", func() Serializable { return &MoveRequest{} })
	r.Register(MsgIDSkillCastRequest, "SkillCastRequest", func() Serializable { return &SkillCastRequest{} })
	r.Register(MsgIDChatMessage, "ChatMessage", func() Serializable { return &ChatMessage{} })
	return r
}

// ---------- Serializable 的 JSON 辅助工具 ----------

// JSON 编解码器直接对结构体使用 json.Marshal/Unmarshal，
// 因此 Serializable.Marshal/Unmarshal 方法不用于 JSON 编码。
// 这是设计上的选择：JSON 编解码器使用 encoding/json，二进制编解码器使用 Serializable。

// jsonWrapper 在 JSON 格式输出中携带消息类型名称。
type jsonWrapper struct {
	MsgType string      `json:"msg_type"`
	Data    interface{} `json:"data"`
}

// FormatJSON 返回消息的格式化 JSON 表示。
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
