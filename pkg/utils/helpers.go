package utils //nolint:revive // package name is intentional

func Must[T any](value T, err error) T { //nolint: ireturn // generic
	if err != nil {
		panic(err)
	}

	return value
}

func StringOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}

	return value
}
