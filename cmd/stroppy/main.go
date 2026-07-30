package main

import (
	_ "github.com/stroppy-io/stroppy/pkg/driver/csv"
	_ "github.com/stroppy-io/stroppy/pkg/driver/mysql"
	_ "github.com/stroppy-io/stroppy/pkg/driver/noop"
	_ "github.com/stroppy-io/stroppy/pkg/driver/picodata"
	_ "github.com/stroppy-io/stroppy/pkg/driver/postgres"
	_ "github.com/stroppy-io/stroppy/pkg/driver/ydb"

	"github.com/stroppy-io/stroppy/cmd/stroppy/commands"
)

func main() {
	commands.Execute()
}
