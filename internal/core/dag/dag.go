package dag

import (
	"errors"

	"github.com/samber/lo/mutable"
)

var (
	ErrCycleDetected = errors.New("cycle detected in graph")
)

type Edge[Id comparable] interface {
	GetFromId() Id
	GetToId() Id
}

type Node[Id comparable] interface {
	GetId() Id
}

type Dagger[Id comparable, N Node[Id], E Edge[Id]] interface {
	GetNodes() []N
	GetEdges() []E
}

// HasCycle detects if the graph contains a cycle using DFS
func HasCycle[Id comparable, N Node[Id], E Edge[Id]](dagger Dagger[Id, N, E]) bool {
	// Build adjacency list
	adjacency := make(map[Id][]Id)
	for _, edge := range dagger.GetEdges() {
		from := edge.GetFromId()
		to := edge.GetToId()
		adjacency[from] = append(adjacency[from], to)
	}

	// Track visited nodes and nodes in current path (for cycle detection)
	visited := make(map[Id]bool)
	inPath := make(map[Id]bool)

	// DFS function to detect cycle
	var hasCycleDFS func(Id) bool
	hasCycleDFS = func(nodeId Id) bool {
		if inPath[nodeId] {
			// Found a back edge - cycle detected
			return true
		}
		if visited[nodeId] {
			// Already processed this node
			return false
		}

		visited[nodeId] = true
		inPath[nodeId] = true

		// Visit all neighbors
		for _, neighbor := range adjacency[nodeId] {
			if hasCycleDFS(neighbor) {
				return true
			}
		}

		// Remove from current path when backtracking
		inPath[nodeId] = false
		return false
	}

	// Check all nodes (graph might be disconnected)
	for _, node := range dagger.GetNodes() {
		if !visited[node.GetId()] {
			if hasCycleDFS(node.GetId()) {
				return true
			}
		}
	}

	return false
}

func IsDagBy[Id comparable, N Node[Id], E Edge[Id]](dagger Dagger[Id, N, E], f func(N) bool) bool {
	for _, node := range dagger.GetNodes() {
		if !f(node) {
			return false
		}
	}
	return true
}

func FilterNodes[Id comparable, N Node[Id], E Edge[Id]](dagger Dagger[Id, N, E], f func(N) bool) []N {
	filtered := make([]N, 0)
	for _, node := range dagger.GetNodes() {
		if f(node) {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func GetLayers[Id comparable, N Node[Id], E Edge[Id]](dagger Dagger[Id, N, E]) ([][]N, error) {
	// Check for cycles first
	if HasCycle(dagger) {
		return nil, ErrCycleDetected
	}

	// Build adjacency list and in-degree map
	adjacency := make(map[Id][]Id)
	inDegree := make(map[Id]int)
	nodeMap := make(map[Id]N)

	// Initialize all nodes with 0 in-degree
	for _, node := range dagger.GetNodes() {
		inDegree[node.GetId()] = 0
		nodeMap[node.GetId()] = node
	}

	// Build adjacency list and calculate in-degrees
	for _, edge := range dagger.GetEdges() {
		from := edge.GetFromId()
		to := edge.GetToId()
		adjacency[from] = append(adjacency[from], to)
		inDegree[to]++
	}

	layers := make([][]N, 0)
	processedCount := 0
	totalNodes := len(dagger.GetNodes())

	// Process nodes layer by layer
	for processedCount < totalNodes {
		currentLayer := make([]N, 0)

		// Find all nodes with in-degree 0
		for id, degree := range inDegree {
			if degree == 0 {
				if node, exists := nodeMap[id]; exists {
					currentLayer = append(currentLayer, node)
				}
			}
		}

		// If no nodes found but we haven't processed all nodes, something is wrong
		if len(currentLayer) == 0 && processedCount < totalNodes {
			// This shouldn't happen if HasCycle works correctly, but let's be safe
			return nil, ErrCycleDetected
		}

		// Add current layer
		if len(currentLayer) > 0 {
			layers = append(layers, currentLayer)
		}

		// Remove processed nodes and update in-degrees
		for _, node := range currentLayer {
			id := node.GetId()
			delete(inDegree, id)
			delete(nodeMap, id)
			processedCount++

			// Decrease in-degree for all neighbors
			for _, neighbor := range adjacency[id] {
				if _, exists := inDegree[neighbor]; exists {
					inDegree[neighbor]--
				}
			}
		}
	}

	// Add empty layer at the end for compatibility
	layers = append(layers, []N{})

	return layers, nil
}

var ErrNoLayerFound = errors.New("no layer found")

// GetRootLayer returns the root layer of the DAG (nodes with no incoming edges)
func GetRootLayer[Id comparable, N Node[Id], E Edge[Id]](dagger Dagger[Id, N, E]) ([]N, error) {
	layers, err := GetLayers(dagger)
	if err != nil {
		return nil, err
	}
	if len(layers) > 0 {
		return layers[0], nil
	}
	return []N{}, ErrNoLayerFound
}

// GetNextLayerBy function to get layer of nodes to process next by f(previous_layer) == true
func GetNextLayerBy[Id comparable, N Node[Id], E Edge[Id]](
	dagger Dagger[Id, N, E],
	f func([]N) bool,
) ([]N, error) {
	layers, err := GetLayers(dagger)
	if err != nil {
		return nil, err
	}
	for i := 1; i < len(layers); i++ {
		if f(layers[i-1]) {
			return layers[i], nil
		}
	}
	return nil, ErrNoLayerFound
}

// GetNextLayerByOrRoot returns the next layer to process or the root layer if no next layer is found
func GetNextLayerByOrRoot[Id comparable, N Node[Id], E Edge[Id]](
	dagger Dagger[Id, N, E],
	f func([]N) bool,
) ([]N, error) {
	layers, err := GetLayers(dagger)
	if err != nil {
		return nil, err
	}
	if len(layers) == 0 {
		return nil, ErrNoLayerFound
	}
	if len(layers) == 1 {
		return layers[0], nil
	}
	for i := 1; i < len(layers); i++ {
		if f(layers[i-1]) {
			return layers[i], nil
		}
	}
	return nil, ErrNoLayerFound
}

func GetLayerByReversed[Id comparable, N Node[Id], E Edge[Id]](
	dagger Dagger[Id, N, E],
	f func([]N) bool,
) ([]N, error) {
	layers, err := GetLayers(dagger)
	if err != nil {
		return nil, err
	}
	mutable.Reverse(layers)
	for _, layer := range layers {
		if f(layer) {
			return layer, nil
		}
	}
	return nil, ErrNoLayerFound
}

// GetLayerBy function to get layer of nodes by f(layer) == true
func GetLayerBy[Id comparable, N Node[Id], E Edge[Id]](
	dagger Dagger[Id, N, E],
	f func([]N) bool,
) ([]N, error) {
	layers, err := GetLayers(dagger)
	if err != nil {
		return nil, err
	}
	for _, layer := range layers {
		if f(layer) {
			return layer, nil
		}
	}
	return nil, ErrNoLayerFound
}

func AllBy[Id comparable, N Node[Id], E Edge[Id]](dagger Dagger[Id, N, E], f func(N) bool) bool {
	for _, node := range dagger.GetNodes() {
		if !f(node) {
			return false
		}
	}
	return true
}

func AnyBy[Id comparable, N Node[Id], E Edge[Id]](dagger Dagger[Id, N, E], f func(N) bool) bool {
	for _, node := range dagger.GetNodes() {
		if f(node) {
			return true
		}
	}
	return false
}
