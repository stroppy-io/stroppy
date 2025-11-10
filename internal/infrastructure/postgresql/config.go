package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host         string `mapstructure:"host" default:"localhost" validate:"required,hostname|ip"`
	Port         int    `mapstructure:"port" default:"5432" validate:"required,min=1,max=65535"`
	Username     string `mapstructure:"username" validate:"required"`
	Password     string `mapstructure:"password" validate:"required"`
	Database     string `mapstructure:"database" validate:"required"`
	SSLMode      string `mapstructure:"ssl_mode" default:"disable" validate:"oneof=disable require verify_ca verify_full"`
	PoolMaxConns int    `mapstructure:"pool_max_conns" default:"10"`
	PoolMinConns int    `mapstructure:"pool_min_conns" default:"2"`
	LogLevel     string `mapstructure:"log_level" default:"info" validate:"oneof=debug info warn error"`
}

func (c *Config) String() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s&pool_max_conns=%d&pool_min_conns=%d",
		c.Username, c.Password, c.Host, c.Port, c.Database,
		c.SSLMode, c.PoolMaxConns, c.PoolMinConns,
	)
}

func (c *Config) Parse() (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(c.String())
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
