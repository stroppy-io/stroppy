package crossplaneservice

import (
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/crossplane"
)

func IsResourceReady(resource *crossplane.Resource) bool {
	return resource.GetReady() &&
		resource.GetSynced() &&
		resource.GetExternalId() != ""
}
