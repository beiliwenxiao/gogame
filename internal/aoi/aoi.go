package aoi

import (
	"math"
	"sync"

	"gogame/internal/engine"
)

// AOIBoundaryZone 定义两个 AOI 管理器之间的边界区域。
type AOIBoundaryZone struct {
	LocalMin  engine.Vector3 // 本地边界区域最小坐标
	LocalMax  engine.Vector3 // 本地边界区域最大坐标
	RemoteMin engine.Vector3 // 远端边界区域最小坐标
	RemoteMax engine.Vector3 // 远端边界区域最大坐标
}

// CrossBoundaryEntity 保存跨边界可见实体的信息。
type CrossBoundaryEntity struct {
	EntityID engine.EntityID
	SourceID string         // 来源 AOI 管理器标识（本地为空，跨边界为 neighborID）
	Position engine.Vector3
}

// OcclusionData 保存用于遮挡计算的高度图和层级信息。
type OcclusionData struct {
	HeightMap  [][]float32 // 二维高度图，索引为 [z][x]
	LayerIndex int         // 该数据所属的地图层级
}

// AOIManager 管理场景中实体的兴趣区域（AOI）。
// 通过 SpatialIndex 跟踪实体位置，维护每个实体的可见实体列表，
// 并在可见性变化时触发进入/离开视野回调。
type AOIManager interface {
	Add(entityID engine.EntityID, pos engine.Vector3, aoiRadius float32)
	Remove(entityID engine.EntityID)
	UpdatePosition(entityID engine.EntityID, newPos engine.Vector3)
	GetVisibleEntities(entityID engine.EntityID) []engine.EntityID
	SetAOIRadius(entityID engine.EntityID, radius float32)
	OnEnterView(handler func(watcher, target engine.EntityID))
	OnLeaveView(handler func(watcher, target engine.EntityID))
	// 跨边界 AOI 支持
	RegisterNeighborAOI(neighborID string, neighbor AOIManager, boundaryZone *AOIBoundaryZone)
	UnregisterNeighborAOI(neighborID string)
	GetCrossBoundaryVisibleEntities(entityID engine.EntityID) []CrossBoundaryEntity
	GetBoundaryEntities(neighborID string) []engine.EntityID
	// 遮挡支持
	SetOcclusionData(data *OcclusionData)
	SetEntityLayer(entityID engine.EntityID, layerIndex int)
	IsOccluded(watcher, target engine.EntityID) bool
}

// aoiEntity 存储每个实体的 AOI 状态。
type aoiEntity struct {
	pos        engine.Vector3
	radius     float32
	visible    map[engine.EntityID]struct{} // 当前可见集合
	layerIndex int                          // 地图层级索引，用于遮挡计算
}

// neighborInfo 存储已注册的邻居 AOI 管理器及其边界区域。
type neighborInfo struct {
	manager      AOIManager
	boundaryZone *AOIBoundaryZone
}

// aoiManager 是 AOIManager 的具体实现。
type aoiManager struct {
	mu       sync.RWMutex
	spatial  SpatialIndex
	entities map[engine.EntityID]*aoiEntity

	enterHandlers []func(watcher, target engine.EntityID)
	leaveHandlers []func(watcher, target engine.EntityID)

	neighbors     map[string]*neighborInfo // neighborID → 邻居信息
	occlusionData *OcclusionData           // 用于遮挡检测的高度图
}

// NewAOIManager 创建由给定 SpatialIndex 支持的新 AOIManager。
func NewAOIManager(spatial SpatialIndex) AOIManager {
	return &aoiManager{
		spatial:   spatial,
		entities:  make(map[engine.EntityID]*aoiEntity),
		neighbors: make(map[string]*neighborInfo),
	}
}

// Add 在给定位置和半径处向 AOI 系统注册实体。
// 将实体插入空间索引并计算其初始可见集合。
func (m *aoiManager) Add(entityID engine.EntityID, pos engine.Vector3, aoiRadius float32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if aoiRadius <= 0 {
		aoiRadius = 1
	}

	// Insert into spatial index first so other queries can find this entity.
	m.spatial.Insert(entityID, pos)

	ent := &aoiEntity{
		pos:     pos,
		radius:  aoiRadius,
		visible: make(map[engine.EntityID]struct{}),
	}
	m.entities[entityID] = ent

	// Compute initial visible set for the new entity (what the new entity can see).
	nearby := m.spatial.QueryRange(pos, aoiRadius)
	for _, nid := range nearby {
		if nid == entityID {
			continue
		}
		if _, ok := m.entities[nid]; !ok {
			continue
		}
		ent.visible[nid] = struct{}{}
		m.fireEnterView(entityID, nid)
	}

	// Check all existing entities to see if any of them can now see the new entity.
	for eid, other := range m.entities {
		if eid == entityID {
			continue
		}
		if distSq(other.pos, pos) <= other.radius*other.radius {
			if _, already := other.visible[entityID]; !already {
				other.visible[entityID] = struct{}{}
				m.fireEnterView(eid, entityID)
			}
		}
	}
}

