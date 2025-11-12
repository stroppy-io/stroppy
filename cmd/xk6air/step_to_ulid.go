package xk6air

import (
	"github.com/oklog/ulid/v2"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var stepNameToId cmap.ConcurrentMap[string, ulid.ULID] = cmap.New[ulid.ULID]()

func getStepId(name string) ulid.ULID {
	id, ok := stepNameToId.Get(name)
	if !ok {
		id = ulid.Make()
		stepNameToId.Set(name, id)
	}
	return id
}
