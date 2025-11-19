package dag

import (
	"reflect"
	"testing"
)

// Test implementations of the interfaces

type TestNode struct {
	Id    int
	Value string
}

func (n TestNode) GetId() int {
	return n.Id
}

type TestEdge struct {
	FromId int
	ToId   int
}

func (e TestEdge) GetFromId() int {
	return e.FromId
}

func (e TestEdge) GetToId() int {
	return e.ToId
}

type TestDagger struct {
	Nodes []TestNode
	Edges []TestEdge
}

func (d TestDagger) GetNodes() []TestNode {
	return d.Nodes
}

func (d TestDagger) GetEdges() []TestEdge {
	return d.Edges
}

// Tests for HasCycle

func TestHasCycle_NoCycle_EmptyGraph(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{},
		Edges: []TestEdge{},
	}

	if HasCycle(dagger) {
		t.Error("Expected no cycle in empty graph")
	}
}

func TestHasCycle_NoCycle_SingleNode(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "single"},
		},
		Edges: []TestEdge{},
	}

	if HasCycle(dagger) {
		t.Error("Expected no cycle in single node graph")
	}
}

func TestHasCycle_NoCycle_LinearChain(t *testing.T) {
	// 1 -> 2 -> 3
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 3},
		},
	}

	if HasCycle(dagger) {
		t.Error("Expected no cycle in linear chain")
	}
}

func TestHasCycle_NoCycle_DAG(t *testing.T) {
	// Complex DAG without cycle:
	//   1 -> 3
	//   2 -> 3
	//   3 -> 4
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
			{Id: 4, Value: "d"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 3},
			{FromId: 2, ToId: 3},
			{FromId: 3, ToId: 4},
		},
	}

	if HasCycle(dagger) {
		t.Error("Expected no cycle in valid DAG")
	}
}

func TestHasCycle_SelfLoop(t *testing.T) {
	// Node pointing to itself: 1 -> 1
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "self"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 1},
		},
	}

	if !HasCycle(dagger) {
		t.Error("Expected cycle detection for self-loop")
	}
}

func TestHasCycle_SimpleCycle(t *testing.T) {
	// Simple cycle: 1 -> 2 -> 1
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 1},
		},
	}

	if !HasCycle(dagger) {
		t.Error("Expected cycle detection for simple cycle")
	}
}

func TestHasCycle_LongerCycle(t *testing.T) {
	// Longer cycle: 1 -> 2 -> 3 -> 1
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 3},
			{FromId: 3, ToId: 1},
		},
	}

	if !HasCycle(dagger) {
		t.Error("Expected cycle detection for longer cycle")
	}
}

func TestHasCycle_ComplexGraphWithCycle(t *testing.T) {
	// Complex graph with cycle:
	//   1 -> 2
	//   2 -> 3
	//   3 -> 4
	//   4 -> 2 (creates cycle)
	//   1 -> 5
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
			{Id: 4, Value: "d"},
			{Id: 5, Value: "e"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 3},
			{FromId: 3, ToId: 4},
			{FromId: 4, ToId: 2}, // cycle!
			{FromId: 1, ToId: 5},
		},
	}

	if !HasCycle(dagger) {
		t.Error("Expected cycle detection in complex graph")
	}
}

func TestHasCycle_DisconnectedGraphNoCycle(t *testing.T) {
	// Two disconnected components, no cycles:
	//   1 -> 2
	//   3 -> 4
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
			{Id: 4, Value: "d"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 3, ToId: 4},
		},
	}

	if HasCycle(dagger) {
		t.Error("Expected no cycle in disconnected graph without cycles")
	}
}

func TestHasCycle_DisconnectedGraphWithCycle(t *testing.T) {
	// Two disconnected components, one has a cycle:
	//   1 -> 2 -> 1 (cycle)
	//   3 -> 4
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
			{Id: 4, Value: "d"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 1}, // cycle
			{FromId: 3, ToId: 4},
		},
	}

	if !HasCycle(dagger) {
		t.Error("Expected cycle detection in disconnected graph with cycle")
	}
}

