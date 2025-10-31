package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stroppy-io/stroppy/pkg/common/proto"
	"github.com/stroppy-io/stroppy/pkg/driver"
	"go.uber.org/zap"
)

func main() {

}

func InsertOldWay(
	ctx context.Context,
	lg *zap.Logger,
	cfg *proto.DriverConfig,
	descriptor *proto.InsertDescriptor,
	conn *pgx.Conn,
) error {
	driver, err := driver.Dispatch(ctx, lg, cfg)
	if err != nil {
		return fmt.Errorf("dispatch: %w", err)
	}
	driver.InsertValues(ctx, descriptor, 1000)
	return nil
}
