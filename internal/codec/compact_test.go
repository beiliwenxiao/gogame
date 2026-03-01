package codec

import (
	"errors"
	"strings"
	"testing"

	"gfgame/internal/engine"
)

// ---------- OperationDictionary tests ----------

func TestOperationDictionary_PreRegistered(t *testing.T) {
	d := NewOperationDictionary()

	expected := map[engine.OperationCode]string{
		engine.OpMoveUp:    "move_up",
		engine.OpMoveDown:  "move_down",
		engine.OpMoveLeft:  "move_left",
		engine.OpMoveRight: "move_right",
		engine.OpAttack:    "attack",
		engine.OpSkill:     "skill",
		engine.OpInteract:  "interact",
		engine.OpChat:      "chat",
	}

	all := d.AllCodes()
	if len(all) != len(expected) {
		t.Fatalf("expected %d codes, got %d", len(expected), len(all))
	}

	for code, wantType := range expected {
		gotType, ok := d.GetOpType(code)
		if !ok {
			t.Fatalf("GetOpType(%c): not found", code)
		}
		if gotType != wantType {
			t.Fatalf("GetOpType(%c): got %q, want %q", code, gotType, wantType)
		}

		gotCode, ok := d.GetOpCode(wantType)
		if !ok {
			t.Fatalf("GetOpCode(%q): not found", wantType)
		}
		if gotCode != code {
			t.Fatalf("GetOpCode(%q): got %c, want %c", wantType, gotCode, code)
		}
	}
}

func TestOperationDictionary_UnknownCode(t *testing.T) {
	d := NewOperationDictionary()
	_, ok := d.GetOpType(engine.OperationCode('x'))
	if ok {
		t.Fatal("expected unknown code to return false")
	}
}

func TestOperationDictionary_UnknownType(t *testing.T) {
	d := NewOperationDictionary()
	_, ok := d.GetOpCode("nonexistent")
	if ok {
		t.Fatal("expected unknown type to return false")
	}
}

func TestOperationDictionary_CustomRegister(t *testing.T) {
	d := NewOperationDictionary()
	d.Register(engine.OperationCode('x'), "custom_op")

	opType, ok := d.GetOpType(engine.OperationCode('x'))
	if !ok || opType != "custom_op" {
		t.Fatalf("expected custom_op, got %q (ok=%v)", opType, ok)
	}
}

// ---------- ShortIDMapper tests ----------

func TestShortIDMapper_AssignAndResolve(t *testing.T) {
	m := NewShortIDMapper(2)

	sid, err := m.Assign(engine.EntityID(100))
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if sid != "A" {
		t.Fatalf("expected first ID to be A, got %q", sid)
	}

	eid, ok := m.Resolve("A")
	if !ok || eid != 100 {
		t.Fatalf("Resolve(A): got %d, ok=%v", eid, ok)
	}
}

func TestShortIDMapper_AssignMultiple(t *testing.T) {
	m := NewShortIDMapper(2)

	ids := make([]string, 26)
	for i := 0; i < 26; i++ {
		sid, err := m.Assign(engine.EntityID(i + 1))
		if err != nil {
			t.Fatalf("Assign(%d): %v", i+1, err)
		}
		ids[i] = sid
	}

	// First 26 should be A-Z
	for i := 0; i < 26; i++ {
		expected := string(rune('A' + i))
		if ids[i] != expected {
			t.Fatalf("ID[%d]: got %q, want %q", i, ids[i], expected)
		}
	}

	// 27th should be AA
	sid, err := m.Assign(engine.EntityID(27))
	if err != nil {
		t.Fatalf("Assign(27): %v", err)
	}
	if sid != "AA" {
		t.Fatalf("expected AA, got %q", sid)
	}
}

func TestShortIDMapper_AssignIdempotent(t *testing.T) {
	m := NewShortIDMapper(2)

	sid1, err := m.Assign(engine.EntityID(42))
	if err != nil {
		t.Fatal(err)
	}
	sid2, err := m.Assign(engine.EntityID(42))
	if err != nil {
		t.Fatal(err)
	}
	if sid1 != sid2 {
		t.Fatalf("expected same ID for same entity, got %q and %q", sid1, sid2)
	}
}

