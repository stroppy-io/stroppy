package xk6

import (
	"errors"
	"fmt"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	pluginLoggerName = "XK6Plugin"
)

// Instance is created by k6 for every VU.
type Instance struct {
	vu      modules.VU
	exports *sobek.Object
	logger  *zap.Logger
}

func NewXK6Instance(vu modules.VU, exports *sobek.Object) *Instance {
	lg := runPtr.logger
	if vu.State() != nil {
		vu.State().Logger = NewZapFieldLogger(lg)
	}

	return &Instance{
		vu:      vu,
		exports: exports,
		logger:  lg,
	}
}

func (x *Instance) New() *Instance {
	return x
}

func (x *Instance) Exports() modules.Exports {
	return modules.Exports{Default: x}
}

func (x *Instance) Setup(_ string) error {

	return nil
}

func (x *Instance) RunTransaction() string {
	transaction, err := runPtr.unitQueue.GetNextElement()
	if err != nil {
		return fmt.Errorf("can't get query due to: %w", err).Error()
	}
	runPtr.logger.Debug(
		"RunTransaction",
		zap.Any("transaction", transaction),
	)

	stats, err := runPtr.driver.RunTransaction(
		x.vu.Context(),
		transaction,
	)
	if err != nil {
		return fmt.Errorf("can't run query due to: %w", err).Error()
	}

	bytes, err := protojson.MarshalOptions{Multiline: false}.Marshal(stats)
	if err != nil {
		return fmt.Errorf("can't marshall stats due to: %w", err).Error()
	}

	return string(bytes)
}

var ErrDriverIsNil = errors.New("driver is nil")

func (x *Instance) Teardown() error {
	x.logger.Debug("K6 Teardown Start")
	if runPtr.driver == nil {
		return ErrDriverIsNil
	}
	x.logger.Debug("K6 Teardown driver")
	errDriver := runPtr.driver.Teardown(x.vu.Context())
	x.logger.Debug("K6 Teardown unit queue")
	errQueue := runPtr.unitQueue.Stop()
	x.logger.Debug("K6 Teardown End")
	return errors.Join(errQueue, errDriver)
}
