package help

import (
	"strings"
	"testing"
)

func TestRemovedConfigFieldsAreAbsentFromHelp(t *testing.T) {
	var content strings.Builder
	for _, topic := range topics {
		content.WriteString(topic.Long)
	}

	for _, field := range []string{"k6Args", "k6Config", "defaultTxIsolation", "default_tx_isolation"} {
		if strings.Contains(content.String(), field) {
			t.Errorf("help contains removed field %q", field)
		}
	}
}

func TestLoggerDefaultsInConfigFileHelp(t *testing.T) {
	for _, topic := range topics {
		if topic.Name != "config-file" {
			continue
		}

		if !strings.Contains(topic.Long, "With the debug/development defaults, stroppy logs:") {
			t.Error("config-file help does not state the debug/development logger defaults")
		}

		if strings.Contains(topic.Long, "At INFO level (default)") {
			t.Error("config-file help incorrectly states that info is the logger default")
		}

		return
	}

	t.Error("config-file help topic is not registered")
}
