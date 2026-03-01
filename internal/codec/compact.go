package codec

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"unicode"

	"gfgame/internal/engine"
)

// Errors returned by the compact codec.
var (
	ErrNoOperations       = errors.New("compact: no operations to encode")
	ErrEmptyShortID       = errors.New("compact: empty short ID")
	ErrInvalidOpCode      = errors.New("compact: invalid operation code")
	ErrInvalidCompactData = errors.New("compact: invalid compact data")
	ErrShortIDExhausted   = errors.New("compact: short ID space exhausted")
	ErrEntityNotMapped    = errors.New("compact: entity not mapped")
	ErrBatchDisabled      = errors.New("compact: batch encoding disabled")
)

// CompactOperation represents a single operation in compact format.
type CompactOperation struct {
	SourceShortID string
	OpCode        engine.OperationCode
	Params        []string
}

// CompactMessage holds decoded operations and the raw encoded string.
type CompactMessage struct {
	Operations []CompactOperation
	RawData    string
}

// ---------- OperationDictionary ----------

// OperationDictionary maps operation codes to human-readable type names.
type OperationDictionary interface {
	Register(code engine.OperationCode, opType string)
	GetOpType(code engine.OperationCode) (string, bool)
	GetOpCode(opType string) (engine.OperationCode, bool)
	AllCodes() map[engine.OperationCode]string
}

// opDict is the default implementation of OperationDictionary.
type opDict struct {
	mu         sync.RWMutex
	codeToType map[engine.OperationCode]string
	typeToCode map[string]engine.OperationCode
}

// NewOperationDictionary creates a new OperationDictionary pre-populated with
// all 8 standard operation codes.
func NewOperationDictionary() OperationDictionary {
	d := &opDict{
		codeToType: make(map[engine.OperationCode]string),
		typeToCode: make(map[string]engine.OperationCode),
	}
	d.Register(engine.OpMoveUp, "move_up")
	d.Register(engine.OpMoveDown, "move_down")
	d.Register(engine.OpMoveLeft, "move_left")
	d.Register(engine.OpMoveRight, "move_right")
	d.Register(engine.OpAttack, "attack")
	d.Register(engine.OpSkill, "skill")
	d.Register(engine.OpInteract, "interact")
	d.Register(engine.OpChat, "chat")
	return d
}

func (d *opDict) Register(code engine.OperationCode, opType string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.codeToType[code] = opType
	d.typeToCode[opType] = code
}

func (d *opDict) GetOpType(code engine.OperationCode) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	t, ok := d.codeToType[code]
	return t, ok
}

func (d *opDict) GetOpCode(opType string) (engine.OperationCode, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	c, ok := d.typeToCode[opType]
	return c, ok
}

func (d *opDict) AllCodes() map[engine.OperationCode]string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make(map[engine.OperationCode]string, len(d.codeToType))
	for k, v := range d.codeToType {
		out[k] = v
	}
	return out
}

// ---------- ShortIDMapper ----------

// ShortIDMapper maps long EntityIDs to short uppercase letter IDs.
type ShortIDMapper interface {
	Assign(entityID engine.EntityID) (string, error)
	Resolve(shortID string) (engine.EntityID, bool)
	Release(entityID engine.EntityID)
	GetShortID(entityID engine.EntityID) (string, bool)
}

// shortIDMapper is the default implementation of ShortIDMapper.
// It generates IDs: A, B, ..., Z, AA, AB, ..., AZ, BA, ..., ZZ, etc.
type shortIDMapper struct {
	mu           sync.RWMutex
	maxLen       int
	entityToShort map[engine.EntityID]string
	shortToEntity map[string]engine.EntityID
	nextIndex    int // monotonically increasing counter for ID generation
}

// NewShortIDMapper creates a ShortIDMapper with the given max short ID length.
func NewShortIDMapper(maxLen int) ShortIDMapper {
	if maxLen <= 0 {
		maxLen = 2
	}
	return &shortIDMapper{
		maxLen:        maxLen,
		entityToShort: make(map[engine.EntityID]string),
		shortToEntity: make(map[string]engine.EntityID),
		nextIndex:     0,
	}
}

