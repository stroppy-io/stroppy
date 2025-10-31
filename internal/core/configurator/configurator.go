package configurator

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"os"
	"reflect"
	"strings"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"github.com/stroppy-io/stroppy/pkg/core/logger"
)

const (
	FileName        = "config"
	FileExtension   = "yaml"
	FilePath        = "./"
	FilePathEnvName = "CONFIG_PATH"
)

func extractMapstructurePaths(i interface{}, prefix string, paths *[]string) {
	v := reflect.ValueOf(i)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" {
			continue
		}
		if field.Type.Kind() == reflect.Struct {
			extractMapstructurePaths(v.Field(i).Interface(), prefix+tag+".", paths)
		} else {
			fullPath := prefix + tag
			*paths = append(*paths, fullPath)
		}
	}
}

func extractPaths[T any](i T) map[string]string {
	var paths []string
	extractMapstructurePaths(i, "", &paths)
	ret := make(map[string]string)
	for _, path := range paths {
		ret[path] = strings.ReplaceAll(strings.ToUpper(path), ".", "_")
	}
	return ret
}

type loadOptions struct {
	filename string
	fileExt  string
	filePath string
}

type LoadOption func(options *loadOptions)

func newLoadOptions(opts ...LoadOption) *loadOptions {
	fp := os.Getenv(FilePathEnvName)
	if fp == "" {
		fp = FilePath
	}
	lOpt := &loadOptions{
		filename: FileName,
		fileExt:  FileExtension,
		filePath: fp,
	}
	for _, o := range opts {
		o(lOpt)
	}
	return lOpt
}

func WithFileName(filename string) LoadOption {
	return func(options *loadOptions) {
		options.filename = filename
	}
}
func WithFileExt(ext string) LoadOption {
	return func(options *loadOptions) {
		options.fileExt = ext
	}
}
func WithFilePath(path string) LoadOption {
	return func(options *loadOptions) {
		options.fileExt = path
	}
}

func LoadConfig[T any](opts ...LoadOption) (*T, error) {
	lOpts := newLoadOptions(opts...)
	// Загружаем переменные из .env файла, если он есть
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load .env: %w", err)
		}
	}

	v := viper.New()

	logger.NewStructLogger("configurator").Debug(
		"load config file",
		zap.String("filename", lOpts.filename),
		zap.String("fileExt", lOpts.fileExt),
		zap.String("filePath", lOpts.filePath),
	)
	// Задаем имя файла конфигурации (без расширения)
	v.SetConfigName(lOpts.filename)
	v.SetConfigType(lOpts.fileExt)
	v.AddConfigPath(lOpts.filePath)
	v.AutomaticEnv()

	// Читаем конфиг из файла, если он существует
	if err := v.ReadInConfig(); err != nil {
		var configFileNotFoundError viper.ConfigFileNotFoundError
		if !errors.As(err, &configFileNotFoundError) {
			return nil, err // Возвращаем ошибку, если она не связана с отсутствием файла
		}
	}
	// Переменные окружения
	//for _, s := range os.Environ() {
	//a := strings.SplitN(s, "=", 2)
	//err := v.BindEnv(strings.Replace(a[0], "_", ".", -1), a[0])
	//if err != nil {
	//	return nil, err
	//}
	//}
	// Заполняем структуру конфигурации
	config := new(T)
	for envPath, envKey := range extractPaths(config) {
		err := v.BindEnv(envPath, envKey)
		if err != nil {
			return nil, err
		}
	}
	err := defaults.Set(config) // Устанавливаем значения по умолчанию из тегов
	if err != nil {
		return nil, err
	}
	if err := v.Unmarshal(config); err != nil {
		return nil, err
	}

	// Валидируем конфиг
	validate := validator.New()
	err = validate.Struct(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}