func TestShortIDMapper_Release(t *testing.T) {
	m := NewShortIDMapper(2)

	sid, _ := m.Assign(engine.EntityID(1))
	m.Release(engine.EntityID(1))

	_, ok := m.Resolve(sid)
	if ok {
		t.Fatal("expected Resolve to return false after Release")
	}

	_, ok = m.GetShortID(engine.EntityID(1))
	if ok {
		t.Fatal("expected GetShortID to return false after Release")
	}
}

func TestShortIDMapper_GetShortID(t *testing.T) {
	m := NewShortIDMapper(2)

	_, ok := m.GetShortID(engine.EntityID(999))
	if ok {
		t.Fatal("expected false for unmapped entity")
	}

	m.Assign(engine.EntityID(999))
	sid, ok := m.GetShortID(engine.EntityID(999))
	if !ok || sid == "" {
		t.Fatal("expected valid short ID")
	}
}

func TestShortIDMapper_Exhaustion(t *testing.T) {
	m := NewShortIDMapper(1) // Only A-Z (26 IDs)

	for i := 0; i < 26; i++ {
		_, err := m.Assign(engine.EntityID(i + 1))
		if err != nil {
			t.Fatalf("Assign(%d): %v", i+1, err)
		}
	}

	_, err := m.Assign(engine.EntityID(27))
	if !errors.Is(err, ErrShortIDExhausted) {
		t.Fatalf("expected ErrShortIDExhausted, got %v", err)
	}
}

func TestShortIDMapper_ResolveUnknown(t *testing.T) {
	m := NewShortIDMapper(2)
	_, ok := m.Resolve("ZZ")
	if ok {
		t.Fatal("expected false for unknown short ID")
	}
}

// ---------- indexToShortID tests ----------

func TestIndexToShortID(t *testing.T) {
	tests := []struct {
		index int
		want  string
	}{
		{0, "A"},
		{1, "B"},
		{25, "Z"},
		{26, "AA"},
		{27, "AB"},
		{51, "AZ"},
		{52, "BA"},
	}
	for _, tt := range tests {
		got := indexToShortID(tt.index)
		if got != tt.want {
			t.Errorf("indexToShortID(%d) = %q, want %q", tt.index, got, tt.want)
		}
	}
}

// ---------- CompactCodec Encode tests ----------

func newTestCodec() CompactCodec {
	return NewCompactCodec(CompactCodecConfig{
		MaxShortIDLength: 2,
		EnableBatch:      true,
	})
}

func TestCompactCodec_EncodeSimple(t *testing.T) {
	cc := newTestCodec()

	ops := []CompactOperation{
		{SourceShortID: "A", OpCode: engine.OpMoveUp, Params: []string{"10"}},
	}
	encoded, err := cc.Encode(ops)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "Au10" {
		t.Fatalf("expected Au10, got %q", encoded)
	}
}

func TestCompactCodec_EncodeMultipleOps(t *testing.T) {
	cc := newTestCodec()

	ops := []CompactOperation{
		{SourceShortID: "A", OpCode: engine.OpMoveUp, Params: []string{"10"}},
		{SourceShortID: "A", OpCode: engine.OpAttack, Params: []string{"B"}},
	}
	encoded, err := cc.Encode(ops)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "Au10AaB" {
		t.Fatalf("expected Au10AaB, got %q", encoded)
	}
}

func TestCompactCodec_EncodeNoParams(t *testing.T) {
	cc := newTestCodec()

	ops := []CompactOperation{
		{SourceShortID: "B", OpCode: engine.OpAttack, Params: nil},
	}
	encoded, err := cc.Encode(ops)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "Ba" {
		t.Fatalf("expected Ba, got %q", encoded)
	}
}

