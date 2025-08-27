package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/stroppy-io/stroppy-core/pkg/logger"
	stroppy "github.com/stroppy-io/stroppy-core/pkg/proto"
)

type visitorInFunc func(
	msg protoreflect.Message,
	field protoreflect.FieldDescriptor,
	value protoreflect.Value,
)
type visitorOutFunc func(
	msg protoreflect.Message,
	field protoreflect.FieldDescriptor,
	value protoreflect.Value,
)

var ErrUnsupportedFieldKind = errors.New("unsupported field kind")

func traverseMessage(
	msg protoreflect.Message,
	inFunc visitorInFunc,
	outFunc visitorOutFunc,
) {
	md := msg.Descriptor()

	fields := md.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)

		value := msg.Get(field)
		// TODO: add support for List[Message], Map, probably Enums
		if (field.Kind() == protoreflect.MessageKind && !(field.IsList() || field.IsMap())) ||
			field.Kind() != protoreflect.MessageKind && !field.IsMap() {
			inFunc(msg, field, value)

			if field.Kind() == protoreflect.MessageKind {
				traverseMessage(value.Message(), inFunc, outFunc)
			}

			outFunc(msg, field, value)
		}
	}
}

func updateConfigWithDirectEnvs(config *stroppy.Config) error {
	var errTotal error

	log := logger.Global().WithOptions(zap.WithCaller(false))

	confPath := []string{"CONFIG"}

	traverseMessage(config.ProtoReflect(),
		func(msg protoreflect.Message, field protoreflect.FieldDescriptor, value protoreflect.Value) {
			confPath = append(confPath, field.TextName())
			// TODO: add support for lists of premitives at least
			if field.Kind() != protoreflect.MessageKind && !field.IsList() {
				envName := strings.Join(confPath, "__")
				envName = strings.ToUpper(envName)

				if newValueString, exists := os.LookupEnv(envName); exists {
					log.Debug("Config overridden",
						zap.String("env", envName),
						zap.String("old_value", value.String()),
						zap.String("new_value", newValueString),
					)

					newValue, err := convertStringToProtoValue(newValueString, field.Kind())
					if err != nil {
						errTotal = errors.Join(errTotal, fmt.Errorf(`env "%s" value "%s" is invalid: %w`, envName, newValueString, err))

						return
					}

					msg.Set(field, newValue)
				}
			}
		},
		func(_ protoreflect.Message, _ protoreflect.FieldDescriptor, _ protoreflect.Value) {
			confPath = confPath[:len(confPath)-1]
		})

	return errTotal
}

func ValidEnvsNames() []string {
	config := protoNew[*stroppy.Config]()

	var envs []string
	// TODO: deduplicate algorithm here and in updateConfigWithDirectEnvs somehow
	confPath := []string{"CONFIG"}

	traverseMessage(config.ProtoReflect(),
		func(_ protoreflect.Message, field protoreflect.FieldDescriptor, _ protoreflect.Value) {
			confPath = append(confPath, field.TextName())
			// TODO: add support for lists of premitives at least
			if field.Kind() != protoreflect.MessageKind && !field.IsList() {
				envName := strings.Join(confPath, "__")
				envName = strings.ToUpper(envName)
				envs = append(envs, envName)
			}
		},
		func(_ protoreflect.Message, _ protoreflect.FieldDescriptor, _ protoreflect.Value) {
			confPath = confPath[:len(confPath)-1]
		})

	return envs
}

func convertStringToProtoValue(value string, kind protoreflect.Kind) (protoreflect.Value, error) {
	switch kind {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(value), nil

	case protoreflect.BoolKind:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfBool(b), nil

	case protoreflect.Int32Kind:
		i, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfInt32(int32(i)), nil

	case protoreflect.Int64Kind:
		i, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfInt64(i), nil

	case protoreflect.Uint32Kind:
		i, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfUint32(uint32(i)), nil

	case protoreflect.Uint64Kind:
		i, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfUint64(i), nil

	case protoreflect.FloatKind:
		f, err := strconv.ParseFloat(value, 32)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfFloat32(float32(f)), nil

	case protoreflect.DoubleKind:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return protoreflect.Value{}, err
		}

		return protoreflect.ValueOfFloat64(f), nil

	default:
		return protoreflect.Value{},
			fmt.Errorf("%w: %s", ErrUnsupportedFieldKind, kind)
	}
}