// Tests for IsDagBy

func TestIsDagBy_AllNodesMatch(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "positive"},
			{Id: 2, Value: "positive"},
			{Id: 3, Value: "positive"},
		},
		Edges: []TestEdge{},
	}

	result := IsDagBy(dagger, func(n TestNode) bool {
		return n.Value == "positive"
	})

	if !result {
		t.Errorf("Expected true, got false")
	}
}

func TestIsDagBy_SomeNodesDoNotMatch(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "positive"},
			{Id: 2, Value: "negative"},
			{Id: 3, Value: "positive"},
		},
		Edges: []TestEdge{},
	}

	result := IsDagBy(dagger, func(n TestNode) bool {
		return n.Value == "positive"
	})

	if result {
		t.Errorf("Expected false, got true")
	}
}

func TestIsDagBy_EmptyGraph(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{},
		Edges: []TestEdge{},
	}

	result := IsDagBy(dagger, func(n TestNode) bool {
		return n.Id > 0
	})

	if !result {
		t.Errorf("Expected true for empty graph, got false")
	}
}

func TestIsDagBy_IdCondition(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 5, Value: "a"},
			{Id: 10, Value: "b"},
			{Id: 15, Value: "c"},
		},
		Edges: []TestEdge{},
	}

	result := IsDagBy(dagger, func(n TestNode) bool {
		return n.Id >= 5
	})

	if !result {
		t.Errorf("Expected true, got false")
	}

	result = IsDagBy(dagger, func(n TestNode) bool {
		return n.Id > 10
	})

	if result {
		t.Errorf("Expected false, got true")
	}
}

// Tests for FilterNodes

func TestFilterNodes_NoMatch(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
		},
		Edges: []TestEdge{},
	}

	filtered := FilterNodes(dagger, func(n TestNode) bool {
		return n.Value == "z"
	})

	if len(filtered) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(filtered))
	}
}

func TestFilterNodes_AllMatch(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
		},
		Edges: []TestEdge{},
	}

	filtered := FilterNodes(dagger, func(n TestNode) bool {
		return n.Id > 0
	})

	if len(filtered) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(filtered))
	}
}

func TestFilterNodes_PartialMatch(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
			{Id: 4, Value: "d"},
		},
		Edges: []TestEdge{},
	}

	filtered := FilterNodes(dagger, func(n TestNode) bool {
		return n.Id%2 == 0
	})

	if len(filtered) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(filtered))
	}

	expectedIds := []int{2, 4}
	for i, node := range filtered {
		if node.Id != expectedIds[i] {
			t.Errorf("Expected node id %d at position %d, got %d", expectedIds[i], i, node.Id)
		}
	}
}

func TestFilterNodes_EmptyGraph(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{},
		Edges: []TestEdge{},
	}

	filtered := FilterNodes(dagger, func(n TestNode) bool {
		return true
	})

	if len(filtered) != 0 {
		t.Errorf("Expected 0 nodes, got %d", len(filtered))
	}
}

// Tests for GetLayers

func TestGetLayers_EmptyGraph(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{},
		Edges: []TestEdge{},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have at least 1 layer (the empty terminating layer)
	if len(layers) == 0 {
		t.Fatal("Expected at least 1 layer")
	}

	// Last layer should be empty
	if len(layers[len(layers)-1]) != 0 {
		t.Errorf("Expected last layer to be empty, got %d nodes", len(layers[len(layers)-1]))
	}
}

func TestGetLayers_SingleNode(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
		},
		Edges: []TestEdge{},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(layers) < 2 {
		t.Fatal("Expected at least 2 layers")
	}

	// First layer should have the node
	if len(layers[0]) != 1 {
		t.Errorf("Expected first layer to have 1 node, got %d", len(layers[0]))
	}

	// Last layer should be empty
	if len(layers[len(layers)-1]) != 0 {
		t.Errorf("Expected last layer to be empty, got %d nodes", len(layers[len(layers)-1]))
	}
}