func TestCompactCodec_EncodeEmptyOps(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.Encode(nil)
	if !errors.Is(err, ErrNoOperations) {
		t.Fatalf("expected ErrNoOperations, got %v", err)
	}
}

func TestCompactCodec_EncodeEmptyShortID(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.Encode([]CompactOperation{{SourceShortID: "", OpCode: engine.OpAttack}})
	if !errors.Is(err, ErrEmptyShortID) {
		t.Fatalf("expected ErrEmptyShortID, got %v", err)
	}
}

func TestCompactCodec_EncodeInvalidOpCode(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.Encode([]CompactOperation{{SourceShortID: "A", OpCode: engine.OperationCode('x')}})
	if !errors.Is(err, ErrInvalidOpCode) {
		t.Fatalf("expected ErrInvalidOpCode, got %v", err)
	}
}

// ---------- CompactCodec Decode tests ----------

func TestCompactCodec_DecodeSimple(t *testing.T) {
	cc := newTestCodec()

	ops, err := cc.Decode("Au10")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].SourceShortID != "A" {
		t.Fatalf("expected short ID A, got %q", ops[0].SourceShortID)
	}
	if ops[0].OpCode != engine.OpMoveUp {
		t.Fatalf("expected OpMoveUp, got %c", ops[0].OpCode)
	}
	if len(ops[0].Params) != 1 || ops[0].Params[0] != "10" {
		t.Fatalf("expected params [10], got %v", ops[0].Params)
	}
}

func TestCompactCodec_DecodeMultipleOps(t *testing.T) {
	cc := newTestCodec()

	ops, err := cc.Decode("Au10AaB")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}

	// First op: A move_up 10
	if ops[0].SourceShortID != "A" || ops[0].OpCode != engine.OpMoveUp {
		t.Fatalf("op[0]: got %+v", ops[0])
	}
	if len(ops[0].Params) != 1 || ops[0].Params[0] != "10" {
		t.Fatalf("op[0] params: got %v", ops[0].Params)
	}

	// Second op: A attack B
	if ops[1].SourceShortID != "A" || ops[1].OpCode != engine.OpAttack {
		t.Fatalf("op[1]: got %+v", ops[1])
	}
	if len(ops[1].Params) != 1 || ops[1].Params[0] != "B" {
		t.Fatalf("op[1] params: got %v", ops[1].Params)
	}
}

func TestCompactCodec_DecodeNoParams(t *testing.T) {
	cc := newTestCodec()

	ops, err := cc.Decode("Ba")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].SourceShortID != "B" || ops[0].OpCode != engine.OpAttack {
		t.Fatalf("got %+v", ops[0])
	}
	if len(ops[0].Params) != 0 {
		t.Fatalf("expected no params, got %v", ops[0].Params)
	}
}

func TestCompactCodec_DecodeMultiCharShortID(t *testing.T) {
	cc := newTestCodec()

	ops, err := cc.Decode("AAu5")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].SourceShortID != "AA" {
		t.Fatalf("expected short ID AA, got %q", ops[0].SourceShortID)
	}
}

func TestCompactCodec_DecodeEmpty(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.Decode("")
	if !errors.Is(err, ErrInvalidCompactData) {
		t.Fatalf("expected ErrInvalidCompactData, got %v", err)
	}
}

func TestCompactCodec_DecodeInvalidStart(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.Decode("1Au10")
	if !errors.Is(err, ErrInvalidCompactData) {
		t.Fatalf("expected ErrInvalidCompactData, got %v", err)
	}
}

func TestCompactCodec_DecodeNoOpCode(t *testing.T) {
	cc := newTestCodec()
	// "AB" - two uppercase letters but no opcode follows
	_, err := cc.Decode("AB")
	if err == nil {
		t.Fatal("expected error for data with no opcode")
	}
}

