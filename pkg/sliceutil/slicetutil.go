package sliceutil

func Map[From any, To any](slice []From, fn func(v From) To) []To {
	mapped := make([]To, 0, len(slice))
	for i := range slice {
		mapped = append(mapped, fn(slice[i]))
	}

	return mapped
}
