package k8s

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

const (
	fieldManager = "go-apply"
	trueString   = "True"
)

const (
	syncedCondition = "Synced"
	readyCondition  = "Ready"
)

const (
	statusField     = "status"
	atProviderField = "atProvider"
	idField         = "id"
)

func pointer(b bool) *bool { return &b }

func getCondition(obj *unstructured.Unstructured, condType string) string {
	conds, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return ""
	}

	for _, c := range conds {
		m, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		t, _ := m["type"].(string)
		s, _ := m["status"].(string)
		if t == condType {
			return s
		}
	}
	return ""
}
