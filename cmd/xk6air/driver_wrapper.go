package xk6air

import (
	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type DriverWrapper struct {
	vu  modules.VU
	lg  *zap.Logger
	drv driver.Driver
}

func (d *DriverWrapper) RunQuery(sql string, args map[string]any) {
	d.drv.RunQuery(d.vu.Context(), sql, args)
}

// InsertValuesBin starts bulk insert blocking operation on driver.
func (d *DriverWrapper) InsertValuesBin(
	insertMsg []byte,
	count int64,
) error {
	var descriptor stroppy.InsertDescriptor
	err := proto.Unmarshal(insertMsg, &descriptor)
	if err != nil {
		return err
	}

	_, err = d.drv.InsertValues(d.vu.Context(), &descriptor)
	if err != nil {
		return err
	}

	// can't binary unconvert stats on ts side
	// statsMsg, err := proto.Marshal(stats)
	// if err != nil {
	// 	return err
	// }
	return nil
}
