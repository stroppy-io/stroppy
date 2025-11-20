package kv

import (
	"bytes"
	"regexp"

	"github.com/samber/lo"
	"github.com/stroppy-io/stroppy-cloud-panel/internal/proto/panel"
)

type strOrBytes interface {
	[]byte | string
}

func EvalKv[T strOrBytes](target T, kvMap *panel.KV_Map) (T, error) {
	newBytes := []byte(target)
	for _, kv := range kvMap.Kvs {
		newBytes = bytes.ReplaceAll(newBytes, []byte("${"+kv.Key+"}"), []byte(kv.Value))
	}
	return T(newBytes), nil
}

func Set(kvMap *panel.KV_Map, key, value string) {
	for _, kvs := range kvMap.GetKvs() {
		if kvs.GetKey() == key {
			kvs.Value = value
		}
	}
}

var re = regexp.MustCompile(`\$\{(STROPPY_[^}]+)\}`)

// ExtractKvValues Get all KVs from target string
func ExtractKvValues[T strOrBytes](target T) *panel.KV_Keys {
	kvMap := &panel.KV_Keys{}

	// Regex for ${STROPPY_*}
	matches := re.FindAllSubmatch([]byte(target), -1)
	for _, m := range matches {
		// m[1] contains STROPPY_KEY
		key := m[1]
		kvMap.Keys = append(kvMap.Keys, string(key))
	}

	return kvMap
}

func MergeKeysWithExisted(existing []*panel.KvInfo, present *panel.KV_Keys) *panel.KV_Map {
	ret := &panel.KV_Map{}
	for _, p := range present.Keys {
		exists, founded := lo.Find(existing, func(item *panel.KvInfo) bool {
			return item.Key == p
		})
		if founded {
			ret.Kvs = append(ret.Kvs, &panel.KV{
				Key:   p,
				Value: "",
				Info:  exists.GetInfo(),
			})
		} else {
			ret.Kvs = append(ret.Kvs, &panel.KV{
				Key:   p,
				Value: "",
				Info:  &panel.KV_Info{},
			})
		}
	}
	return ret
}
