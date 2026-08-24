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
