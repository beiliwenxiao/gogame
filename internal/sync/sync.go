// Package sync 为 MMRPG 游戏引擎提供同步模块，
// 支持帧同步（锁步）和权威状态同步两种模式。
package sync

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"gogame/internal/engine"
	"gogame/internal/network"
)

// ---------------------------------------------------------------------------
// Syncer 接口 – 统一同步抽象
// ---------------------------------------------------------------------------

// Syncer 是统一的同步接口。上层通过此接口交互，
// 无需关心底层使用的是锁步同步还是状态同步。
type Syncer interface {
	Mode() engine.SyncMode
	OnInput(session network.Session, input engine.PlayerInput)
	OnTick(tick uint64)
	OnReconnect(session network.Session)
	// AddSession 向同步器注册客户端会话。
	AddSession(session network.Session)
	// RemoveSession 注销客户端会话。
	RemoveSession(sessionID string)
	// SessionCount 返回活跃会话数量。
	SessionCount() int
}

// LockstepSyncer 扩展 Syncer，提供锁步同步特有能力。
type LockstepSyncer interface {
	Syncer
	HistoryFrames() int
}

// StateSyncer 扩展 Syncer，提供状态同步特有能力。
type StateSyncer interface {
	Syncer
	SnapshotInterval() int
}

// ---------------------------------------------------------------------------
// SyncManagerConfig
// ---------------------------------------------------------------------------

// SyncManagerConfig 保存 SyncManager 的配置。
type SyncManagerConfig struct {
	DefaultMode         engine.SyncMode
	LockstepTimeout     time.Duration // 默认 100ms
	HistoryFrames       int           // 默认 300
	SnapshotInterval    int           // 默认 100
	AutoSwitchThreshold int           // 默认 50
}

// DefaultSyncManagerConfig 返回带有合理默认值的 SyncManagerConfig。
func DefaultSyncManagerConfig() SyncManagerConfig {
	return SyncManagerConfig{
		DefaultMode:         engine.SyncModeLockstep,
		LockstepTimeout:     100 * time.Millisecond,
		HistoryFrames:       300,
		SnapshotInterval:    100,
		AutoSwitchThreshold: 50,
	}
}

// ---------------------------------------------------------------------------
// FrameData – 单个锁步帧
// ---------------------------------------------------------------------------

// FrameData 表示包含所有玩家输入的单个锁步帧。
type FrameData struct {
	FrameID uint64               `json:"frame_id"`
	Inputs  []engine.PlayerInput `json:"inputs"`
}

// ---------------------------------------------------------------------------
// lockstepSyncer 实现
// ---------------------------------------------------------------------------

type lockstepSyncer struct {
	mu       sync.RWMutex
	timeout  time.Duration
	maxHist  int
	sessions map[string]network.Session // sessionID → Session

	// 每 tick 的输入收集
	pendingInputs []engine.PlayerInput

	// 历史环形缓冲区
	history    []FrameData
	histHead   int // 下一个写入位置
	histCount  int // 已存储帧数
	lastFrame  uint64
}

// NewLockstepSyncer 创建新的锁步同步器。
func NewLockstepSyncer(timeout time.Duration, historyFrames int) LockstepSyncer {
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	if historyFrames <= 0 {
		historyFrames = 300
	}
	return &lockstepSyncer{
		timeout:  timeout,
		maxHist:  historyFrames,
		sessions: make(map[string]network.Session),
		history:  make([]FrameData, historyFrames),
	}
}

func (ls *lockstepSyncer) Mode() engine.SyncMode { return engine.SyncModeLockstep }

func (ls *lockstepSyncer) HistoryFrames() int { return ls.maxHist }

func (ls *lockstepSyncer) AddSession(session network.Session) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.sessions[session.ID()] = session
}

func (ls *lockstepSyncer) RemoveSession(sessionID string) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	delete(ls.sessions, sessionID)
}

func (ls *lockstepSyncer) SessionCount() int {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return len(ls.sessions)
}

// OnInput 收集当前 tick 的玩家输入。
func (ls *lockstepSyncer) OnInput(session network.Session, input engine.PlayerInput) {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	ls.pendingInputs = append(ls.pendingInputs, input)
}