func TestGetLayers_LinearChain(t *testing.T) {
	// 1 -> 2 -> 3
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 3},
		},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have 4 layers: [1], [2], [3], []
	if len(layers) != 4 {
		t.Errorf("Expected 4 layers, got %d", len(layers))
	}

	// First layer should have node 1
	if len(layers[0]) != 1 || layers[0][0].Id != 1 {
		t.Errorf("Expected first layer to have node 1")
	}

	// Second layer should have node 2
	if len(layers[1]) != 1 || layers[1][0].Id != 2 {
		t.Errorf("Expected second layer to have node 2")
	}

	// Third layer should have node 3
	if len(layers[2]) != 1 || layers[2][0].Id != 3 {
		t.Errorf("Expected third layer to have node 3")
	}

	// Last layer should be empty
	if len(layers[3]) != 0 {
		t.Errorf("Expected last layer to be empty, got %d nodes", len(layers[3]))
	}
}

func TestGetLayers_DAG(t *testing.T) {
	// Complex DAG:
	//   1 -> 3
	//   2 -> 3
	//   3 -> 4
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
			{Id: 3, Value: "c"},
			{Id: 4, Value: "d"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 3},
			{FromId: 2, ToId: 3},
			{FromId: 3, ToId: 4},
		},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have 4 layers: [1,2], [3], [4], []
	if len(layers) != 4 {
		t.Errorf("Expected 4 layers, got %d", len(layers))
	}

	// First layer should have nodes 1 and 2
	if len(layers[0]) != 2 {
		t.Errorf("Expected first layer to have 2 nodes, got %d", len(layers[0]))
	}

	// Last layer should be empty
	if len(layers[len(layers)-1]) != 0 {
		t.Errorf("Expected last layer to be empty, got %d nodes", len(layers[len(layers)-1]))
	}
}

func TestGetLayers_WithCycle_ReturnsError(t *testing.T) {
	// Graph with cycle: 1 -> 2 -> 1
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 1},
		},
	}

	_, err := GetLayers(dagger)
	if err == nil {
		t.Fatal("Expected error for graph with cycle")
	}

	if err != ErrCycleDetected {
		t.Errorf("Expected ErrCycleDetected, got %v", err)
	}
}

// Tests for GetNextLayerBy

func TestGetNextLayerBy_EmptyGraph(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{},
		Edges: []TestEdge{},
	}

	nextLayer, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		return len(layer) == 0
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Empty graph has only one layer, so there's no next layer
	if nextLayer != nil {
		t.Errorf("Expected nil result for empty graph, got layer with %d nodes", len(nextLayer))
	}
}

func TestGetNextLayerBy_NoMatch(t *testing.T) {
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
			{Id: 2, Value: "b"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
		},
	}

	nextLayer, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		return len(layer) == 100 // Never matches
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nextLayer != nil {
		t.Errorf("Expected nil result, got layer with %d nodes", len(nextLayer))
	}
}

func TestGetNextLayerBy_WithCycle_ReturnsError(t *testing.T) {
	// Node with self-loop edge
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "a"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 1},
		},
	}

	_, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		return len(layer) == 1
	})

	if err == nil {
		t.Fatal("Expected error for graph with cycle")
	}

	if err != ErrCycleDetected {
		t.Errorf("Expected ErrCycleDetected, got %v", err)
	}
}

// Real-world examples for GetNextLayerBy

