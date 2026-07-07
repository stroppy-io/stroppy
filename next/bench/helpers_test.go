package bench

// envMap builds a getenv from a map. Shared test helper for the bench package's
// tests (resolution, probe, wiring).
func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}
