package xk6air

import (
	"fmt"
	"sync"

	"github.com/stroppy-io/stroppy/pkg/common/proto/stroppy"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type DriverWrapper struct {
	vu                modules.VU
	lg                *zap.Logger
	drv               driver.Driver
	onceUpdateDialler sync.Once
}

func (d *DriverWrapper) RunQuery(sql string, args map[string]any) any {
	d.onceUpdateDialler.Do(
		func() { d.drv.UpdateDialler(d.vu.Context(), d.vu.State().Dialer.DialContext) },
	)

	stats, err := d.drv.RunQuery(d.vu.Context(), sql, args)
	if err != nil {
		return fmt.Errorf("error while executing sql query: %w", err)
	}
	return stats
}

// InsertValuesBin starts bulk insert blocking operation on driver.
func (d *DriverWrapper) InsertValuesBin(
	insertMsg []byte,
	count int64,
) any {
	d.onceUpdateDialler.Do(
		func() { d.drv.UpdateDialler(d.vu.Context(), d.vu.State().Dialer.DialContext) },
	)
	var descriptor stroppy.InsertDescriptor
	err := proto.Unmarshal(insertMsg, &descriptor)
	if err != nil {
		return err
	}

	stats, err := d.drv.InsertValues(d.vu.Context(), &descriptor)
	if err != nil {
		return err
	}

	return stats
}