// indexToShortID converts a zero-based index to an uppercase letter ID.
// 0->A, 1->B, ..., 25->Z, 26->AA, 27->AB, ..., 51->AZ, 52->BA, ...
func indexToShortID(index int) string {
	if index < 26 {
		return string(rune('A' + index))
	}
	var result []byte
	idx := index
	for idx >= 0 {
		result = append([]byte{byte('A' + idx%26)}, result...)
		idx = idx/26 - 1
		if idx < 0 {
			break
		}
	}
	return string(result)
}

func (m *shortIDMapper) Assign(entityID engine.EntityID) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If already assigned, return existing
	if sid, ok := m.entityToShort[entityID]; ok {
		return sid, nil
	}

	// Generate next available short ID
	for {
		sid := indexToShortID(m.nextIndex)
		m.nextIndex++

		if len(sid) > m.maxLen {
			return "", ErrShortIDExhausted
		}

		// Check if this short ID is already in use (shouldn't happen with monotonic counter, but be safe)
		if _, taken := m.shortToEntity[sid]; !taken {
			m.entityToShort[entityID] = sid
			m.shortToEntity[sid] = entityID
			return sid, nil
		}
	}
}

func (m *shortIDMapper) Resolve(shortID string) (engine.EntityID, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	eid, ok := m.shortToEntity[shortID]
	return eid, ok
}

func (m *shortIDMapper) Release(entityID engine.EntityID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if sid, ok := m.entityToShort[entityID]; ok {
		delete(m.shortToEntity, sid)
		delete(m.entityToShort, entityID)
	}
}

func (m *shortIDMapper) GetShortID(entityID engine.EntityID) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sid, ok := m.entityToShort[entityID]
	return sid, ok
}

// ---------- CompactCodec ----------

// CompactCodec encodes/decodes compact operation strings.
type CompactCodec interface {
	Encode(ops []CompactOperation) (string, error)
	Decode(data string) ([]CompactOperation, error)
	EncodeBatch(entityShortID string, ops []CompactOperation) (string, error)
	Format(data string) (string, error)
	ShortIDMapper() ShortIDMapper
	Dictionary() OperationDictionary
}

// CompactCodecConfig holds configuration for the compact codec.
type CompactCodecConfig struct {
	MaxShortIDLength int  // default 2
	EnableBatch      bool // default true
}

// compactCodec is the default implementation of CompactCodec.
type compactCodec struct {
	mapper ShortIDMapper
	dict   OperationDictionary
	config CompactCodecConfig
}

// NewCompactCodec creates a new CompactCodec with the given config.
func NewCompactCodec(config CompactCodecConfig) CompactCodec {
	if config.MaxShortIDLength <= 0 {
		config.MaxShortIDLength = 2
	}
	return &compactCodec{
		mapper: NewShortIDMapper(config.MaxShortIDLength),
		dict:   NewOperationDictionary(),
		config: config,
	}
}

// NewCompactCodecWithDeps creates a CompactCodec with injected dependencies.
func NewCompactCodecWithDeps(mapper ShortIDMapper, dict OperationDictionary, config CompactCodecConfig) CompactCodec {
	return &compactCodec{
		mapper: mapper,
		dict:   dict,
		config: config,
	}
}

func (c *compactCodec) ShortIDMapper() ShortIDMapper { return c.mapper }
func (c *compactCodec) Dictionary() OperationDictionary { return c.dict }

// Encode encodes a slice of CompactOperations into a compact string.
// Format: {ShortID}{opcode}{params...} concatenated for each operation.
// Params are comma-separated. A leading comma separates params from the opcode
// when params are present, ensuring unambiguous parsing.
func (c *compactCodec) Encode(ops []CompactOperation) (string, error) {
	if len(ops) == 0 {
		return "", ErrNoOperations
	}

	var buf strings.Builder
	for _, op := range ops {
		if op.SourceShortID == "" {
			return "", ErrEmptyShortID
		}
		if _, ok := c.dict.GetOpType(op.OpCode); !ok {
			log.Printf("compact: encode: unknown operation code %c", op.OpCode)
			return "", fmt.Errorf("%w: %c", ErrInvalidOpCode, op.OpCode)
		}
		buf.WriteString(op.SourceShortID)
		buf.WriteByte(byte(op.OpCode))
		for j, p := range op.Params {
			if j > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(p)
		}
	}
	return buf.String(), nil
}

