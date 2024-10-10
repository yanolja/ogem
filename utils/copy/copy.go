package copy

import "encoding/json"

func Deep[T any](src T) (T, error) {
	var dst T
	bytes, err := json.Marshal(src)
	if err != nil {
		return dst, err
	}
	if err := json.Unmarshal(bytes, &dst); err != nil {
		return dst, err
	}
	return dst, nil
}
