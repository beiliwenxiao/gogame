// Package aoi 为 MMRPG 游戏引擎提供兴趣区域（AOI）管理，
// 包括空间索引和可见性跟踪。
package aoi

import (
	"math"
	"sync"

	"gfgame/internal/engine"
)

// SpatialIndex 提供对三维空间中实体位置的空间查询。
type SpatialIndex interface {
	Insert(entityID engine.EntityID, pos engine.Vector3)
	Remove(entityID engine.EntityID)
	Update(entityID engine.EntityID, newPos engine.Vector3)
	QueryRange(center engine.Vector3, radius float32) []engine.EntityID
}

// GridConfig 保存基于网格的空间索引配置。
type GridConfig struct {
	CellSize float32 // 每个网格单元的宽/高（默认 100）。
}

// DefaultGridConfig 返回带有合理默认值的 GridConfig。
func DefaultGridConfig() GridConfig {
	return GridConfig{CellSize: 100}
}

// cellKey 标识二维网格中的单个单元（使用 X/Z 平面）。
type cellKey struct {
	cx, cz int
}

// entityEntry 存储实体的当前位置和所在单元。
type entityEntry struct {
	pos  engine.Vector3
	cell cellKey
}

// GridSpatialIndex 是基于网格（九宫格）的空间索引。
// 它将世界划分为 X-Z 平面上的均匀单元，并跟踪每个单元中的实体。
// 范围查询只检查与查询圆重叠的单元，在大型世界中效率较高。
type GridSpatialIndex struct {
	mu       sync.RWMutex
	cellSize float32
	cells    map[cellKey]map[engine.EntityID]struct{} // 单元 → 实体集合
	entities map[engine.EntityID]*entityEntry            // 实体 → 位置 + 单元
}

// NewGridSpatialIndex 创建新的基于网格的空间索引。
func NewGridSpatialIndex(cfg GridConfig) *GridSpatialIndex {
	cs := cfg.CellSize
	if cs <= 0 {
		cs = DefaultGridConfig().CellSize
	}
	return &GridSpatialIndex{
		cellSize: cs,
		cells:    make(map[cellKey]map[engine.EntityID]struct{}),
		entities: make(map[engine.EntityID]*entityEntry),
	}
}

// posToCell 将世界坐标转换为网格单元键（X-Z 平面）。
func (g *GridSpatialIndex) posToCell(pos engine.Vector3) cellKey {
	return cellKey{
		cx: int(math.Floor(float64(pos.X) / float64(g.cellSize))),
		cz: int(math.Floor(float64(pos.Z) / float64(g.cellSize))),
	}
}

// Insert 在给定位置添加实体。
func (g *GridSpatialIndex) Insert(entityID engine.EntityID, pos engine.Vector3) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// 若已存在，先移除以避免过期引用。
	if old, ok := g.entities[entityID]; ok {
		g.removeCellEntry(old.cell, entityID)
	}

	ck := g.posToCell(pos)
	g.entities[entityID] = &entityEntry{pos: pos, cell: ck}
	g.addCellEntry(ck, entityID)
}

// Remove 从索引中删除实体。
func (g *GridSpatialIndex) Remove(entityID engine.EntityID) {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.entities[entityID]
	if !ok {
		return
	}
	g.removeCellEntry(entry.cell, entityID)
	delete(g.entities, entityID)
}

// Update 将实体移动到新位置，必要时更新其所在单元。
func (g *GridSpatialIndex) Update(entityID engine.EntityID, newPos engine.Vector3) {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.entities[entityID]
	if !ok {
		// 实体尚未跟踪 — 视为插入。
		ck := g.posToCell(newPos)
		g.entities[entityID] = &entityEntry{pos: newPos, cell: ck}
		g.addCellEntry(ck, entityID)
		return
	}

	newCell := g.posToCell(newPos)
	entry.pos = newPos

	if newCell != entry.cell {
		g.removeCellEntry(entry.cell, entityID)
		entry.cell = newCell
		g.addCellEntry(newCell, entityID)
	}
}

// QueryRange 返回距中心点给定半径内的所有实体 ID
// （X-Z 平面上的欧几里得距离；Y 轴也纳入计算）。
func (g *GridSpatialIndex) QueryRange(center engine.Vector3, radius float32) []engine.EntityID {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if radius <= 0 {
		return nil
	}

	r64 := float64(radius)
	r2 := r64 * r64

	// 确定可能与查询圆重叠的单元范围。
	minCX := int(math.Floor((float64(center.X) - r64) / float64(g.cellSize)))
	maxCX := int(math.Floor((float64(center.X) + r64) / float64(g.cellSize)))
	minCZ := int(math.Floor((float64(center.Z) - r64) / float64(g.cellSize)))
	maxCZ := int(math.Floor((float64(center.Z) + r64) / float64(g.cellSize)))

	var result []engine.EntityID

	cx64, cy64, cz64 := float64(center.X), float64(center.Y), float64(center.Z)

	for x := minCX; x <= maxCX; x++ {
		for z := minCZ; z <= maxCZ; z++ {
			bucket, ok := g.cells[cellKey{cx: x, cz: z}]
			if !ok {
				continue
			}
			for eid := range bucket {
				entry := g.entities[eid]
				dx := float64(entry.pos.X) - cx64
				dy := float64(entry.pos.Y) - cy64
				dz := float64(entry.pos.Z) - cz64
				if dx*dx+dy*dy+dz*dz <= r2 {
					result = append(result, eid)
				}
			}
		}
	}
	return result
}

// ---------- 内部辅助函数 ----------

func (g *GridSpatialIndex) addCellEntry(ck cellKey, eid engine.EntityID) {
	bucket, ok := g.cells[ck]
	if !ok {
		bucket = make(map[engine.EntityID]struct{})
		g.cells[ck] = bucket
	}
	bucket[eid] = struct{}{}
}

func (g *GridSpatialIndex) removeCellEntry(ck cellKey, eid engine.EntityID) {
	bucket, ok := g.cells[ck]
	if !ok {
		return
	}
	delete(bucket, eid)
	if len(bucket) == 0 {
		delete(g.cells, ck)
	}
}
