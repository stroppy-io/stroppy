package defaults

func StringOrDefault(s string, defaultValue string) string {
	if s == "" {
		return defaultValue
	}
	return s
}

func StringPtrOrDefault(s *string, defaultValue string) string {
	if s == nil {
		return defaultValue
	}
	return *s
}
