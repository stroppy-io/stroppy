package build

import (
	"fmt"
	"github.com/google/uuid"
)

var (
	Version          = "0.0.0" //nolint: gochecknoglobals // This file could be global
	ServiceName      = ""      //nolint: gochecknoglobals // This file could be global
	GlobalInstanceId = fmt.Sprintf("%s-%s-%s", ServiceName, Version, uuid.NewString())
)