// OnTick 将收集到的所有输入打包为一帧，存入历史记录，
// 并广播给所有会话。
func (ls *lockstepSyncer) OnTick(tick uint64) {
	ls.mu.Lock()

	// 从收集的输入构建帧数据
	frame := FrameData{
		FrameID: tick,
		Inputs:  ls.pendingInputs,
	}
	if frame.Inputs == nil {
		frame.Inputs = []engine.PlayerInput{}
	}

	// 存入环形缓冲区
	ls.history[ls.histHead] = frame
	ls.histHead = (ls.histHead + 1) % ls.maxHist
	if ls.histCount < ls.maxHist {
		ls.histCount++
	}
	ls.lastFrame = tick

	// 重置下一 tick 的待处理输入
	ls.pendingInputs = nil

	// 快照会话列表用于广播（避免持锁期间进行 I/O）
	sessions := make([]network.Session, 0, len(ls.sessions))
	for _, s := range ls.sessions {
		sessions = append(sessions, s)
	}
	ls.mu.Unlock()

	// 序列化并广播
	data, err := json.Marshal(frame)
	if err != nil {
		log.Printf("[sync] lockstep: failed to marshal frame %d: %v", tick, err)
		return
	}
	for _, s := range sessions {
		if sendErr := s.Send(data); sendErr != nil {
			log.Printf("[sync] lockstep: failed to send frame %d to session %s: %v", tick, s.ID(), sendErr)
		}
	}
}

// OnReconnect 向重连会话发送所有缺失的历史帧。
func (ls *lockstepSyncer) OnReconnect(session network.Session) {
	ls.mu.RLock()
	frames := ls.getHistoryFramesLocked()
	ls.mu.RUnlock()

	for _, frame := range frames {
		data, err := json.Marshal(frame)
		if err != nil {
			log.Printf("[sync] lockstep: reconnect marshal error: %v", err)
			continue
		}
		if sendErr := session.Send(data); sendErr != nil {
			log.Printf("[sync] lockstep: reconnect send error to %s: %v", session.ID(), sendErr)
			return
		}
	}
}

// getHistoryFramesLocked 按时间顺序返回已存储的历史帧。
// 调用方必须至少持有读锁。
func (ls *lockstepSyncer) getHistoryFramesLocked() []FrameData {
	if ls.histCount == 0 {
		return nil
	}
	result := make([]FrameData, 0, ls.histCount)
	start := (ls.histHead - ls.histCount + ls.maxHist) % ls.maxHist
	for i := 0; i < ls.histCount; i++ {
		idx := (start + i) % ls.maxHist
		result = append(result, ls.history[idx])
	}
	return result
}

