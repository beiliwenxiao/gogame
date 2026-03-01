// Package aoi provides Area of Interest management for the MMRPG game engine,
// including spatial indexing and visibility tracking.
package aoi

import (
	"math"
	"sync"

	"gfgame/internal/engine"
)

// SpatialIndex provides spatial queries over entities positioned in 3D space.
type SpatialIndex interface {
	Insert(entityID engine.EntityID, pos engine.Vector3)
	Remove(entityID engine.EntityID)
	Update(entityID engine.EntityID, newPos engine.Vector3)
	QueryRange(center engine.Vector3, radius float32) []engine.EntityID
}

// GridConfig holds configuration for the grid-based spatial index.
type GridConfig struct {
	CellSize float32 // Width/height of each grid cell (default 100).
}

// DefaultGridConfig returns a GridConfig with sensible defaults.
func DefaultGridConfig() GridConfig {
	return GridConfig{CellSize: 100}
}

// cellKey identifies a single cell in the 2D grid (using X/Z plane).
type cellKey struct {
	cx, cz int
}

// entityEntry stores the current position and cell of an entity.
type entityEntry struct {
	pos  engine.Vector3
	cell cellKey
}

// GridSpatialIndex is a grid-based (九宫格) spatial index.
// It divides the world into uniform cells on the X-Z plane and tracks which
// entities reside in each cell. Range queries inspect only the cells that
// overlap the query circle, making them efficient for large worlds.
type GridSpatialIndex struct {
	mu       sync.RWMutex
	cellSize float32
	cells    map[cellKey]map[engine.EntityID]struct{} // cell → set of entities
	entities map[engine.EntityID]*entityEntry            // entity → position + cell
}

// NewGridSpatialIndex creates a new grid-based spatial index.
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

// posToCell converts a world position to the grid cell key (X-Z plane).
func (g *GridSpatialIndex) posToCell(pos engine.Vector3) cellKey {
	return cellKey{
		cx: int(math.Floor(float64(pos.X) / float64(g.cellSize))),
		cz: int(math.Floor(float64(pos.Z) / float64(g.cellSize))),
	}
}

// Insert adds an entity at the given position.
func (g *GridSpatialIndex) Insert(entityID engine.EntityID, pos engine.Vector3) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// If already present, remove first to avoid stale references.
	if old, ok := g.entities[entityID]; ok {
		g.removeCellEntry(old.cell, entityID)
	}

	ck := g.posToCell(pos)
	g.entities[entityID] = &entityEntry{pos: pos, cell: ck}
	g.addCellEntry(ck, entityID)
}

// Remove deletes an entity from the index.
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

// Update moves an entity to a new position, updating its cell if needed.
func (g *GridSpatialIndex) Update(entityID engine.EntityID, newPos engine.Vector3) {
	g.mu.Lock()
	defer g.mu.Unlock()

	entry, ok := g.entities[entityID]
	if !ok {
		// Entity not tracked yet — treat as insert.
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

// QueryRange returns all entity IDs within the given radius from center
// (Euclidean distance on the X-Z plane; Y is also considered).
func (g *GridSpatialIndex) QueryRange(center engine.Vector3, radius float32) []engine.EntityID {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if radius <= 0 {
		return nil
	}

	r64 := float64(radius)
	r2 := r64 * r64

	// Determine the range of cells that could overlap the query circle.
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

// ---------- internal helpers ----------

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
