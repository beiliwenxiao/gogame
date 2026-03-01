package codec

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

// ---------- helpers ----------

func defaultRegistry() *MessageRegistry {
	return NewDefaultRegistry()
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertEqual(t *testing.T, label string, got, want interface{}) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", label, got, want)
	}
}

// ---------- MessageRegistry tests ----------

func TestRegistry_RegisterAndCreate(t *testing.T) {
	r := NewMessageRegistry()
	r.Register(0x0001, "TestMsg", func() Serializable { return &LoginRequest{} })

	body, err := r.Create(0x0001)
	assertNoError(t, err)
	if _, ok := body.(*LoginRequest); !ok {
		t.Fatalf("expected *LoginRequest, got %T", body)
	}
}

func TestRegistry_UnknownID(t *testing.T) {
	r := NewMessageRegistry()
	_, err := r.Create(0xFFFF)
	if !errors.Is(err, ErrUnknownMessageID) {
		t.Fatalf("expected ErrUnknownMessageID, got %v", err)
	}
}

func TestRegistry_Name(t *testing.T) {
	r := NewMessageRegistry()
	r.Register(0x0001, "LoginRequest", func() Serializable { return &LoginRequest{} })
	assertEqual(t, "name", r.Name(0x0001), "LoginRequest")
	// Unknown ID returns formatted string
	name := r.Name(0xFFFF)
	if name == "" {
		t.Fatal("expected non-empty name for unknown ID")
	}
}

// ---------- ProtobufCodec tests ----------

func TestProtobufCodec_EncodeDecodeLoginRequest(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	orig := &LoginRequest{Token: "abc123", ProtocolVersion: 42}
	msg := Message{ID: MsgIDLoginRequest, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDLoginRequest, data)
	assertNoError(t, err)

	got := decoded.Body.(*LoginRequest)
	assertEqual(t, "Token", got.Token, orig.Token)
	assertEqual(t, "ProtocolVersion", got.ProtocolVersion, orig.ProtocolVersion)
}

func TestProtobufCodec_EncodeDecodeMoveRequest(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	orig := &MoveRequest{X: 1.5, Y: -2.3, Z: 100.0}
	msg := Message{ID: MsgIDMoveRequest, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDMoveRequest, data)
	assertNoError(t, err)

	got := decoded.Body.(*MoveRequest)
	assertEqual(t, "X", got.X, orig.X)
	assertEqual(t, "Y", got.Y, orig.Y)
	assertEqual(t, "Z", got.Z, orig.Z)
}

func TestProtobufCodec_EncodeDecodeSkillCastRequest(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	orig := &SkillCastRequest{SkillID: 999, TargetID: 12345, TargetX: 10, TargetY: 20, TargetZ: 30}
	msg := Message{ID: MsgIDSkillCastRequest, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDSkillCastRequest, data)
	assertNoError(t, err)

	got := decoded.Body.(*SkillCastRequest)
	assertEqual(t, "SkillID", got.SkillID, orig.SkillID)
	assertEqual(t, "TargetID", got.TargetID, orig.TargetID)
	assertEqual(t, "TargetX", got.TargetX, orig.TargetX)
	assertEqual(t, "TargetY", got.TargetY, orig.TargetY)
	assertEqual(t, "TargetZ", got.TargetZ, orig.TargetZ)
}

func TestProtobufCodec_EncodeDecodeChatMessage(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	orig := &ChatMessage{SenderID: 42, Channel: 3, Content: "Hello, World! 你好世界"}
	msg := Message{ID: MsgIDChatMessage, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDChatMessage, data)
	assertNoError(t, err)

	got := decoded.Body.(*ChatMessage)
	assertEqual(t, "SenderID", got.SenderID, orig.SenderID)
	assertEqual(t, "Channel", got.Channel, orig.Channel)
	assertEqual(t, "Content", got.Content, orig.Content)
}

func TestProtobufCodec_NilBody(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	_, err := c.Encode(Message{ID: MsgIDLoginRequest, Body: nil})
	if !errors.Is(err, ErrBodyNil) {
		t.Fatalf("expected ErrBodyNil, got %v", err)
	}
}

func TestProtobufCodec_UnknownMsgID(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	_, err := c.DecodeWithID(0xFFFF, []byte{1, 2, 3})
	if !errors.Is(err, ErrUnknownMessageID) {
		t.Fatalf("expected ErrUnknownMessageID, got %v", err)
	}
}

