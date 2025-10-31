package configurator

import (
	"testing"
)

// DatabaseConfig содержит параметры базы данных
type DatabaseConfig struct {
	Host string `mapstructure:"host" validate:"required,hostname|ip" default:"localhost"`
	//Port     int    `mapstructure:"port" validate:"required,min=1024,max=65535" default:"5432"`
	//User     string `mapstructure:"user" validate:"required" default:"admin"`
	//Password string `mapstructure:"password" validate:"required"`
	//DBName   string `mapstructure:"dbname" validate:"required" default:"mydb"`
	Some string `mapstructure:"some_shit" validate:"required" default:"mydb"`
}

// Config структура для хранения конфигурации
type Config struct {
	AppName string `mapstructure:"app_name" validate:"required" default:"MyApp"`
	//Port     int            `mapstructure:"port" validate:"required,min=1024,max=65535" default:"8080"`
	//Debug    bool           `mapstructure:"debug" default:"false"`
	Database DatabaseConfig `mapstructure:"database" validate:"required"`
}

func TestLoadConfig_FromYAML(t *testing.T) {
	cfg := new(Config)
	ph := extractPaths(cfg)
	t.Log(ph)
	//config, err := LoadConfig[Config]() // config_test.yaml должен существовать
	//require.NoError(t, err)
	//
	//assert.Equal(t, "MyApp", config.AppName)
	//assert.Equal(t, 8080, config.Port)
	//assert.Equal(t, false, config.Debug)
	//assert.Equal(t, "localhost", config.Database.Host)
	//assert.Equal(t, 5433, config.Database.Port)
	//assert.Equal(t, "postgres", config.Database.User)
	//assert.Equal(t, "postgres", config.Database.Password)
	//assert.Equal(t, "postgres", config.Database.DBName)
}
