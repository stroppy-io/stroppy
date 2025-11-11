package postgres

import (
	"strings"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
)

func (d *Driver) CopyFromQuery(query *stroppy.DriverQuery) error {
	// here it's a bad query
	table := strings.Split(query.GetRequest(), " ")
	tableName := table[0]
	paramsNames := table[1:]

	values := make([]any, len(query.GetParams()))

	err := d.fillParamsToValues(query, values)
	if err != nil {
		return err
	}

	source, ok := d.tableToCopyChannel.Get(tableName)
	if !ok { // init
		source = d.copyFromStarter(tableName, paramsNames)
		d.tableToCopyChannel.SetIfAbsent(tableName, source)
	}

	source <- values

	return nil
}

func (d *Driver) CloseCopyChannels() {
	for _, source := range d.tableToCopyChannel.Items() {
		close(source)
	}

	d.tableToCopyChannel.Clear()
}
