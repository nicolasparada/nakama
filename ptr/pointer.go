package ptr

func From[T any](v T) *T {
	return &v
}

func Or[T any](v *T, defaultValue T) T {
	if v != nil {
		return *v
	}
	return defaultValue
}