func TestGetNextLayerBy_WorkflowProcessing(t *testing.T) {
	// Simulate a workflow: start -> process1, process2 -> end
	// start (1) has no incoming edges
	// process1 (2), process2 (3) depend on start
	// end (4) depends on both process1 and process2
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "start"},
			{Id: 2, Value: "process1"},
			{Id: 3, Value: "process2"},
			{Id: 4, Value: "end"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 1, ToId: 3},
			{FromId: 2, ToId: 4},
			{FromId: 3, ToId: 4},
		},
	}

	// Get layers first to understand structure
	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Find next layer after the layer containing "start" node
	nextLayer, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		for _, node := range layer {
			if node.Value == "start" {
				return true
			}
		}
		return false
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nextLayer == nil {
		t.Fatal("Expected to find next layer after start layer")
	}

	// Verify we found a layer
	t.Logf("Found next layer with %d nodes", len(nextLayer))
	t.Logf("Total layers: %d", len(layers))
}

func TestGetNextLayerBy_AllNodesHaveStatus(t *testing.T) {
	// Simulate processing nodes based on value condition
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "ready"},
			{Id: 2, Value: "ready"},
			{Id: 3, Value: "pending"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 3},
			{FromId: 2, ToId: 3},
		},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Find next layer when all nodes in previous layer are "ready"
	nextLayer, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		if len(layer) == 0 {
			return false
		}
		for _, node := range layer {
			if node.Value != "ready" {
				return false
			}
		}
		return true
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should return some layer
	t.Logf("Total layers: %d", len(layers))
	if nextLayer != nil {
		t.Logf("Found next layer with %d nodes", len(nextLayer))
	} else {
		t.Log("No next layer found (no layer had all ready nodes)")
	}
}

func TestGetNextLayerBy_MinimumNodesThreshold(t *testing.T) {
	// Real scenario: process next batch when previous batch has at least N nodes
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "batch1"},
			{Id: 2, Value: "batch1"},
			{Id: 3, Value: "batch1"},
			{Id: 4, Value: "batch2"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 4},
			{FromId: 2, ToId: 4},
			{FromId: 3, ToId: 4},
		},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Find next layer when previous layer has at least 3 nodes
	nextLayer, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		return len(layer) >= 3
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nextLayer == nil {
		t.Fatal("Expected to find next layer after layer with 3+ nodes")
	}

	t.Logf("Total layers: %d", len(layers))
	t.Logf("Found next layer with %d nodes", len(nextLayer))
}

func TestGetNextLayerBy_AggregateCondition(t *testing.T) {
	// Real scenario: process next layer when sum of IDs in previous layer exceeds threshold
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 10, Value: "low"},
			{Id: 20, Value: "medium"},
			{Id: 30, Value: "high"},
			{Id: 5, Value: "result"},
		},
		Edges: []TestEdge{
			{FromId: 10, ToId: 5},
			{FromId: 20, ToId: 5},
			{FromId: 30, ToId: 5},
		},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Find next layer when sum of IDs in previous layer is greater than 25
	nextLayer, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		sum := 0
		for _, node := range layer {
			sum += node.Id
		}
		return sum > 25
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if nextLayer != nil {
		t.Logf("Found next layer with %d nodes when aggregate condition met", len(nextLayer))
	}
	t.Logf("Total layers: %d", len(layers))
}

