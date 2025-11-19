package ids

import "github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"

func ExtRefFromResourceDef(
	ref *crossplane.Ref,
	def *crossplane.ResourceDef,
) *crossplane.ExtRef {
	return &crossplane.ExtRef{
		Ref:        ref,
		ApiVersion: def.GetApiVersion(),
		Kind:       def.GetKind(),
	}
}