// GetHistoryFrom 返回从给定帧 ID（含）开始的历史帧。
// 用于重连时只发送缺失的帧。
func (ls *lockstepSyncer) GetHistoryFrom(fromFrame uint64) []FrameData {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	all := ls.getHistoryFramesLocked()
	for i, f := range all {
		if f.FrameID >= fromFrame {
			return all[i:]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// EntityState – 用于增量计算的实体状态跟踪
// ---------------------------------------------------------------------------

// EntityState 表示某时刻单个实体的状态。
type EntityState struct {
	EntityID   engine.EntityID    `json:"entity_id"`
	Properties map[string]float64 `json:"properties"`
}

// Clone 返回 EntityState 的深拷贝。
func (es EntityState) Clone() EntityState {
	props := make(map[string]float64, len(es.Properties))
	for k, v := range es.Properties {
		props[k] = v
	}
	return EntityState{
		EntityID:   es.EntityID,
		Properties: props,
	}
}

// EntityDelta 表示两个 tick 之间单个实体的变化。
type EntityDelta struct {
	EntityID engine.EntityID    `json:"entity_id"`
	Changed  map[string]float64 `json:"changed,omitempty"`
	Removed  []string           `json:"removed,omitempty"`
}

// StateDelta 表示单个 tick 中的所有变化。
type StateDelta struct {
	Tick           uint64        `json:"tick"`
	EntityDeltas   []EntityDelta `json:"entity_deltas,omitempty"`
	RemovedEntities []engine.EntityID `json:"removed_entities,omitempty"`
}

// StateSnapshot 表示给定 tick 时的完整状态快照。
type StateSnapshot struct {
	Tick     uint64        `json:"tick"`
	Entities []EntityState `json:"entities"`
}

// ---------------------------------------------------------------------------
// AOIProvider – AOI 过滤接口
// ---------------------------------------------------------------------------

// AOIProvider 是 StateSyncer 用于过滤哪些实体对给定会话可见的可选接口。
type AOIProvider interface {
	// GetVisibleEntities 返回给定实体可见的实体 ID 列表。
	GetVisibleEntities(entityID engine.EntityID) []engine.EntityID
}

// ---------------------------------------------------------------------------
// stateSyncer 实现
// ---------------------------------------------------------------------------

type stateSyncer struct {
	mu               sync.RWMutex
	snapshotInterval int
	sessions         map[string]network.Session
	sessionEntities  map[string]engine.EntityID // sessionID → 该会话控制的实体

	// 实体状态跟踪
	currentState map[engine.EntityID]EntityState
	prevState    map[engine.EntityID]EntityState

	// 快照存储
	lastSnapshot   *StateSnapshot
	lastSnapshotTick uint64

	// 增量历史（用于重连）
	deltaHistory []StateDelta
	maxDeltas    int // 保留自上次快照以来的增量 + 缓冲

	// AOI 提供者（可选）
	aoiProvider AOIProvider
}

// NewStateSyncer 创建新的状态同步器。
func NewStateSyncer(snapshotInterval int, aoiProvider AOIProvider) StateSyncer {
	if snapshotInterval <= 0 {
		snapshotInterval = 100
	}
	return &stateSyncer{
		snapshotInterval: snapshotInterval,
		sessions:         make(map[string]network.Session),
		sessionEntities:  make(map[string]engine.EntityID),
		currentState:     make(map[engine.EntityID]EntityState),
		prevState:        make(map[engine.EntityID]EntityState),
		maxDeltas:        snapshotInterval * 2,
		aoiProvider:      aoiProvider,
	}
}

func (ss *stateSyncer) Mode() engine.SyncMode { return engine.SyncModeState }

func (ss *stateSyncer) SnapshotInterval() int { return ss.snapshotInterval }

func (ss *stateSyncer) AddSession(session network.Session) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.sessions[session.ID()] = session
}

func (ss *stateSyncer) RemoveSession(sessionID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.sessions, sessionID)
	delete(ss.sessionEntities, sessionID)
}

func (ss *stateSyncer) SessionCount() int {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return len(ss.sessions)
}

// SetSessionEntity 将会话与其控制的实体关联（用于 AOI 过滤）。
func (ss *stateSyncer) SetSessionEntity(sessionID string, entityID engine.EntityID) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.sessionEntities[sessionID] = entityID
}

// SetEntityState 更新实体的权威状态。
// 应在每个 tick 由游戏逻辑调用。
func (ss *stateSyncer) SetEntityState(entityID engine.EntityID, props map[string]float64) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.currentState[entityID] = EntityState{
		EntityID:   entityID,
		Properties: props,
	}
}

// RemoveEntity 从状态跟踪中移除实体。
func (ss *stateSyncer) RemoveEntity(entityID engine.EntityID) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.currentState, entityID)
}

// OnInput 处理玩家输入（状态同步在服务端运行权威逻辑，
// 因此输入被排队等待游戏逻辑处理）。
func (ss *stateSyncer) OnInput(session network.Session, input engine.PlayerInput) {
	// 状态同步中，输入由权威游戏逻辑处理。
	// 同步器仅确认接收；实际处理在游戏循环中进行。
}