// isUpperLetter returns true if the byte is an uppercase ASCII letter.
func isUpperLetter(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

// isOpCode returns true if the byte is a registered operation code.
func (c *compactCodec) isOpCode(b byte) bool {
	_, ok := c.dict.GetOpType(engine.OperationCode(b))
	return ok
}

// Decode parses a compact string back into a slice of CompactOperations.
// It supports both explicit format ({ShortID}{opcode}{params}...) and batch
// format where the short ID appears once and subsequent opcodes reuse it.
// Example: "Au10aB" decodes as entity A: move_up(10), attack(B).
func (c *compactCodec) Decode(data string) ([]CompactOperation, error) {
	if data == "" {
		log.Printf("compact: decode: empty data")
		return nil, fmt.Errorf("%w: empty data", ErrInvalidCompactData)
	}

	var ops []CompactOperation
	i := 0
	n := len(data)
	lastShortID := ""

	for i < n {
		shortID := ""

		if isUpperLetter(data[i]) {
			// Could be a short ID or a param-like uppercase sequence.
			// Consume uppercase letters and check what follows.
			sidStart := i
			for i < n && isUpperLetter(data[i]) {
				i++
			}
			candidate := data[sidStart:i]

			if i < n && c.isOpCode(data[i]) {
				// Uppercase letters followed by an opcode = this is a short ID
				shortID = candidate
			} else if i >= n {
				// Uppercase letters at end of string with no opcode = invalid
				log.Printf("compact: decode: unexpected end after %q at position %d", candidate, sidStart)
				return nil, fmt.Errorf("%w: unexpected end after %q", ErrInvalidCompactData, candidate)
			} else {
				// Uppercase letters NOT followed by opcode = invalid
				log.Printf("compact: decode: expected opcode after %q at position %d", candidate, i)
				return nil, fmt.Errorf("%w: expected opcode at position %d", ErrInvalidCompactData, i)
			}
		} else if c.isOpCode(data[i]) {
			// Opcode without preceding short ID = batch continuation, reuse last short ID
			if lastShortID == "" {
				log.Printf("compact: decode: opcode without short ID at position %d", i)
				return nil, fmt.Errorf("%w: opcode without short ID at position %d", ErrInvalidCompactData, i)
			}
			shortID = lastShortID
		} else {
			log.Printf("compact: decode: unexpected character %c at position %d", data[i], i)
			return nil, fmt.Errorf("%w: unexpected character at position %d", ErrInvalidCompactData, i)
		}

		lastShortID = shortID

		// Parse OpCode
		if i >= n || !c.isOpCode(data[i]) {
			log.Printf("compact: decode: expected opcode at position %d", i)
			return nil, fmt.Errorf("%w: expected opcode at position %d", ErrInvalidCompactData, i)
		}
		opCode := engine.OperationCode(data[i])
		i++

		// Parse Params: consume characters until we hit an opcode (batch continuation)
		// or an uppercase letter directly followed by an opcode (new entity operation).
		// We check one character at a time to correctly handle cases like "AaBBd5"
		// where the first "B" is a param and the second "B" is a new short ID.
		var params []string
		var paramBuf strings.Builder
		for i < n {
			if c.isOpCode(data[i]) {
				// This is the next opcode (batch continuation), stop param parsing
				break
			}
			if isUpperLetter(data[i]) {
				// Check if this uppercase letter starts a new operation:
				// It does if the next non-uppercase character is an opcode.
				// We need to find where the uppercase sequence ends and check what follows.
				nextNonUpper := i + 1
				for nextNonUpper < n && isUpperLetter(data[nextNonUpper]) {
					nextNonUpper++
				}
				if nextNonUpper < n && c.isOpCode(data[nextNonUpper]) {
					// The uppercase sequence data[i:nextNonUpper] followed by opcode = new operation.
					// But we need to determine: how much of this uppercase sequence is the new short ID
					// vs. how much is still a param?
					// Strategy: if we have accumulated param data or this is the first char after opcode,
					// and the SINGLE char at position i followed by checking if i+1 starts a valid
					// operation, we split at the earliest boundary.
					//
					// Check if just data[i] alone, with data[i+1:] being parseable as a new operation:
					if i+1 < n && (c.isOpCode(data[i+1]) || isUpperLetter(data[i+1])) {
						// data[i] could be a param, and data[i+1:] starts a new operation
						// But we need to verify: does data[i+1:] actually start with a valid short ID + opcode?
						if i+1 < n && c.isOpCode(data[i+1]) {
							// data[i] is a param (single uppercase letter), data[i+1] is an opcode
							// Wait, that means data[i] is a param and the opcode at i+1 is batch continuation
							// Actually no: if data[i] is uppercase and data[i+1] is opcode, then
							// data[i] could be a 1-char short ID for a new operation.
							// We need to decide: is data[i] a param or a new short ID?
							// Convention: if we're in param parsing and see uppercase+opcode,
							// it's a new operation boundary.
							if paramBuf.Len() > 0 {
								params = append(params, paramBuf.String())
								paramBuf.Reset()
							}
							break
						}
						// data[i+1] is uppercase - check further
						j := i + 1
						for j < n && isUpperLetter(data[j]) {
							j++
						}
						if j < n && c.isOpCode(data[j]) {
							// data[i] is a param, data[i+1:j] is a new short ID
							paramBuf.WriteByte(data[i])
							i++
							if paramBuf.Len() > 0 {
								params = append(params, paramBuf.String())
								paramBuf.Reset()
							}
							break
						}
					}
					// Default: treat entire uppercase sequence as new operation boundary
					if paramBuf.Len() > 0 {
						params = append(params, paramBuf.String())
						paramBuf.Reset()
					}
					break
				}
				// Uppercase letters NOT followed by opcode = param (e.g., at end of string)
				for i < nextNonUpper {
					paramBuf.WriteByte(data[i])
					i++
				}
				continue
			}
			if data[i] == ',' {
				if paramBuf.Len() > 0 {
					params = append(params, paramBuf.String())
					paramBuf.Reset()
				}
				i++
				continue
			}
			paramBuf.WriteByte(data[i])
			i++
		}
		if paramBuf.Len() > 0 {
			params = append(params, paramBuf.String())
		}

		ops = append(ops, CompactOperation{
			SourceShortID: shortID,
			OpCode:        opCode,
			Params:        params,
		})
	}

	return ops, nil
}

// EncodeBatch merges multiple operations from the same entity into a single
// compact string. The entity's short ID appears once at the beginning, followed
// by all operations concatenated.
func (c *compactCodec) EncodeBatch(entityShortID string, ops []CompactOperation) (string, error) {
	if !c.config.EnableBatch {
		return "", ErrBatchDisabled
	}
	if entityShortID == "" {
		return "", ErrEmptyShortID
	}
	if len(ops) == 0 {
		return "", ErrNoOperations
	}

	var buf strings.Builder
	buf.WriteString(entityShortID)

	for _, op := range ops {
		if _, ok := c.dict.GetOpType(op.OpCode); !ok {
			log.Printf("compact: encode batch: unknown operation code %c", op.OpCode)
			return "", fmt.Errorf("%w: %c", ErrInvalidOpCode, op.OpCode)
		}
		buf.WriteByte(byte(op.OpCode))
		for j, p := range op.Params {
			if j > 0 {
				buf.WriteByte(',')
			}
			buf.WriteString(p)
		}
	}
	return buf.String(), nil
}

// Format decodes a compact string and returns a human-readable representation.
// Example: "Au10aB" -> "Entity A: move_up(10), attack(B)"
func (c *compactCodec) Format(data string) (string, error) {
	ops, err := c.Decode(data)
	if err != nil {
		return "", err
	}

	// Group operations by entity
	type entityOps struct {
		shortID string
		ops     []CompactOperation
	}
	var groups []entityOps
	groupMap := make(map[string]int)

	for _, op := range ops {
		idx, exists := groupMap[op.SourceShortID]
		if !exists {
			idx = len(groups)
			groups = append(groups, entityOps{shortID: op.SourceShortID})
			groupMap[op.SourceShortID] = idx
		}
		groups[idx].ops = append(groups[idx].ops, op)
	}

	var lines []string
	for _, g := range groups {
		var opStrs []string
		for _, op := range g.ops {
			opType, _ := c.dict.GetOpType(op.OpCode)
			if len(op.Params) > 0 {
				opStrs = append(opStrs, fmt.Sprintf("%s(%s)", opType, strings.Join(op.Params, ",")))
			} else {
				opStrs = append(opStrs, opType)
			}
		}
		lines = append(lines, fmt.Sprintf("Entity %s: %s", g.shortID, strings.Join(opStrs, ", ")))
	}
	return strings.Join(lines, "; "), nil
}

// isValidShortID checks if a string is a valid short ID (all uppercase letters).
func isValidShortID(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}
