package util

func Uniq[T comparable](arr []T) []T {
	exists := map[T]struct{}{}

	results := make([]T, 0, len(arr))
	for _, a := range arr {
		_, ok := exists[a]

		if ok {
			continue
		}

		exists[a] = struct{}{}
		results = append(results, a)
	}

	return results
}