// OnTick 计算增量，可选地生成快照，并向会话发送更新。
func (ss *stateSyncer) OnTick(tick uint64) {
	ss.mu.Lock()

	// 计算前后状态的增量
	delta := ss.computeDeltaLocked(tick)

	// 将增量存入历史
	ss.deltaHistory = append(ss.deltaHistory, delta)
	if len(ss.deltaHistory) > ss.maxDeltas {
		ss.deltaHistory = ss.deltaHistory[1:]
	}

	// 检查是否需要完整快照
	needSnapshot := tick > 0 && tick%uint64(ss.snapshotInterval) == 0
	var snapshot *StateSnapshot
	if needSnapshot {
		snapshot = ss.takeSnapshotLocked(tick)
		ss.lastSnapshot = snapshot
		ss.lastSnapshotTick = tick
		// 裁剪增量历史：只保留快照之后的增量
		ss.trimDeltaHistoryLocked(tick)
	}

	// 将 prevState 更新为当前状态，用于下一 tick 的增量计算
	ss.prevState = make(map[engine.EntityID]EntityState, len(ss.currentState))
	for id, state := range ss.currentState {
		ss.prevState[id] = state.Clone()
	}

	// 快照会话及其实体映射，用于发送
	infos := make([]syncSessionInfo, 0, len(ss.sessions))
	for sid, s := range ss.sessions {
		eid, has := ss.sessionEntities[sid]
		infos = append(infos, syncSessionInfo{session: s, entityID: eid, hasEntity: has})
	}

	ss.mu.Unlock()

	// 向每个会话发送快照或增量
	if needSnapshot && snapshot != nil {
		ss.broadcastSnapshot(snapshot, infos)
	} else if len(delta.EntityDeltas) > 0 || len(delta.RemovedEntities) > 0 {
		ss.broadcastDelta(&delta, infos)
	}
}

// computeDeltaLocked 计算状态增量。调用方必须持有锁。
func (ss *stateSyncer) computeDeltaLocked(tick uint64) StateDelta {
	delta := StateDelta{Tick: tick}

	// 检查变化/新增实体
	for id, cur := range ss.currentState {
		prev, existed := ss.prevState[id]
		if !existed {
			// 新实体 – 所有属性均为"变化"
			ed := EntityDelta{
				EntityID: id,
				Changed:  make(map[string]float64, len(cur.Properties)),
			}
			for k, v := range cur.Properties {
				ed.Changed[k] = v
			}
			delta.EntityDeltas = append(delta.EntityDeltas, ed)
			continue
		}

		// 已有实体 – 查找属性变化
		ed := EntityDelta{EntityID: id}
		for k, v := range cur.Properties {
			if oldV, ok := prev.Properties[k]; !ok || oldV != v {
				if ed.Changed == nil {
					ed.Changed = make(map[string]float64)
				}
				ed.Changed[k] = v
			}
		}
		// 查找已移除的属性
		for k := range prev.Properties {
			if _, ok := cur.Properties[k]; !ok {
				ed.Removed = append(ed.Removed, k)
			}
		}
		if len(ed.Changed) > 0 || len(ed.Removed) > 0 {
			delta.EntityDeltas = append(delta.EntityDeltas, ed)
		}
	}

	// 检查已移除实体
	for id := range ss.prevState {
		if _, exists := ss.currentState[id]; !exists {
			delta.RemovedEntities = append(delta.RemovedEntities, id)
		}
	}

	return delta
}

// takeSnapshotLocked 创建完整状态快照。调用方必须持有锁。
func (ss *stateSyncer) takeSnapshotLocked(tick uint64) *StateSnapshot {
	snap := &StateSnapshot{
		Tick:     tick,
		Entities: make([]EntityState, 0, len(ss.currentState)),
	}
	for _, state := range ss.currentState {
		snap.Entities = append(snap.Entities, state.Clone())
	}
	return snap
}

// trimDeltaHistoryLocked 移除早于快照 tick 的增量。
func (ss *stateSyncer) trimDeltaHistoryLocked(snapshotTick uint64) {
	trimIdx := 0
	for i, d := range ss.deltaHistory {
		if d.Tick > snapshotTick {
			trimIdx = i
			break
		}
		trimIdx = i + 1
	}
	if trimIdx > 0 && trimIdx <= len(ss.deltaHistory) {
		ss.deltaHistory = ss.deltaHistory[trimIdx:]
	}
}

// syncSessionInfo 保存广播操作所需的会话信息。
type syncSessionInfo struct {
	session   network.Session
	entityID  engine.EntityID
	hasEntity bool
}