func TestCompactCodec_DecodeBatchFormat(t *testing.T) {
	cc := newTestCodec()

	// "Au10aB" is the canonical batch example from the spec:
	// Entity A: move_up(10), attack(B)
	ops, err := cc.Decode("Au10aB")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}

	// First op: A move_up 10
	if ops[0].SourceShortID != "A" || ops[0].OpCode != engine.OpMoveUp {
		t.Fatalf("op[0]: got %+v", ops[0])
	}
	if len(ops[0].Params) != 1 || ops[0].Params[0] != "10" {
		t.Fatalf("op[0] params: got %v", ops[0].Params)
	}

	// Second op: A attack B (batch continuation, reuses short ID A)
	if ops[1].SourceShortID != "A" || ops[1].OpCode != engine.OpAttack {
		t.Fatalf("op[1]: got %+v", ops[1])
	}
	if len(ops[1].Params) != 1 || ops[1].Params[0] != "B" {
		t.Fatalf("op[1] params: got %v", ops[1].Params)
	}
}

// ---------- CompactCodec RoundTrip tests ----------

func TestCompactCodec_RoundTrip(t *testing.T) {
	cc := newTestCodec()

	original := []CompactOperation{
		{SourceShortID: "A", OpCode: engine.OpMoveUp, Params: []string{"10"}},
		{SourceShortID: "A", OpCode: engine.OpAttack, Params: []string{"B"}},
		{SourceShortID: "B", OpCode: engine.OpMoveDown, Params: []string{"5"}},
	}

	encoded, err := cc.Encode(original)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := cc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != len(original) {
		t.Fatalf("expected %d ops, got %d", len(original), len(decoded))
	}

	for i := range original {
		if decoded[i].SourceShortID != original[i].SourceShortID {
			t.Fatalf("op[%d] ShortID: got %q, want %q", i, decoded[i].SourceShortID, original[i].SourceShortID)
		}
		if decoded[i].OpCode != original[i].OpCode {
			t.Fatalf("op[%d] OpCode: got %c, want %c", i, decoded[i].OpCode, original[i].OpCode)
		}
		if len(decoded[i].Params) != len(original[i].Params) {
			t.Fatalf("op[%d] Params len: got %d, want %d", i, len(decoded[i].Params), len(original[i].Params))
		}
		for j := range original[i].Params {
			if decoded[i].Params[j] != original[i].Params[j] {
				t.Fatalf("op[%d] Param[%d]: got %q, want %q", i, j, decoded[i].Params[j], original[i].Params[j])
			}
		}
	}
}

func TestCompactCodec_RoundTripNoParams(t *testing.T) {
	cc := newTestCodec()

	original := []CompactOperation{
		{SourceShortID: "C", OpCode: engine.OpInteract},
	}

	encoded, err := cc.Encode(original)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := cc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 op, got %d", len(decoded))
	}
	if decoded[0].SourceShortID != "C" || decoded[0].OpCode != engine.OpInteract {
		t.Fatalf("got %+v", decoded[0])
	}
	if len(decoded[0].Params) != 0 {
		t.Fatalf("expected no params, got %v", decoded[0].Params)
	}
}

func TestCompactCodec_RoundTripMultipleParams(t *testing.T) {
	cc := newTestCodec()

	original := []CompactOperation{
		{SourceShortID: "A", OpCode: engine.OpSkill, Params: []string{"100", "B"}},
	}

	encoded, err := cc.Encode(original)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := cc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != 1 {
		t.Fatalf("expected 1 op, got %d", len(decoded))
	}
	if len(decoded[0].Params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(decoded[0].Params), decoded[0].Params)
	}
	if decoded[0].Params[0] != "100" || decoded[0].Params[1] != "B" {
		t.Fatalf("params mismatch: got %v", decoded[0].Params)
	}
}

// ---------- EncodeBatch tests ----------

