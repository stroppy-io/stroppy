package xk6

import (
	"errors"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"
	"go.uber.org/zap"

	"github.com/stroppy-io/stroppy/pkg/core/logger"
	stroppy "github.com/stroppy-io/stroppy/pkg/core/proto"
	"github.com/stroppy-io/stroppy/pkg/driver/postgres"
)

const (
	pluginLoggerName = "XK6Plugin"
)

type Instance struct {
	vu      modules.VU
	exports *sobek.Object
	logger  *zap.Logger
}

func NewXK6Instance(vu modules.VU, exports *sobek.Object) *Instance {
	lg := logger.NewFromEnv().
		Named(pluginLoggerName).
		WithOptions(zap.AddCallerSkip(1))
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

func (x *Instance) Setup(runContextBytes string) error {
	runContext, err := Serialized[*stroppy.StepContext](runContextBytes).Unmarshal()
	if err != nil {
		return err
	}

	x.logger.Debug(
		"Setup",
		zap.Uint64("seed", runContext.GetGlobalConfig().GetRun().GetSeed()),
	)

	drv := postgres.NewDriver()

	err = drv.Initialize(x.vu.Context(), runContext)
	if err != nil {
		return err
	}

	runPtr = newRuntimeContext(
		drv,
		x.logger,
		runContext,
	)

	return nil
}

//goland:noinspection t
func (x *Instance) GenerateQueue() (string, error) {
	stepTransactions := make([]*stroppy.DriverTransaction, 0)

	for _, unitDescr := range runPtr.runContext.GetStep().GetUnits() {
		queries, err := runPtr.driver.BuildTransactionsFromUnit(
			x.vu.Context(),
			&stroppy.UnitBuildContext{
				Context: runPtr.runContext,
				Unit:    unitDescr,
			},
		)
		if err != nil {
			return "", err
		}

		stepTransactions = append(stepTransactions, queries.GetTransactions()...)
	}

	return MarshalSerialized(&stroppy.DriverTransactionList{Transactions: stepTransactions})
}

func (x *Instance) RunQuery(queryData string) error {
	transaction, err := Serialized[*stroppy.DriverTransaction](queryData).Unmarshal()
	if err != nil {
		return err
	}

	runPtr.logger.Debug(
		"RunQuery",
		zap.Any("transaction", transaction),
	)

	return runPtr.driver.RunTransaction(
		x.vu.Context(),
		transaction,
	)
}

var ErrDriverIsNil = errors.New("driver is nil")

func (x *Instance) Teardown() error {
	if runPtr.driver == nil {
		return ErrDriverIsNil
	}

	return runPtr.driver.Teardown(x.vu.Context())
}
