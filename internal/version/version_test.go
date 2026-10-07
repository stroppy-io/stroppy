package version

import "testing"

func TestResolvePrefersInjectedVersion(t *testing.T) {
	previous := Version
	Version = "v6.2.3"

	t.Cleanup(func() { Version = previous })

	if got := Resolve(); got != "v6.2.3" {
		t.Fatalf("Resolve() = %q", got)
	}
}
