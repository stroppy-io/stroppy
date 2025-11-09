package nodetree

import (
	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/entity/resource"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

// TreeNodeCallback is a callback function for tree node traversal.
// It receives the current node and its depth in the tree (root is depth 0).
// Return false to stop traversal, true to continue.
// If an error is returned, traversal stops immediately and the error is propagated.
type TreeNodeCallback func(node *panel.CloudResource_TreeNode, depth int) (bool, error)

// TraverseTreePreOrder traverses the tree in pre-order (node first, then children).
// The callback is called for each node with the node itself and its depth.
// If the callback returns false, traversal stops immediately.
// If the callback returns an error, traversal stops and the error is returned.
func TraverseTreePreOrder(root *panel.CloudResource_TreeNode, callback TreeNodeCallback) error {
	if root == nil {
		return nil
	}
	_, err := traversePreOrderHelper(root, callback, 0)
	return err
}

func traversePreOrderHelper(node *panel.CloudResource_TreeNode, callback TreeNodeCallback, depth int) (bool, error) {
	if node == nil {
		return true, nil
	}

	// Visit the node first
	continue_, err := callback(node, depth)
	if err != nil {
		return false, err
	}
	if !continue_ {
		return false, nil
	}

	// Then visit children
	for _, child := range node.Children {
		continue_, err := traversePreOrderHelper(child, callback, depth+1)
		if err != nil {
			return false, err
		}
		if !continue_ {
			return false, nil
		}
	}

	return true, nil
}

// TraverseTreePostOrder traverses the tree in post-order (children first, then node).
// The callback is called for each node with the node itself and its depth.
// If the callback returns false, traversal stops immediately.
// If the callback returns an error, traversal stops and the error is returned.
func TraverseTreePostOrder(root *panel.CloudResource_TreeNode, callback TreeNodeCallback) error {
	if root == nil {
		return nil
	}
	_, err := traversePostOrderHelper(root, callback, 0)
	return err
}

func traversePostOrderHelper(node *panel.CloudResource_TreeNode, callback TreeNodeCallback, depth int) (bool, error) {
	if node == nil {
		return true, nil
	}

	// Visit children first
	for _, child := range node.Children {
		continue_, err := traversePostOrderHelper(child, callback, depth+1)
		if err != nil {
			return false, err
		}
		if !continue_ {
			return false, nil
		}
	}

	// Then visit the node
	return callback(node, depth)
}

// TraverseTreeBreadthFirst traverses the tree level by level (breadth-first).
// The callback is called for each node with the node itself and its depth.
// If the callback returns false, traversal stops immediately.
// If the callback returns an error, traversal stops and the error is returned.
func TraverseTreeBreadthFirst(root *panel.CloudResource_TreeNode, callback TreeNodeCallback) error {
	if root == nil {
		return nil
	}

	type queueItem struct {
		node  *panel.CloudResource_TreeNode
		depth int
	}

	queue := []queueItem{{node: root, depth: 0}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.node == nil {
			continue
		}

		// Visit the node
		continue_, err := callback(item.node, item.depth)
		if err != nil {
			return err
		}
		if !continue_ {
			return nil
		}

		// Add children to queue
		for _, child := range item.node.Children {
			if child != nil {
				queue = append(queue, queueItem{node: child, depth: item.depth + 1})
			}
		}
	}

	return nil
}

// FindInTree searches for a node in the tree using pre-order traversal.
// The predicate function should return true when the desired node is found.
// Returns the first matching node or nil if not found.
// If an error occurs during traversal, it is returned.
func FindInTree(root *panel.CloudResource_TreeNode, predicate func(node *panel.CloudResource_TreeNode) bool) (*panel.CloudResource_TreeNode, error) {
	var result *panel.CloudResource_TreeNode

	err := TraverseTreePreOrder(root, func(node *panel.CloudResource_TreeNode, depth int) (bool, error) {
		if predicate(node) {
			result = node
			return false, nil // Stop traversal
		}
		return true, nil // Continue traversal
	})

	return result, err
}

// CollectNodes collects all nodes that match the predicate.
// Returns a slice of all matching nodes.
// If an error occurs during traversal, it is returned.
func CollectNodes(root *panel.CloudResource_TreeNode, predicate func(node *panel.CloudResource_TreeNode) bool) ([]*panel.CloudResource_TreeNode, error) {
	var result []*panel.CloudResource_TreeNode

	err := TraverseTreePreOrder(root, func(node *panel.CloudResource_TreeNode, depth int) (bool, error) {
		if predicate(node) {
			result = append(result, node)
		}
		return true, nil // Always continue
	})

	return result, err
}

func IsNodeAndDescendantsReady(node *panel.CloudResource_TreeNode) (bool, error) {
	nodeDescendants, err := GetDescendants(node)
	if err != nil {
		return false, err
	}
	return IsNodeReady(node) && lo.CountBy(nodeDescendants, IsNodeReady) == len(nodeDescendants), nil
}

func IsNodeReady(node *panel.CloudResource_TreeNode) bool {
	return node.GetResource().GetResource().GetReady() &&
		node.GetResource().GetResource().GetSynced() &&
		node.GetResource().GetResource().GetExternalId() != ""
}

func GetExtNodeRef(node *panel.CloudResource_TreeNode) *crossplane.ExtRef {
	return resource.ExtRefFromResourceDef(
		node.GetResource().GetResource().GetRef(),
		node.GetResource().GetResource().GetResourceDef(),
	)
}

func GetDescendants(root *panel.CloudResource_TreeNode) ([]*panel.CloudResource_TreeNode, error) {
	return CollectNodes(root, func(node *panel.CloudResource_TreeNode) bool {
		return node.GetId() != root.GetId()
	})
}

// CountNodes counts the total number of nodes in the tree.
// If an error occurs during traversal, it is returned.
func CountNodes(root *panel.CloudResource_TreeNode) (int, error) {
	count := 0
	err := TraverseTreePreOrder(root, func(node *panel.CloudResource_TreeNode, depth int) (bool, error) {
		count++
		return true, nil
	})
	return count, err
}

// GetMaxDepth returns the maximum depth of the tree (root is depth 0).
// If an error occurs during traversal, it is returned.
func GetMaxDepth(root *panel.CloudResource_TreeNode) (int, error) {
	maxDepth := -1
	err := TraverseTreePreOrder(root, func(node *panel.CloudResource_TreeNode, depth int) (bool, error) {
		if depth > maxDepth {
			maxDepth = depth
		}
		return true, nil
	})
	return maxDepth, err
}

// GetParents returns all parent nodes from root to the target node (excluding the target itself).
// The slice is ordered from root to immediate parent.
// Returns nil if the node is not found in the tree.
// The predicate function should return true when the target node is found.
func GetParents(root *panel.CloudResource_TreeNode, predicate func(node *panel.CloudResource_TreeNode) bool) ([]*panel.CloudResource_TreeNode, error) {
	var path []*panel.CloudResource_TreeNode
	found := findPathToNode(root, predicate, &path)
	if !found || len(path) == 0 {
		return nil, nil
	}
	// Return all nodes except the last one (which is the target node itself)
	return path[:len(path)-1], nil
}

// GetAncestors returns all ancestor nodes from root to the target node (including the target itself).
// The slice is ordered from root to the target node.
// Returns nil if the node is not found in the tree.
// The predicate function should return true when the target node is found.
func GetAncestors(root *panel.CloudResource_TreeNode, predicate func(node *panel.CloudResource_TreeNode) bool) ([]*panel.CloudResource_TreeNode, error) {
	var path []*panel.CloudResource_TreeNode
	found := findPathToNode(root, predicate, &path)
	if !found {
		return nil, nil
	}
	return path, nil
}

// findPathToNode is a helper function that finds the path from root to a node matching the predicate.
// Returns true if the node is found, false otherwise.
func findPathToNode(node *panel.CloudResource_TreeNode, predicate func(node *panel.CloudResource_TreeNode) bool, path *[]*panel.CloudResource_TreeNode) bool {
	if node == nil {
		return false
	}

	// Add current node to path
	*path = append(*path, node)

	// Check if this is the target node
	if predicate(node) {
		return true
	}

	// Search in children
	for _, child := range node.Children {
		if findPathToNode(child, predicate, path) {
			return true
		}
	}

	// Node not found in this subtree, remove from path
	*path = (*path)[:len(*path)-1]
	return false
}
