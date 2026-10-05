package dsqgen

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed templates/*.tpl
var templateFS embed.FS

// LoadTemplates parses every vendored query template, returned in query-number
// order (query1 … query99).
func LoadTemplates() ([]*Template, error) {
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil, err
	}

	type named struct {
		num int
		ent fs.DirEntry
	}
	var ns []named
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "query") || !strings.HasSuffix(e.Name(), ".tpl") {
			continue
		}
		num, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(e.Name(), "query"), ".tpl"))
		if err != nil {
			continue // dialect templates (ansi.tpl etc.) if any slipped in
		}
		ns = append(ns, named{num, e})
	}
	sort.Slice(ns, func(i, j int) bool { return ns[i].num < ns[j].num })

	out := make([]*Template, 0, len(ns))
	for _, n := range ns {
		src, err := templateFS.ReadFile("templates/" + n.ent.Name())
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(n.ent.Name(), ".tpl")
		t, err := ParseTemplate(name, string(src))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", name, err)
		}
		out = append(out, t)
	}
	return out, nil
}
