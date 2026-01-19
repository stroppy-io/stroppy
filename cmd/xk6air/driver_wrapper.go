package xk6air

import (
	"github.com/grafana/sobek"
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

// RunUnit runs a single driver unit: query | transaction | create_table | insert
func (d *DriverWrapper) RunUnit(unitMsg []byte) (sobek.ArrayBuffer, error) {
	var unit stroppy.UnitDescriptor
	err := proto.Unmarshal(unitMsg, &unit)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	stats, err := d.drv.RunTransaction(d.vu.Context(), &unit)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	statsMsg, err := proto.Marshal(stats)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}
	return d.vu.Runtime().NewArrayBuffer(statsMsg), nil
}

// InsertValues starts bulk insert blocking operation on driver.
func (d *DriverWrapper) InsertValues(insertMsg []byte, count int64) (sobek.ArrayBuffer, error) {
	var descriptor stroppy.InsertDescriptor
	err := proto.Unmarshal(insertMsg, &descriptor)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	stats, err := d.drv.InsertValues(d.vu.Context(), &descriptor, count)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}

	statsMsg, err := proto.Marshal(stats)
	if err != nil {
		return sobek.ArrayBuffer{}, err
	}
	return d.vu.Runtime().NewArrayBuffer(statsMsg), nil
}
