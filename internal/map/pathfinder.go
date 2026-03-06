package gamemap

import (
	"container/heap"
	"errors"
	"math"

	"gogame/internal/engine"
)

// Pathfinder errors.
var (
	ErrPathNotFound     = errors.New("no path found")
	ErrStartNotWalkable = errors.New("start position is not walkable")
	ErrEndNotWalkable   = errors.New("end position is not walkable")
	ErrNoLayerPath      = errors.New("no layer connection path available")
)

// Pathfinder finds paths on a Map, supporting multi-layer A* pathfinding.
type Pathfinder interface {
	FindPath(mapData *Map, start, end engine.Vector3, startLayer, endLayer int) ([]engine.Vector3, error)
}

// pathNode represents a node in the A* search graph.
type pathNode struct {
	x, z  int
	layer int
	g     float64 // cost from start
	h     float64 // heuristic to goal
	f     float64 // g + h
	parent *pathNode
	// If this node was reached via a layer connection, store the connection ID.
	viaConnection string
}

// nodeKey uniquely identifies a position in the search space.
type nodeKey struct {
	x, z, layer int
}

// priorityQueue implements heap.Interface for A* open set.
type priorityQueue []*pathNode

func (pq priorityQueue) Len() int            { return len(pq) }
func (pq priorityQueue) Less(i, j int) bool  { return pq[i].f < pq[j].f }
func (pq priorityQueue) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *priorityQueue) Push(x interface{}) { *pq = append(*pq, x.(*pathNode)) }
func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	*pq = old[:n-1]
	return item
}

// astarPathfinder implements Pathfinder using A* algorithm.
type astarPathfinder struct{}

// NewPathfinder creates a new A* pathfinder.
func NewPathfinder() Pathfinder {
	return &astarPathfinder{}
}

// FindPath finds a path from start to end, potentially across layers.
func (p *astarPathfinder) FindPath(mapData *Map, start, end engine.Vector3, startLayer, endLayer int) ([]engine.Vector3, error) {
	// Validate start and end positions.
	sx, sz := int(start.X), int(start.Z)
	ex, ez := int(end.X), int(end.Z)

	if !mapData.IsWalkable(startLayer, sx, sz) {
		return nil, ErrStartNotWalkable
	}
	if !mapData.IsWalkable(endLayer, ex, ez) {
		return nil, ErrEndNotWalkable
	}

	// If cross-layer, check that a connection path exists.
	if startLayer != endLayer {
		if !p.hasLayerConnection(mapData, startLayer, endLayer) {
			return nil, ErrNoLayerPath
		}
	}

	// A* search.
	startNode := &pathNode{
		x: sx, z: sz, layer: startLayer,
		g: 0,
		h: heuristic(sx, sz, startLayer, ex, ez, endLayer),
	}
	startNode.f = startNode.g + startNode.h

	openSet := &priorityQueue{startNode}
	heap.Init(openSet)

	closed := make(map[nodeKey]bool)
	gScores := make(map[nodeKey]float64)
	gScores[nodeKey{sx, sz, startLayer}] = 0

	// 4-directional movement.
	dirs := [][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}

	for openSet.Len() > 0 {
		current := heap.Pop(openSet).(*pathNode)
		key := nodeKey{current.x, current.z, current.layer}

		if current.x == ex && current.z == ez && current.layer == endLayer {
			return reconstructPath(current), nil
		}

		if closed[key] {
			continue
		}
		closed[key] = true

		// Explore neighbors on the same layer.
		for _, d := range dirs {
			nx, nz := current.x+d[0], current.z+d[1]
			if !mapData.IsWalkable(current.layer, nx, nz) {
				continue
			}
			nKey := nodeKey{nx, nz, current.layer}
			if closed[nKey] {
				continue
			}

			ng := current.g + 1.0
			if prev, exists := gScores[nKey]; exists && ng >= prev {
				continue
			}
			gScores[nKey] = ng

			neighbor := &pathNode{
				x: nx, z: nz, layer: current.layer,
				g: ng,
				h: heuristic(nx, nz, current.layer, ex, ez, endLayer),
				parent: current,
			}
			neighbor.f = neighbor.g + neighbor.h
			heap.Push(openSet, neighbor)
		}

		// Explore layer connections from current position.
		p.expandLayerConnections(mapData, current, openSet, closed, gScores, ex, ez, endLayer)
	}

	return nil, ErrPathNotFound
}

// expandLayerConnections adds neighbors reachable via layer connections.
func (p *astarPathfinder) expandLayerConnections(mapData *Map, current *pathNode, openSet *priorityQueue, closed map[nodeKey]bool, gScores map[nodeKey]float64, ex, ez, endLayer int) {
	layer, ok := mapData.GetLayer(current.layer)
	if !ok {
		return
	}

	for _, conn := range layer.Connections {
		// Check if current position is at the connection source.
		csx, csz := int(conn.SourcePos.X), int(conn.SourcePos.Z)
		if current.x != csx || current.z != csz {
			continue
		}

		tx, tz := int(conn.TargetPos.X), int(conn.TargetPos.Z)
		if !mapData.IsWalkable(conn.TargetLayer, tx, tz) {
			continue
		}

		nKey := nodeKey{tx, tz, conn.TargetLayer}
		if closed[nKey] {
			continue
		}

		// Layer transition cost = 2.0 (slightly more than a normal step).
		ng := current.g + 2.0
		if prev, exists := gScores[nKey]; exists && ng >= prev {
			continue
		}
		gScores[nKey] = ng

		neighbor := &pathNode{
			x: tx, z: tz, layer: conn.TargetLayer,
			g:             ng,
			h:             heuristic(tx, tz, conn.TargetLayer, ex, ez, endLayer),
			parent:        current,
			viaConnection: conn.ID,
		}
		neighbor.f = neighbor.g + neighbor.h
		heap.Push(openSet, neighbor)
	}
}

// hasLayerConnection checks if there's any connection path between two layers.
func (p *astarPathfinder) hasLayerConnection(mapData *Map, fromLayer, toLayer int) bool {
	// BFS to check reachability.
	visited := make(map[int]bool)
	queue := []int{fromLayer}
	visited[fromLayer] = true

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == toLayer {
			return true
		}

		layer, ok := mapData.GetLayer(current)
		if !ok {
			continue
		}
		for _, conn := range layer.Connections {
			if !visited[conn.TargetLayer] {
				visited[conn.TargetLayer] = true
				queue = append(queue, conn.TargetLayer)
			}
		}
	}
	return false
}

// heuristic estimates cost from (x,z,layer) to (ex,ez,endLayer).
func heuristic(x, z, layer, ex, ez, endLayer int) float64 {
	dx := math.Abs(float64(x - ex))
	dz := math.Abs(float64(z - ez))
	h := dx + dz // Manhattan distance
	if layer != endLayer {
		h += 2.0 // extra cost for layer transition
	}
	return h
}

// reconstructPath builds the path from goal back to start.
func reconstructPath(node *pathNode) []engine.Vector3 {
	var path []engine.Vector3
	for n := node; n != nil; n = n.parent {
		path = append(path, engine.Vector3{
			X: float32(n.x),
			Y: 0,
			Z: float32(n.z),
		})
	}
	// Reverse.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}