func TestGetNextLayerBy_MultipleIterations(t *testing.T) {
	// Real scenario: iterating through layers sequentially
	dagger := TestDagger{
		Nodes: []TestNode{
			{Id: 1, Value: "stage1"},
			{Id: 2, Value: "stage2"},
			{Id: 3, Value: "stage3"},
		},
		Edges: []TestEdge{
			{FromId: 1, ToId: 2},
			{FromId: 2, ToId: 3},
		},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	t.Logf("Total layers: %d", len(layers))

	// Try to find next layer multiple times with different conditions
	firstNext, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		return len(layer) > 0
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if firstNext != nil {
		t.Logf("First next layer has %d nodes", len(firstNext))
	}

	// Find next layer when layer contains node with id=1
	secondNext, err := GetNextLayerBy(dagger, func(layer []TestNode) bool {
		for _, node := range layer {
			if node.Id == 1 {
				return true
			}
		}
		return false
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if secondNext != nil {
		t.Logf("Second next layer has %d nodes", len(secondNext))
	}
}

// Test with string IDs

type StringNode struct {
	Id    string
	Value int
}

func (n StringNode) GetId() string {
	return n.Id
}

type StringEdge struct {
	FromId string
	ToId   string
}

func (e StringEdge) GetFromId() string {
	return e.FromId
}

func (e StringEdge) GetToId() string {
	return e.ToId
}

type StringDagger struct {
	Nodes []StringNode
	Edges []StringEdge
}

func (d StringDagger) GetNodes() []StringNode {
	return d.Nodes
}

func (d StringDagger) GetEdges() []StringEdge {
	return d.Edges
}

func TestIsDagBy_WithStringIds(t *testing.T) {
	dagger := StringDagger{
		Nodes: []StringNode{
			{Id: "node1", Value: 10},
			{Id: "node2", Value: 20},
			{Id: "node3", Value: 30},
		},
		Edges: []StringEdge{},
	}

	result := IsDagBy(dagger, func(n StringNode) bool {
		return n.Value >= 10
	})

	if !result {
		t.Errorf("Expected true, got false")
	}
}

func TestFilterNodes_WithStringIds(t *testing.T) {
	dagger := StringDagger{
		Nodes: []StringNode{
			{Id: "alpha", Value: 5},
			{Id: "beta", Value: 15},
			{Id: "gamma", Value: 25},
		},
		Edges: []StringEdge{},
	}

	filtered := FilterNodes(dagger, func(n StringNode) bool {
		return n.Value > 10
	})

	if len(filtered) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(filtered))
	}

	expectedIds := []string{"beta", "gamma"}
	gotIds := []string{}
	for _, node := range filtered {
		gotIds = append(gotIds, node.Id)
	}

	if !reflect.DeepEqual(gotIds, expectedIds) {
		t.Errorf("Expected ids %v, got %v", expectedIds, gotIds)
	}
}

func TestGetLayers_WithStringIds(t *testing.T) {
	// DAG with string IDs: a -> b -> c
	dagger := StringDagger{
		Nodes: []StringNode{
			{Id: "a", Value: 1},
			{Id: "b", Value: 2},
			{Id: "c", Value: 3},
		},
		Edges: []StringEdge{
			{FromId: "a", ToId: "b"},
			{FromId: "b", ToId: "c"},
		},
	}

	layers, err := GetLayers(dagger)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(layers) != 4 {
		t.Errorf("Expected 4 layers, got %d", len(layers))
	}

	// First layer should have node "a"
	if len(layers[0]) != 1 || layers[0][0].Id != "a" {
		t.Errorf("Expected first layer to have node 'a'")
	}
}

func TestGetNextLayerBy_WithStringIds(t *testing.T) {
	// DAG with string IDs
	dagger := StringDagger{
		Nodes: []StringNode{
			{Id: "x", Value: 100},
			{Id: "y", Value: 200},
		},
		Edges: []StringEdge{
			{FromId: "x", ToId: "y"},
		},
	}

	nextLayer, err := GetNextLayerBy(dagger, func(layer []StringNode) bool {
		return len(layer) == 1
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should find next layer after first layer with 1 node
	if nextLayer == nil {
		t.Fatal("Expected to find next layer")
	}
}

func TestHasCycle_WithStringIds(t *testing.T) {
	// Test cycle detection with string IDs
	dagger := StringDagger{
		Nodes: []StringNode{
			{Id: "a", Value: 1},
			{Id: "b", Value: 2},
		},
		Edges: []StringEdge{
			{FromId: "a", ToId: "b"},
			{FromId: "b", ToId: "a"}, // cycle
		},
	}

	if !HasCycle(dagger) {
		t.Error("Expected cycle detection with string IDs")
	}
}