// Remove unregisters an entity from the AOI system.
// It fires leave-view events for all watchers that had this entity visible.
func (m *aoiManager) Remove(entityID engine.EntityID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ent, ok := m.entities[entityID]
	if !ok {
		return
	}

	// Notify all entities that currently see this entity.
	for otherID := range ent.visible {
		if other, exists := m.entities[otherID]; exists {
			if _, vis := other.visible[entityID]; vis {
				delete(other.visible, entityID)
				m.fireLeaveView(otherID, entityID)
			}
		}
	}

	// Notify this entity about losing sight of all visible entities.
	for otherID := range ent.visible {
		m.fireLeaveView(entityID, otherID)
	}

	m.spatial.Remove(entityID)
	delete(m.entities, entityID)
}

// UpdatePosition moves an entity to a new position and recalculates visibility.
func (m *aoiManager) UpdatePosition(entityID engine.EntityID, newPos engine.Vector3) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ent, ok := m.entities[entityID]
	if !ok {
		return
	}

	ent.pos = newPos
	m.spatial.Update(entityID, newPos)

	// Recalculate visible set for the moved entity.
	m.recalcVisible(entityID, ent)

	// Recalculate visible sets for entities that might be affected.
	// We query a generous range to find entities whose AOI might overlap.
	// Use the moved entity's radius as a baseline; other entities with larger
	// radii will also be checked since they queried us.
	m.recalcNearby(entityID, ent)
}

// GetVisibleEntities returns the list of entities currently visible to the given entity.
func (m *aoiManager) GetVisibleEntities(entityID engine.EntityID) []engine.EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ent, ok := m.entities[entityID]
	if !ok {
		return nil
	}

	result := make([]engine.EntityID, 0, len(ent.visible))
	for id := range ent.visible {
		result = append(result, id)
	}
	return result
}

// SetAOIRadius changes the AOI radius for an entity and recalculates visibility.
func (m *aoiManager) SetAOIRadius(entityID engine.EntityID, radius float32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ent, ok := m.entities[entityID]
	if !ok {
		return
	}

	if radius <= 0 {
		radius = 1
	}
	ent.radius = radius

	// Recalculate this entity's visible set with the new radius.
	m.recalcVisible(entityID, ent)
}

// OnEnterView registers a callback invoked when target enters watcher's view.
func (m *aoiManager) OnEnterView(handler func(watcher, target engine.EntityID)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enterHandlers = append(m.enterHandlers, handler)
}

// OnLeaveView registers a callback invoked when target leaves watcher's view.
func (m *aoiManager) OnLeaveView(handler func(watcher, target engine.EntityID)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.leaveHandlers = append(m.leaveHandlers, handler)
}

// ---------- Cross-boundary AOI methods ----------

// RegisterNeighborAOI registers a neighboring AOI manager with its boundary zone.
func (m *aoiManager) RegisterNeighborAOI(neighborID string, neighbor AOIManager, boundaryZone *AOIBoundaryZone) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.neighbors[neighborID] = &neighborInfo{
		manager:      neighbor,
		boundaryZone: boundaryZone,
	}
}

// UnregisterNeighborAOI removes a previously registered neighbor.
func (m *aoiManager) UnregisterNeighborAOI(neighborID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.neighbors, neighborID)
}

// GetCrossBoundaryVisibleEntities returns visible entities from both the local
// AOI manager and any registered neighbors whose boundary zones overlap.
func (m *aoiManager) GetCrossBoundaryVisibleEntities(entityID engine.EntityID) []CrossBoundaryEntity {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ent, ok := m.entities[entityID]
	if !ok {
		return nil
	}

	var result []CrossBoundaryEntity

	// Add local visible entities.
	for visID := range ent.visible {
		other, exists := m.entities[visID]
		if !exists {
			continue
		}
		result = append(result, CrossBoundaryEntity{
			EntityID: visID,
			SourceID: "", // local
			Position: other.pos,
		})
	}

	// Check if this entity is in any neighbor's local boundary zone.
	for nID, ni := range m.neighbors {
		if !inBoundaryZone(ent.pos, ni.boundaryZone.LocalMin, ni.boundaryZone.LocalMax) {
			continue
		}
		// Query the neighbor for entities in its remote (from our perspective) boundary zone.
		// The neighbor stores the boundary zone with LocalMin/LocalMax referring to its own space,
		// but from our side we registered the zone so that RemoteMin/RemoteMax is the neighbor's area.
		// GetBoundaryEntities on the neighbor returns entities in the neighbor's local boundary zone.
		// We need to call the neighbor with our neighborID so it can look up the right zone.
		// However, the neighbor may have registered us with a different ID.
		// The convention is: when A registers B as neighbor "B", B registers A as neighbor "A".
		// So we query the neighbor's boundary entities using the zone's remote bounds directly.
		remoteEntities := m.getRemoteBoundaryEntities(ni)
		for _, rid := range remoteEntities {
			result = append(result, CrossBoundaryEntity{
				EntityID: rid.EntityID,
				SourceID: nID,
				Position: rid.Position,
			})
		}
	}

	return result
}

