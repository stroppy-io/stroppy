package postgres

import (
	"strings"

	stroppy "github.com/stroppy-io/stroppy/pkg/common/proto"
)

func (d *Driver) CopyFromQuery(query *stroppy.DriverQuery) (err error) {

	// here it's a bad query
	table := strings.Split(query.GetRequest(), " ")
	tableName := table[0]
	paramsNames := table[1:]

	values := make([]any, len(query.GetParams()))
	err = d.fillParamsToValues(query, values)
	if err != nil {
		return err
	}

	source, ok := d.connMap.Get(tableName)
	if !ok { // init
		source = d.f(tableName, paramsNames)
		d.connMap.SetIfAbsent(tableName, source)
	}

	source <- values
	return nil
}

func (d *Driver) ResetCopyFrom() {
	for _, source := range d.connMap.Items() {
		close(source)
	}
	d.connMap.Clear()
}