func TestCompactCodec_EncodeBatch(t *testing.T) {
	cc := newTestCodec()

	ops := []CompactOperation{
		{SourceShortID: "A", OpCode: engine.OpMoveUp, Params: []string{"10"}},
		{SourceShortID: "A", OpCode: engine.OpAttack, Params: []string{"B"}},
	}

	encoded, err := cc.EncodeBatch("A", ops)
	if err != nil {
		t.Fatal(err)
	}
	// Should be "Au10aB" - entity ID appears once at the start
	if encoded != "Au10aB" {
		t.Fatalf("expected Au10aB, got %q", encoded)
	}
}

func TestCompactCodec_EncodeBatchNoParams(t *testing.T) {
	cc := newTestCodec()

	ops := []CompactOperation{
		{OpCode: engine.OpAttack},
		{OpCode: engine.OpInteract},
	}

	encoded, err := cc.EncodeBatch("B", ops)
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "Bai" {
		t.Fatalf("expected Bai, got %q", encoded)
	}
}

func TestCompactCodec_EncodeBatchDisabled(t *testing.T) {
	cc := NewCompactCodec(CompactCodecConfig{
		MaxShortIDLength: 2,
		EnableBatch:      false,
	})

	_, err := cc.EncodeBatch("A", []CompactOperation{{OpCode: engine.OpAttack}})
	if !errors.Is(err, ErrBatchDisabled) {
		t.Fatalf("expected ErrBatchDisabled, got %v", err)
	}
}

func TestCompactCodec_EncodeBatchEmptyShortID(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.EncodeBatch("", []CompactOperation{{OpCode: engine.OpAttack}})
	if !errors.Is(err, ErrEmptyShortID) {
		t.Fatalf("expected ErrEmptyShortID, got %v", err)
	}
}

func TestCompactCodec_EncodeBatchEmptyOps(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.EncodeBatch("A", nil)
	if !errors.Is(err, ErrNoOperations) {
		t.Fatalf("expected ErrNoOperations, got %v", err)
	}
}

func TestCompactCodec_EncodeBatchInvalidOpCode(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.EncodeBatch("A", []CompactOperation{{OpCode: engine.OperationCode('x')}})
	if !errors.Is(err, ErrInvalidOpCode) {
		t.Fatalf("expected ErrInvalidOpCode, got %v", err)
	}
}

// ---------- Format tests ----------

func TestCompactCodec_Format(t *testing.T) {
	cc := newTestCodec()

	result, err := cc.Format("Au10")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Entity A") {
		t.Fatalf("expected 'Entity A' in output, got %q", result)
	}
	if !strings.Contains(result, "move_up(10)") {
		t.Fatalf("expected 'move_up(10)' in output, got %q", result)
	}
}

func TestCompactCodec_FormatMultipleOps(t *testing.T) {
	cc := newTestCodec()

	result, err := cc.Format("Au10Ba")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Entity A") {
		t.Fatalf("expected 'Entity A' in output, got %q", result)
	}
	if !strings.Contains(result, "Entity B") {
		t.Fatalf("expected 'Entity B' in output, got %q", result)
	}
	if !strings.Contains(result, "attack") {
		t.Fatalf("expected 'attack' in output, got %q", result)
	}
}