func TestProtobufCodec_Format(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	msg := Message{ID: MsgIDLoginRequest, Body: &LoginRequest{Token: "test", ProtocolVersion: 1}}
	s := c.Format(msg)
	if s == "" {
		t.Fatal("expected non-empty format output")
	}
	// Should contain the message name
	if !containsStr(s, "LoginRequest") {
		t.Fatalf("format output should contain 'LoginRequest', got: %s", s)
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- JSONCodec tests ----------

func TestJSONCodec_EncodeDecodeLoginRequest(t *testing.T) {
	reg := defaultRegistry()
	c := NewJSONCodec(reg)

	orig := &LoginRequest{Token: "jwt-token-xyz", ProtocolVersion: 7}
	msg := Message{ID: MsgIDLoginRequest, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDLoginRequest, data)
	assertNoError(t, err)

	got := decoded.Body.(*LoginRequest)
	assertEqual(t, "Token", got.Token, orig.Token)
	assertEqual(t, "ProtocolVersion", got.ProtocolVersion, orig.ProtocolVersion)
}

func TestJSONCodec_EncodeDecodeMoveRequest(t *testing.T) {
	reg := defaultRegistry()
	c := NewJSONCodec(reg)

	orig := &MoveRequest{X: 3.14, Y: 0, Z: -99.9}
	msg := Message{ID: MsgIDMoveRequest, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDMoveRequest, data)
	assertNoError(t, err)

	got := decoded.Body.(*MoveRequest)
	// JSON float32 round-trip may lose precision; compare with tolerance
	if !float32Near(got.X, orig.X) || !float32Near(got.Y, orig.Y) || !float32Near(got.Z, orig.Z) {
		t.Fatalf("MoveRequest mismatch: got {%v,%v,%v}, want {%v,%v,%v}", got.X, got.Y, got.Z, orig.X, orig.Y, orig.Z)
	}
}

func float32Near(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-3
}

func TestJSONCodec_EncodeDecodeChatMessage(t *testing.T) {
	reg := defaultRegistry()
	c := NewJSONCodec(reg)

	orig := &ChatMessage{SenderID: 100, Channel: 1, Content: "GG"}
	msg := Message{ID: MsgIDChatMessage, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDChatMessage, data)
	assertNoError(t, err)

	got := decoded.Body.(*ChatMessage)
	assertEqual(t, "SenderID", got.SenderID, orig.SenderID)
	assertEqual(t, "Channel", got.Channel, orig.Channel)
	assertEqual(t, "Content", got.Content, orig.Content)
}

func TestJSONCodec_Format(t *testing.T) {
	reg := defaultRegistry()
	c := NewJSONCodec(reg)

	msg := Message{ID: MsgIDChatMessage, Body: &ChatMessage{SenderID: 1, Channel: 0, Content: "hi"}}
	s := c.Format(msg)
	if !containsStr(s, "ChatMessage") {
		t.Fatalf("format output should contain 'ChatMessage', got: %s", s)
	}
}

// ---------- PacketWriter / PacketReader tests ----------

func TestPacketWriterReader_RoundTrip(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pw := NewPacketWriter(c)
	pr := NewPacketReader(c)

	orig := Message{ID: MsgIDLoginRequest, Body: &LoginRequest{Token: "hello", ProtocolVersion: 10}}
	pkt, err := pw.WritePacket(orig)
	assertNoError(t, err)

	pr.Append(pkt)
	decoded, err := pr.ReadPacket()
	assertNoError(t, err)

	got := decoded.Body.(*LoginRequest)
	assertEqual(t, "ID", decoded.ID, orig.ID)
	assertEqual(t, "Token", got.Token, "hello")
	assertEqual(t, "ProtocolVersion", got.ProtocolVersion, uint32(10))
}

func TestPacketReader_MultiplePackets(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pw := NewPacketWriter(c)
	pr := NewPacketReader(c)

	msgs := []Message{
		{ID: MsgIDLoginRequest, Body: &LoginRequest{Token: "a", ProtocolVersion: 1}},
		{ID: MsgIDMoveRequest, Body: &MoveRequest{X: 1, Y: 2, Z: 3}},
		{ID: MsgIDChatMessage, Body: &ChatMessage{SenderID: 5, Channel: 0, Content: "test"}},
	}

	var allBytes []byte
	for _, m := range msgs {
		pkt, err := pw.WritePacket(m)
		assertNoError(t, err)
		allBytes = append(allBytes, pkt...)
	}

	pr.Append(allBytes)
	decoded, errs := pr.ReadAllPackets()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(decoded) != 3 {
		t.Fatalf("expected 3 packets, got %d", len(decoded))
	}
	assertEqual(t, "msg[0].ID", decoded[0].ID, MsgIDLoginRequest)
	assertEqual(t, "msg[1].ID", decoded[1].ID, MsgIDMoveRequest)
	assertEqual(t, "msg[2].ID", decoded[2].ID, MsgIDChatMessage)
}

func TestPacketReader_PartialData(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pw := NewPacketWriter(c)
	pr := NewPacketReader(c)

	orig := Message{ID: MsgIDLoginRequest, Body: &LoginRequest{Token: "partial", ProtocolVersion: 5}}
	pkt, err := pw.WritePacket(orig)
	assertNoError(t, err)

	// Feed only half the packet first
	half := len(pkt) / 2
	pr.Append(pkt[:half])
	_, err = pr.ReadPacket()
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}

	// Feed the rest
	pr.Append(pkt[half:])
	decoded, err := pr.ReadPacket()
	assertNoError(t, err)
	got := decoded.Body.(*LoginRequest)
	assertEqual(t, "Token", got.Token, "partial")
}

func TestPacketReader_InvalidPayloadLen(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pr := NewPacketReader(c)

	// Craft a packet with payloadLen = 0 (invalid, must be >= 2)
	bad := make([]byte, 4)
	binary.BigEndian.PutUint32(bad, 0)
	pr.Append(bad)

	_, err := pr.ReadPacket()
	if !errors.Is(err, ErrInvalidPacketLen) {
		t.Fatalf("expected ErrInvalidPacketLen, got %v", err)
	}
}

func TestPacketReader_UnknownMsgID(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pr := NewPacketReader(c)

	// Craft a valid-looking packet with unknown msgID 0xFFFF
	body := []byte{1, 2, 3}
	payloadLen := 2 + len(body)
	pkt := make([]byte, 4+payloadLen)
	binary.BigEndian.PutUint32(pkt[0:4], uint32(payloadLen))
	binary.BigEndian.PutUint16(pkt[4:6], 0xFFFF)
	copy(pkt[6:], body)

	pr.Append(pkt)
	_, err := pr.ReadPacket()
	if !errors.Is(err, ErrUnknownMessageID) {
		t.Fatalf("expected ErrUnknownMessageID, got %v", err)
	}
	// Buffer should be consumed
	assertEqual(t, "buffered", pr.Buffered(), 0)
}

func TestPacketWriterReader_JSONCodec(t *testing.T) {
	reg := defaultRegistry()
	c := NewJSONCodec(reg)
	pw := NewPacketWriter(c)
	pr := NewPacketReader(c)

	orig := Message{ID: MsgIDChatMessage, Body: &ChatMessage{SenderID: 77, Channel: 2, Content: "json test"}}
	pkt, err := pw.WritePacket(orig)
	assertNoError(t, err)

	pr.Append(pkt)
	decoded, err := pr.ReadPacket()
	assertNoError(t, err)

	got := decoded.Body.(*ChatMessage)
	assertEqual(t, "SenderID", got.SenderID, uint64(77))
	assertEqual(t, "Content", got.Content, "json test")
}

// ---------- Packet framing format verification ----------

func TestPacketFormat_WireLayout(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pw := NewPacketWriter(c)

	msg := Message{ID: MsgIDMoveRequest, Body: &MoveRequest{X: 1, Y: 2, Z: 3}}
	pkt, err := pw.WritePacket(msg)
	assertNoError(t, err)

	// Verify wire format: [4-byte length][2-byte msgID][body]
	if len(pkt) < PacketHeaderSize {
		t.Fatalf("packet too short: %d bytes", len(pkt))
	}
	payloadLen := binary.BigEndian.Uint32(pkt[0:4])
	msgID := binary.BigEndian.Uint16(pkt[4:6])
	bodyBytes := pkt[6:]

	assertEqual(t, "payloadLen", int(payloadLen), 2+len(bodyBytes))
	assertEqual(t, "msgID", msgID, MsgIDMoveRequest)
	assertEqual(t, "bodyLen", len(bodyBytes), 12) // 3 x float32
}

// ---------- InvalidMessageError tests ----------

func TestInvalidMessageError(t *testing.T) {
	inner := errors.New("bad data")
	wrapped := WrapWithClientID("client-42", inner)

	var ime *InvalidMessageError
	if !errors.As(wrapped, &ime) {
		t.Fatal("expected InvalidMessageError")
	}
	assertEqual(t, "ClientID", ime.ClientID, "client-42")
	if !errors.Is(wrapped, inner) {
		t.Fatal("expected Unwrap to return inner error")
	}
	if !containsStr(wrapped.Error(), "client-42") {
		t.Fatalf("error string should contain client ID, got: %s", wrapped.Error())
	}
}

func TestWrapWithClientID_Nil(t *testing.T) {
	if WrapWithClientID("x", nil) != nil {
		t.Fatal("expected nil for nil error")
	}
}

// ---------- Edge cases ----------

func TestProtobufCodec_EmptyToken(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	orig := &LoginRequest{Token: "", ProtocolVersion: 0}
	msg := Message{ID: MsgIDLoginRequest, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDLoginRequest, data)
	assertNoError(t, err)

	got := decoded.Body.(*LoginRequest)
	assertEqual(t, "Token", got.Token, "")
	assertEqual(t, "ProtocolVersion", got.ProtocolVersion, uint32(0))
}

func TestProtobufCodec_LargeContent(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)

	// Create a chat message with a large content string
	bigContent := make([]byte, 10000)
	for i := range bigContent {
		bigContent[i] = byte('A' + (i % 26))
	}
	orig := &ChatMessage{SenderID: 1, Channel: 0, Content: string(bigContent)}
	msg := Message{ID: MsgIDChatMessage, Body: orig}

	data, err := c.Encode(msg)
	assertNoError(t, err)

	decoded, err := c.DecodeWithID(MsgIDChatMessage, data)
	assertNoError(t, err)

	got := decoded.Body.(*ChatMessage)
	assertEqual(t, "Content length", len(got.Content), len(orig.Content))
	assertEqual(t, "Content", got.Content, orig.Content)
}

