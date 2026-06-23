package dsdgen

// MakePermutation builds a size-element permutation by Fisher-Yates shuffle
// driven by stream, mirroring Permutations.makePermutation. The fact tables use
// it to assign items to sales lines without repetition within an order.
func MakePermutation(size int, s *RNStream) []int {
	set := make([]int, size)
	for i := range set {
		set[i] = i
	}
	for i := range set {
		j := GenerateUniformRandomInt(0, size-1, s)
		set[i], set[j] = set[j], set[i]
	}

	return set
}

// PermutationEntry returns the 1-based entry at the 1-based index, mirroring
// getPermutationEntry.
func PermutationEntry(perm []int, index int) int {
	return perm[index-1] + 1
}