func (ss *stateSyncer) broadcastSnapshot(snapshot *StateSnapshot, infos []syncSessionInfo) {
	for _, info := range infos {
		filtered := ss.filterForSession(snapshot, info.entityID, info.hasEntity)
		data, err := json.Marshal(filtered)
		if err != nil {
			log.Printf("[sync] state: snapshot marshal error: %v", err)
			continue
		}
		if sendErr := info.session.Send(data); sendErr != nil {
			log.Printf("[sync] state: snapshot send error to %s: %v", info.session.ID(), sendErr)
		}
	}
}

func (ss *stateSyncer) broadcastDelta(delta *StateDelta, infos []syncSessionInfo) {
	for _, info := range infos {
		filtered := ss.filterDeltaForSession(delta, info.entityID, info.hasEntity)
		if len(filtered.EntityDeltas) == 0 && len(filtered.RemovedEntities) == 0 {
			continue
		}
		data, err := json.Marshal(filtered)
		if err != nil {
			log.Printf("[sync] state: delta marshal error: %v", err)
			continue
		}
		if sendErr := info.session.Send(data); sendErr != nil {
			log.Printf("[sync] state: delta send error to %s: %v", info.session.ID(), sendErr)
		}
	}
}

// filterForSession 过滤快照，只包含对该会话实体可见的实体。
func (ss *stateSyncer) filterForSession(snapshot *StateSnapshot, entityID engine.EntityID, hasEntity bool) *StateSnapshot {
	if !hasEntity || ss.aoiProvider == nil {
		return snapshot // 无 AOI 过滤
	}
	visible := ss.getVisibleSet(entityID)
	filtered := &StateSnapshot{
		Tick:     snapshot.Tick,
		Entities: make([]EntityState, 0),
	}
	for _, es := range snapshot.Entities {
		if visible[es.EntityID] || es.EntityID == entityID {
			filtered.Entities = append(filtered.Entities, es)
		}
	}
	return filtered
}

// filterDeltaForSession 过滤增量，只包含对该会话实体可见的实体。
func (ss *stateSyncer) filterDeltaForSession(delta *StateDelta, entityID engine.EntityID, hasEntity bool) StateDelta {
	if !hasEntity || ss.aoiProvider == nil {
		return *delta // 无 AOI 过滤
	}
	visible := ss.getVisibleSet(entityID)
	filtered := StateDelta{Tick: delta.Tick}
	for _, ed := range delta.EntityDeltas {
		if visible[ed.EntityID] || ed.EntityID == entityID {
			filtered.EntityDeltas = append(filtered.EntityDeltas, ed)
		}
	}
	for _, rid := range delta.RemovedEntities {
		if visible[rid] || rid == entityID {
			filtered.RemovedEntities = append(filtered.RemovedEntities, rid)
		}
	}
	return filtered
}

func (ss *stateSyncer) getVisibleSet(entityID engine.EntityID) map[engine.EntityID]bool {
	visible := make(map[engine.EntityID]bool)
	if ss.aoiProvider != nil {
		for _, eid := range ss.aoiProvider.GetVisibleEntities(entityID) {
			visible[eid] = true
		}
	}
	return visible
}

// OnReconnect 发送最新快照及后续增量以恢复状态。
func (ss *stateSyncer) OnReconnect(session network.Session) {
	ss.mu.RLock()
	snapshot := ss.lastSnapshot
	deltas := make([]StateDelta, len(ss.deltaHistory))
	copy(deltas, ss.deltaHistory)
	ss.mu.RUnlock()

	// 先发送快照
	if snapshot != nil {
		data, err := json.Marshal(snapshot)
		if err != nil {
			log.Printf("[sync] state: reconnect snapshot marshal error: %v", err)
			return
		}
		if sendErr := session.Send(data); sendErr != nil {
			log.Printf("[sync] state: reconnect snapshot send error: %v", sendErr)
			return
		}
	}

	// 再发送后续增量
	for _, d := range deltas {
		data, err := json.Marshal(d)
		if err != nil {
			log.Printf("[sync] state: reconnect delta marshal error: %v", err)
			continue
		}
		if sendErr := session.Send(data); sendErr != nil {
			log.Printf("[sync] state: reconnect delta send error: %v", sendErr)
			return
		}
	}
}

