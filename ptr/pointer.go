package ptr

func From[T any](v T) *T {
	return &v
}
