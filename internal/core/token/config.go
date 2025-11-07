package token

import (
	"time"
)

type Config struct {
	HmacSecretKey string        `mapstructure:"hmac_secret_key" validate:"required"`
	Issuer        string        `mapstructure:"issuer" default:"stroppy" validate:"required"`
	LeewaySeconds time.Duration `mapstructure:"leeway" default:"30s" validate:"min=1s"`
	AccessExpire  time.Duration `mapstructure:"expire" default:"10m" validate:"min=1s"`
	RefreshExpire time.Duration `mapstructure:"refresh_expire" default:"30d" validate:"min=1h"`
}