// ApplyDelta 将 StateDelta 应用到 EntityState 映射，生成下一个状态。
// 这是一个纯函数，用于测试增量正确性。
func ApplyDelta(state map[engine.EntityID]EntityState, delta StateDelta) map[engine.EntityID]EntityState {
	result := make(map[engine.EntityID]EntityState, len(state))
	for id, es := range state {
		result[id] = es.Clone()
	}

	// 应用实体增量
	for _, ed := range delta.EntityDeltas {
		es, exists := result[ed.EntityID]
		if !exists {
			es = EntityState{
				EntityID:   ed.EntityID,
				Properties: make(map[string]float64),
			}
		}
		for k, v := range ed.Changed {
			es.Properties[k] = v
		}
		for _, k := range ed.Removed {
			delete(es.Properties, k)
		}
		result[ed.EntityID] = es
	}

	// 移除实体
	for _, rid := range delta.RemovedEntities {
		delete(result, rid)
	}

	return result
}

// ---------------------------------------------------------------------------
// SyncManager – 每个房间的同步器工厂和管理器
// ---------------------------------------------------------------------------

// SyncManager 管理每个房间的同步实例。
type SyncManager struct {
	mu      sync.RWMutex
	config  SyncManagerConfig
	syncers map[string]Syncer // roomID → Syncer
}

// NewSyncManager 使用给定配置创建新的 SyncManager。
func NewSyncManager(config SyncManagerConfig) *SyncManager {
	if config.LockstepTimeout <= 0 {
		config.LockstepTimeout = 100 * time.Millisecond
	}
	if config.HistoryFrames <= 0 {
		config.HistoryFrames = 300
	}
	if config.SnapshotInterval <= 0 {
		config.SnapshotInterval = 100
	}
	if config.AutoSwitchThreshold <= 0 {
		config.AutoSwitchThreshold = 50
	}
	return &SyncManager{
		config:  config,
		syncers: make(map[string]Syncer),
	}
}

// CreateSyncer 为给定房间创建并注册同步器。
// 若未指定模式（负值），则使用配置中的默认模式。
func (sm *SyncManager) CreateSyncer(roomID string, mode engine.SyncMode, aoiProvider AOIProvider) (Syncer, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.syncers[roomID]; exists {
		return nil, fmt.Errorf("syncer already exists for room %s", roomID)
	}

	var syncer Syncer
	switch mode {
	case engine.SyncModeLockstep:
		syncer = NewLockstepSyncer(sm.config.LockstepTimeout, sm.config.HistoryFrames)
	case engine.SyncModeState:
		syncer = NewStateSyncer(sm.config.SnapshotInterval, aoiProvider)
	default:
		return nil, fmt.Errorf("unknown sync mode: %d", mode)
	}

	sm.syncers[roomID] = syncer
	return syncer, nil
}

// GetSyncer 返回给定房间的同步器。
func (sm *SyncManager) GetSyncer(roomID string) (Syncer, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.syncers[roomID]
	return s, ok
}

// RemoveSyncer 移除并返回给定房间的同步器。
func (sm *SyncManager) RemoveSyncer(roomID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.syncers, roomID)
}

// CheckThreshold 检查房间的会话数是否超过自动切换阈值，
// 若超过则记录警告日志。
func (sm *SyncManager) CheckThreshold(roomID string) {
	sm.mu.RLock()
	syncer, ok := sm.syncers[roomID]
	sm.mu.RUnlock()
	if !ok {
		return
	}

	count := syncer.SessionCount()
	if count > sm.config.AutoSwitchThreshold && syncer.Mode() == engine.SyncModeLockstep {
		log.Printf("[WARN] sync: room %s has %d clients (threshold: %d), consider switching to State_Sync mode",
			roomID, count, sm.config.AutoSwitchThreshold)
	}
}

// RoomCount 返回 SyncManager 管理的房间数量。
func (sm *SyncManager) RoomCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.syncers)
}

// Config 返回当前 SyncManager 配置。
func (sm *SyncManager) Config() SyncManagerConfig {
	return sm.config
}