// getRemoteBoundaryEntities queries a neighbor AOI manager for entities within
// the remote boundary zone. It accesses the neighbor's internal state if possible,
// otherwise falls back to the interface methods.
func (m *aoiManager) getRemoteBoundaryEntities(ni *neighborInfo) []CrossBoundaryEntity {
	// If the neighbor is also an aoiManager, we can access its entities directly.
	if mgr, ok := ni.manager.(*aoiManager); ok {
		mgr.mu.RLock()
		defer mgr.mu.RUnlock()

		var result []CrossBoundaryEntity
		for eid, ent := range mgr.entities {
			if inBoundaryZone(ent.pos, ni.boundaryZone.RemoteMin, ni.boundaryZone.RemoteMax) {
				result = append(result, CrossBoundaryEntity{
					EntityID: eid,
					Position: ent.pos,
				})
			}
		}
		return result
	}
	return nil
}

// GetBoundaryEntities returns entity IDs in the local boundary zone
// associated with the given neighborID.
func (m *aoiManager) GetBoundaryEntities(neighborID string) []engine.EntityID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ni, ok := m.neighbors[neighborID]
	if !ok {
		return nil
	}

	var result []engine.EntityID
	for eid, ent := range m.entities {
		if inBoundaryZone(ent.pos, ni.boundaryZone.LocalMin, ni.boundaryZone.LocalMax) {
			result = append(result, eid)
		}
	}
	return result
}

// inBoundaryZone checks if a position is within the axis-aligned bounding box
// defined by min and max coordinates.
func inBoundaryZone(pos, min, max engine.Vector3) bool {
	return pos.X >= min.X && pos.X <= max.X &&
		pos.Y >= min.Y && pos.Y <= max.Y &&
		pos.Z >= min.Z && pos.Z <= max.Z
}

// ---------- Occlusion methods ----------

// SetOcclusionData sets the height map data used for occlusion calculations.
func (m *aoiManager) SetOcclusionData(data *OcclusionData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.occlusionData = data
}

// SetEntityLayer sets the map layer index for an entity (used in occlusion checks).
func (m *aoiManager) SetEntityLayer(entityID engine.EntityID, layerIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ent, ok := m.entities[entityID]; ok {
		ent.layerIndex = layerIndex
	}
}

// IsOccluded checks if the target entity is occluded from the watcher's view
// by terrain height. Returns true if the target is hidden behind higher terrain.
//
// Occlusion logic: sample height values along the line from watcher to target.
// If any intermediate terrain height exceeds both the watcher's and target's
// effective height (position.Y + terrain height at their position), the target
// is considered occluded.
func (m *aoiManager) IsOccluded(watcher, target engine.EntityID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.occlusionData == nil {
		return false
	}

	watcherEnt, ok := m.entities[watcher]
	if !ok {
		return false
	}
	targetEnt, ok := m.entities[target]
	if !ok {
		return false
	}

	// Entities on different layers are not visible to each other
	// (cross-layer visibility is handled by cross-boundary AOI).
	if watcherEnt.layerIndex != targetEnt.layerIndex {
		return true
	}

	hm := m.occlusionData.HeightMap
	if len(hm) == 0 {
		return false
	}

	// Get effective heights (entity Y + terrain height at position).
	watcherH := watcherEnt.pos.Y + getTerrainHeight(hm, watcherEnt.pos.X, watcherEnt.pos.Z)
	targetH := targetEnt.pos.Y + getTerrainHeight(hm, targetEnt.pos.X, targetEnt.pos.Z)
	minH := watcherH
	if targetH < minH {
		minH = targetH
	}

	// Ray-march along the line from watcher to target, sampling terrain height.
	wx, wz := watcherEnt.pos.X, watcherEnt.pos.Z
	tx, tz := targetEnt.pos.X, targetEnt.pos.Z
	dx := tx - wx
	dz := tz - wz
	dist := float32(0)
	if dx != 0 || dz != 0 {
		dist = float32(math.Sqrt(float64(dx*dx + dz*dz)))
	}
	if dist < 2 {
		return false // too close, no occlusion
	}

	// Sample at regular intervals (every 1 unit).
	steps := int(dist)
	if steps > 100 {
		steps = 100 // cap for performance
	}
	for i := 1; i < steps; i++ {
		t := float32(i) / float32(steps)
		sx := wx + dx*t
		sz := wz + dz*t
		terrainH := getTerrainHeight(hm, sx, sz)
		if terrainH > minH {
			return true // terrain blocks line of sight
		}
	}

	return false
}

