package dsdgen

// Shared person-name / word generation used by the store and call_center
// generators (manager names) and any other table that builds words from
// syllables. Mirrors NamesDistributions / RandomValueGenerator.generateWord.
var (
	syllablesDist  = mustLoadStringValues("syllables.dst", 1, 1)
	firstNamesDist = mustLoadStringValues("first_names.dst", 1, 3)
	lastNamesDist  = mustLoadStringValues("last_names.dst", 1, 1)
)

// firstNamesMaleFrequency is the FirstNamesWeights MALE ordinal. dsdgen's
// default is "sexist" (Session.isSexist()==true), so this weight column is used.
const firstNamesMaleFrequency = 0

// generateWord builds a word from a numeric seed by treating it as a base-Size
// number and concatenating the syllable addressed by each digit until the word
// would exceed maxChars. It consumes no RNG draws (the seed is usually the row
// number). Mirrors RandomValueGenerator.generateWord.
func generateWord(seed int64, maxChars int, d *StringValuesDistribution) string {
	size := int64(d.Size())
	var word string
	for seed > 0 {
		syllable := d.ValueAtIndex(0, int(seed%size))
		seed /= size
		if len(word)+len(syllable) <= maxChars {
			word += syllable
		} else {
			break
		}
	}

	return word
}