func TestFormatJSON(t *testing.T) {
	reg := defaultRegistry()
	msg := Message{ID: MsgIDLoginRequest, Body: &LoginRequest{Token: "t", ProtocolVersion: 1}}
	s := FormatJSON(reg, msg)
	if !containsStr(s, "LoginRequest") {
		t.Fatalf("FormatJSON should contain message name, got: %s", s)
	}
	if !containsStr(s, "msg_type") {
		t.Fatalf("FormatJSON should contain msg_type field, got: %s", s)
	}
}

func TestPacketReader_Buffered(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pr := NewPacketReader(c)

	assertEqual(t, "initial buffered", pr.Buffered(), 0)
	pr.Append([]byte{1, 2, 3})
	assertEqual(t, "after append", pr.Buffered(), 3)
}

func TestPacketReader_ReadAllPackets_WithErrors(t *testing.T) {
	reg := defaultRegistry()
	c := NewProtobufCodec(reg)
	pw := NewPacketWriter(c)
	pr := NewPacketReader(c)

	// Write a valid packet
	validMsg := Message{ID: MsgIDLoginRequest, Body: &LoginRequest{Token: "ok", ProtocolVersion: 1}}
	validPkt, err := pw.WritePacket(validMsg)
	assertNoError(t, err)

	// Craft an invalid packet (unknown msgID)
	badBody := []byte{0}
	badPayloadLen := 2 + len(badBody)
	badPkt := make([]byte, 4+badPayloadLen)
	binary.BigEndian.PutUint32(badPkt[0:4], uint32(badPayloadLen))
	binary.BigEndian.PutUint16(badPkt[4:6], 0xFFFF)
	copy(badPkt[6:], badBody)

	// Write another valid packet
	validMsg2 := Message{ID: MsgIDMoveRequest, Body: &MoveRequest{X: 5, Y: 6, Z: 7}}
	validPkt2, err := pw.WritePacket(validMsg2)
	assertNoError(t, err)

	// Append all: valid + bad + valid
	pr.Append(validPkt)
	pr.Append(badPkt)
	pr.Append(validPkt2)

	msgs, errs := pr.ReadAllPackets()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 valid messages, got %d", len(msgs))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	assertEqual(t, "msg[0].ID", msgs[0].ID, MsgIDLoginRequest)
	assertEqual(t, "msg[1].ID", msgs[1].ID, MsgIDMoveRequest)
}
