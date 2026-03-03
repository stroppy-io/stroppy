package common //nolint:revive // package name is intentional

// Out type params should be [T ~R], but it's not allowed by syntax.
func Out[R any, T any](xs []T) []R {
	res := make([]R, 0, len(xs))
	for _, x := range xs {
		res = append(res, any(x).(R)) //nolint:errcheck,forcetypeassert // allow panic
	}

	return res
}

func OutStr[R string, T ~string](xs []T) []R {
	res := make([]R, 0, len(xs))
	for _, x := range xs {
		res = append(res, R(x)) //nolint:errcheck,forcetypeassert // allow panic
	}

	return res
}
