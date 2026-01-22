package xk6air

import (
	"github.com/oklog/ulid/v2"
	cmap "github.com/orcaman/concurrent-map/v2"
)

var stepNameToID cmap.ConcurrentMap[string, ulid.ULID] = cmap.New[ulid.ULID]()

func getStepID(name string) ulid.ULID {
	id, ok := stepNameToID.Get(name)
	if !ok {
		id = ulid.Make()
		stepNameToID.Set(name, id)
	}
	return id
}