// getTerrainHeight samples the height map at a given (x, z) position.
func getTerrainHeight(hm [][]float32, x, z float32) float32 {
	iz := int(z)
	ix := int(x)
	if iz < 0 || iz >= len(hm) {
		return 0
	}
	if ix < 0 || ix >= len(hm[iz]) {
		return 0
	}
	return hm[iz][ix]
}

// ---------- internal helpers ----------

// recalcVisible recalculates the visible set for a single entity and fires
// enter/leave events as needed.
func (m *aoiManager) recalcVisible(entityID engine.EntityID, ent *aoiEntity) {
	nearby := m.spatial.QueryRange(ent.pos, ent.radius)

	newVisible := make(map[engine.EntityID]struct{}, len(nearby))
	for _, nid := range nearby {
		if nid == entityID {
			continue
		}
		if _, ok := m.entities[nid]; ok {
			newVisible[nid] = struct{}{}
		}
	}

	// Detect entities that left the view.
	for oldID := range ent.visible {
		if _, stillVisible := newVisible[oldID]; !stillVisible {
			m.fireLeaveView(entityID, oldID)
		}
	}

	// Detect entities that entered the view.
	for newID := range newVisible {
		if _, wasVisible := ent.visible[newID]; !wasVisible {
			m.fireEnterView(entityID, newID)
		}
	}

	ent.visible = newVisible
}

// recalcNearby recalculates whether other entities can see the moved entity.
// It uses the spatial index to find nearby candidates and also checks entities
// that previously had the moved entity in their visible set.
func (m *aoiManager) recalcNearby(movedID engine.EntityID, movedEnt *aoiEntity) {
	// Collect candidate entity IDs that need checking.
	// 1) Entities near the moved entity (could potentially see it now).
	// We need to query with the max radius any entity might have.
	// Use a cached max or scan — for correctness we track the max.
	maxRadius := m.maxAOIRadius()
	candidates := make(map[engine.EntityID]struct{})

	if maxRadius > 0 {
		nearby := m.spatial.QueryRange(movedEnt.pos, maxRadius)
		for _, nid := range nearby {
			if nid != movedID {
				candidates[nid] = struct{}{}
			}
		}
	}

	// 2) Entities that previously saw the moved entity (might need leave events).
	for eid, ent := range m.entities {
		if eid == movedID {
			continue
		}
		if _, had := ent.visible[movedID]; had {
			candidates[eid] = struct{}{}
		}
	}

	// Now check each candidate.
	for cid := range candidates {
		cent, ok := m.entities[cid]
		if !ok {
			continue
		}

		inRange := distSq(cent.pos, movedEnt.pos) <= cent.radius*cent.radius
		_, wasVisible := cent.visible[movedID]

		if inRange && !wasVisible {
			cent.visible[movedID] = struct{}{}
			m.fireEnterView(cid, movedID)
		} else if !inRange && wasVisible {
			delete(cent.visible, movedID)
			m.fireLeaveView(cid, movedID)
		}
	}
}

// maxAOIRadius returns the maximum AOI radius among all tracked entities.
func (m *aoiManager) maxAOIRadius() float32 {
	var max float32
	for _, e := range m.entities {
		if e.radius > max {
			max = e.radius
		}
	}
	return max
}

// fireEnterView invokes all registered enter-view handlers.
func (m *aoiManager) fireEnterView(watcher, target engine.EntityID) {
	for _, h := range m.enterHandlers {
		h(watcher, target)
	}
}

// fireLeaveView invokes all registered leave-view handlers.
func (m *aoiManager) fireLeaveView(watcher, target engine.EntityID) {
	for _, h := range m.leaveHandlers {
		h(watcher, target)
	}
}

// distSq returns the squared Euclidean distance between two points.
func distSq(a, b engine.Vector3) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return dx*dx + dy*dy + dz*dz
}
