package postgres

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host     string `mapstructure:"host" default:"localhost" validate:"required,hostname|ip"`
	Port     int    `mapstructure:"port" default:"5432" validate:"required,min=1,max=65535"`
	Username string `mapstructure:"username" validate:"required"`
	Password string `mapstructure:"password" validate:"required"`
	Database string `mapstructure:"database" validate:"required"`
	LogLevel string `mapstructure:"log_level" default:"info" validate:"oneof=debug info warn error"`
}

func (c *Config) String() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s",
		c.Username, c.Password, c.Host, c.Port, c.Database,
	)
}

func (c *Config) Parse() (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(c.String())
	if err != nil {
		return nil, err
	}
	return cfg, nil
}
