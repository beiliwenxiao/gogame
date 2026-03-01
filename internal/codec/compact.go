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

// compact 编解码器返回的错误。
var (
	ErrNoOperations       = errors.New("compact: no operations to encode")
	ErrEmptyShortID       = errors.New("compact: empty short ID")
	ErrInvalidOpCode      = errors.New("compact: invalid operation code")
	ErrInvalidCompactData = errors.New("compact: invalid compact data")
	ErrShortIDExhausted   = errors.New("compact: short ID space exhausted")
	ErrEntityNotMapped    = errors.New("compact: entity not mapped")
	ErrBatchDisabled      = errors.New("compact: batch encoding disabled")
)

// CompactOperation 表示紧凑格式中的单个操作。
type CompactOperation struct {
	SourceShortID string
	OpCode        engine.OperationCode
	Params        []string
}

// CompactMessage 保存解码后的操作及原始编码字符串。
type CompactMessage struct {
	Operations []CompactOperation
	RawData    string
}

// ---------- OperationDictionary ----------

// OperationDictionary 将操作码映射到可读类型名称。
type OperationDictionary interface {
	Register(code engine.OperationCode, opType string)
	GetOpType(code engine.OperationCode) (string, bool)
	GetOpCode(opType string) (engine.OperationCode, bool)
	AllCodes() map[engine.OperationCode]string
}

// opDict 是 OperationDictionary 的默认实现。
type opDict struct {
	mu         sync.RWMutex
	codeToType map[engine.OperationCode]string
	typeToCode map[string]engine.OperationCode
}

// NewOperationDictionary 创建一个预填充了 8 个标准操作码的 OperationDictionary。
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

// ShortIDMapper 将长 EntityID 映射为短大写字母 ID。
type ShortIDMapper interface {
	Assign(entityID engine.EntityID) (string, error)
	Resolve(shortID string) (engine.EntityID, bool)
	Release(entityID engine.EntityID)
	GetShortID(entityID engine.EntityID) (string, bool)
}

// shortIDMapper 是 ShortIDMapper 的默认实现。
// 生成 ID 规则：A, B, ..., Z, AA, AB, ..., AZ, BA, ..., ZZ 等。
type shortIDMapper struct {
	mu           sync.RWMutex
	maxLen       int
	entityToShort map[engine.EntityID]string
	shortToEntity map[string]engine.EntityID
	nextIndex    int // 单调递增计数器，用于 ID 生成
}

// NewShortIDMapper 创建一个具有给定最大短 ID 长度的 ShortIDMapper。
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