func TestCompactCodec_FormatInvalidData(t *testing.T) {
	cc := newTestCodec()
	_, err := cc.Format("")
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestCompactCodec_FormatGroupedOps(t *testing.T) {
	cc := newTestCodec()

	// Two ops from same entity should be grouped
	result, err := cc.Format("Au10Aa")
	if err != nil {
		t.Fatal(err)
	}
	// Should show "Entity A: move_up(10), attack"
	if !strings.Contains(result, "Entity A: move_up(10), attack") {
		t.Fatalf("expected grouped output, got %q", result)
	}
}

// ---------- isValidShortID tests ----------

func TestIsValidShortID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"A", true},
		{"AB", true},
		{"ZZ", true},
		{"", false},
		{"a", false},
		{"A1", false},
		{"Ab", false},
	}
	for _, tt := range tests {
		got := isValidShortID(tt.input)
		if got != tt.want {
			t.Errorf("isValidShortID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------- Complex scenario tests ----------

func TestCompactCodec_ComplexScenario(t *testing.T) {
	cc := newTestCodec()

	// Simulate a tick with multiple entities performing various actions
	ops := []CompactOperation{
		{SourceShortID: "A", OpCode: engine.OpMoveUp, Params: []string{"10"}},
		{SourceShortID: "A", OpCode: engine.OpAttack, Params: []string{"B"}},
		{SourceShortID: "B", OpCode: engine.OpMoveDown, Params: []string{"5"}},
		{SourceShortID: "C", OpCode: engine.OpSkill, Params: []string{"42"}},
		{SourceShortID: "C", OpCode: engine.OpInteract, Params: []string{"7"}},
	}

	encoded, err := cc.Encode(ops)
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := cc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != len(ops) {
		t.Fatalf("expected %d ops, got %d", len(ops), len(decoded))
	}

	for i := range ops {
		if decoded[i].SourceShortID != ops[i].SourceShortID {
			t.Errorf("op[%d] ShortID: got %q, want %q", i, decoded[i].SourceShortID, ops[i].SourceShortID)
		}
		if decoded[i].OpCode != ops[i].OpCode {
			t.Errorf("op[%d] OpCode: got %c, want %c", i, decoded[i].OpCode, ops[i].OpCode)
		}
	}
}

func TestCompactCodec_BatchThenDecode(t *testing.T) {
	cc := newTestCodec()

	// Batch encode for entity A
	ops := []CompactOperation{
		{OpCode: engine.OpMoveRight, Params: []string{"20"}},
		{OpCode: engine.OpAttack},
		{OpCode: engine.OpMoveLeft, Params: []string{"5"}},
	}

	encoded, err := cc.EncodeBatch("A", ops)
	if err != nil {
		t.Fatal(err)
	}

	// Decode should produce operations all attributed to entity A
	decoded, err := cc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}

	if len(decoded) != 3 {
		t.Fatalf("expected 3 ops, got %d", len(decoded))
	}

	// First op: A move_right 20
	if decoded[0].SourceShortID != "A" || decoded[0].OpCode != engine.OpMoveRight {
		t.Fatalf("op[0]: got %+v", decoded[0])
	}
	if len(decoded[0].Params) != 1 || decoded[0].Params[0] != "20" {
		t.Fatalf("op[0] params: got %v", decoded[0].Params)
	}
}

// ---------- NewCompactCodecWithDeps test ----------

func TestNewCompactCodecWithDeps(t *testing.T) {
	mapper := NewShortIDMapper(3)
	dict := NewOperationDictionary()
	cc := NewCompactCodecWithDeps(mapper, dict, CompactCodecConfig{
		MaxShortIDLength: 3,
		EnableBatch:      true,
	})

	if cc.ShortIDMapper() != mapper {
		t.Fatal("expected same mapper instance")
	}
	if cc.Dictionary() != dict {
		t.Fatal("expected same dictionary instance")
	}
}

// ---------- Edge case: decode batch-encoded data ----------

func TestCompactCodec_DecodeBatchEncoded(t *testing.T) {
	cc := newTestCodec()

	// "Au10aB" is the canonical example from the spec
	// Entity A: move_up(10), attack(B)
	ops, err := cc.Decode("Au10aB")
	if err != nil {
		t.Fatal(err)
	}

	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}

	// First: A move_up 10
	if ops[0].SourceShortID != "A" || ops[0].OpCode != engine.OpMoveUp {
		t.Fatalf("op[0]: got %+v", ops[0])
	}
	if len(ops[0].Params) != 1 || ops[0].Params[0] != "10" {
		t.Fatalf("op[0] params: got %v", ops[0].Params)
	}

	// Second: A attack B (batch continuation)
	if ops[1].SourceShortID != "A" || ops[1].OpCode != engine.OpAttack {
		t.Fatalf("op[1]: got %+v", ops[1])
	}
	if len(ops[1].Params) != 1 || ops[1].Params[0] != "B" {
		t.Fatalf("op[1] params: got %v", ops[1].Params)
	}
}

