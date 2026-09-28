package main

import (
	"github.com/stroppy-io/stroppy/v6/cmd/stroppy/commands"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/csv"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/mysql"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/noop"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/picodata"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/postgres"
	_ "github.com/stroppy-io/stroppy/v6/pkg/driver/ydb"
)

func main() {
	commands.Execute()
}