// indexToShortID 将从零开始的索引转换为大写字母 ID。
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

	// 若已分配，返回已有的短 ID
	if sid, ok := m.entityToShort[entityID]; ok {
		return sid, nil
	}

	// 生成下一个可用短 ID
	for {
		sid := indexToShortID(m.nextIndex)
		m.nextIndex++

		if len(sid) > m.maxLen {
			return "", ErrShortIDExhausted
		}

		// 检查短 ID 是否已被占用（单调计数器下通常不会，但保险起见）
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

// CompactCodec 对紧凑操作字符串进行编解码。
type CompactCodec interface {
	Encode(ops []CompactOperation) (string, error)
	Decode(data string) ([]CompactOperation, error)
	EncodeBatch(entityShortID string, ops []CompactOperation) (string, error)
	Format(data string) (string, error)
	ShortIDMapper() ShortIDMapper
	Dictionary() OperationDictionary
}

// CompactCodecConfig 保存紧凑编解码器的配置。
type CompactCodecConfig struct {
	MaxShortIDLength int  // 默认 2
	EnableBatch      bool // 默认 true
}

// compactCodec 是 CompactCodec 的默认实现。
type compactCodec struct {
	mapper ShortIDMapper
	dict   OperationDictionary
	config CompactCodecConfig
}

// NewCompactCodec 使用给定配置创建新的 CompactCodec。
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

// NewCompactCodecWithDeps 使用注入的依赖创建 CompactCodec。
func NewCompactCodecWithDeps(mapper ShortIDMapper, dict OperationDictionary, config CompactCodecConfig) CompactCodec {
	return &compactCodec{
		mapper: mapper,
		dict:   dict,
		config: config,
	}
}

func (c *compactCodec) ShortIDMapper() ShortIDMapper { return c.mapper }
func (c *compactCodec) Dictionary() OperationDictionary { return c.dict }

// Encode 将 CompactOperation 切片编码为紧凑字符串。
// 格式：{短ID}{操作码}{参数...}，每个操作依次拼接。
// 参数以逗号分隔。当存在参数时，操作码后紧跟参数，确保解析无歧义。
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

// isUpperLetter 判断字节是否为大写 ASCII 字母。
func isUpperLetter(b byte) bool {
	return b >= 'A' && b <= 'Z'
}

// isOpCode 判断字节是否为已注册的操作码。
func (c *compactCodec) isOpCode(b byte) bool {
	_, ok := c.dict.GetOpType(engine.OperationCode(b))
	return ok
}

// Decode 将紧凑字符串解析回 CompactOperation 切片。
// 支持显式格式（{短ID}{操作码}{参数}...）和批量格式
// （短 ID 出现一次，后续操作码复用该 ID）。
// 示例："Au10aB" 解码为实体 A：move_up(10), attack(B)。
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
			// 可能是短 ID 或类似参数的大写序列。
			// 消费大写字母并检查后续字符。
			sidStart := i
			for i < n && isUpperLetter(data[i]) {
				i++
			}
			candidate := data[sidStart:i]

			if i < n && c.isOpCode(data[i]) {
				// 大写字母后跟操作码 = 这是短 ID
				shortID = candidate
			} else if i >= n {
				// 大写字母在字符串末尾且无操作码 = 无效
				log.Printf("compact: decode: unexpected end after %q at position %d", candidate, sidStart)
				return nil, fmt.Errorf("%w: unexpected end after %q", ErrInvalidCompactData, candidate)
			} else {
				// 大写字母后不跟操作码 = 无效
				log.Printf("compact: decode: expected opcode after %q at position %d", candidate, i)
				return nil, fmt.Errorf("%w: expected opcode at position %d", ErrInvalidCompactData, i)
			}
		} else if c.isOpCode(data[i]) {
			// 操作码前无短 ID = 批量续传，复用上一个短 ID
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

		// 解析操作码
		if i >= n || !c.isOpCode(data[i]) {
			log.Printf("compact: decode: expected opcode at position %d", i)
			return nil, fmt.Errorf("%w: expected opcode at position %d", ErrInvalidCompactData, i)
		}
		opCode := engine.OperationCode(data[i])
		i++

		// 解析参数：逐字符消费，直到遇到操作码（批量续传）
		// 或紧跟操作码的大写字母（新实体操作）。
		var params []string
		var paramBuf strings.Builder
		for i < n {
			if c.isOpCode(data[i]) {
				// 这是下一个操作码（批量续传），停止参数解析
				break
			}
			if isUpperLetter(data[i]) {
				// 检查此大写字母是否开始新操作：
				// 若后续非大写字符是操作码，则是新操作。
				nextNonUpper := i + 1
				for nextNonUpper < n && isUpperLetter(data[nextNonUpper]) {
					nextNonUpper++
				}
				if nextNonUpper < n && c.isOpCode(data[nextNonUpper]) {
					if i+1 < n && (c.isOpCode(data[i+1]) || isUpperLetter(data[i+1])) {
						if i+1 < n && c.isOpCode(data[i+1]) {
							if paramBuf.Len() > 0 {
								params = append(params, paramBuf.String())
								paramBuf.Reset()
							}
							break
						}
						j := i + 1
						for j < n && isUpperLetter(data[j]) {
							j++
						}
						if j < n && c.isOpCode(data[j]) {
							paramBuf.WriteByte(data[i])
							i++
							if paramBuf.Len() > 0 {
								params = append(params, paramBuf.String())
								paramBuf.Reset()
							}
							break
						}
					}
					// 默认：将整个大写序列视为新操作边界
					if paramBuf.Len() > 0 {
						params = append(params, paramBuf.String())
						paramBuf.Reset()
					}
					break
				}
				// 大写字母后不跟操作码 = 参数（如在字符串末尾）
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

// EncodeBatch 将同一实体的多个操作合并为单个紧凑字符串。
// 实体短 ID 出现一次，后跟所有操作的拼接。
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

// Format 解码紧凑字符串并返回可读表示。
// 示例："Au10aB" -> "Entity A: move_up(10), attack(B)"
func (c *compactCodec) Format(data string) (string, error) {
	ops, err := c.Decode(data)
	if err != nil {
		return "", err
	}

	// 按实体分组操作
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

// isValidShortID 检查字符串是否为有效短 ID（全大写字母）。
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
