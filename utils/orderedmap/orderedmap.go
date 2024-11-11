package orderedmap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

type Map struct {
	order []string
	data  map[string]any
}

type Entry struct {
	Key   string
	Value any
}

func New() *Map {
	return &Map{
		order: make([]string, 0),
		data:  make(map[string]any),
	}
}

func (n *Map) Keys() []string {
	return n.order
}

func (n *Map) Entries() []Entry {
	entries := make([]Entry, 0, len(n.order))
	for _, key := range n.order {
		entries = append(entries, Entry{
			Key:   key,
			Value: n.data[key],
		})
	}
	return entries
}

func (n *Map) Set(key string, value any) {
	if _, exists := n.data[key]; !exists {
		n.order = append(n.order, key)
	}
	n.data[key] = value
}

func (n *Map) Get(key string) (any, bool) {
	value, exists := n.data[key]
	return value, exists
}

func (n *Map) MarshalJSON() ([]byte, error) {
	result := "{"
	for i, key := range n.order {
		if i > 0 {
			result += ","
		}
		quotedKey := strconv.Quote(key)
		result += quotedKey + ":"
		if nested, ok := n.data[key].(*Map); ok {
			nestedJSON, err := nested.MarshalJSON()
			if err != nil {
				return nil, err
			}
			result += string(nestedJSON)
		} else {
			valueJSON, err := json.Marshal(n.data[key])
			if err != nil {
				return nil, err
			}
			result += string(valueJSON)
		}
	}
	result += "}"
	return []byte(result), nil
}

func (n *Map) UnmarshalJSON(data []byte) error {
	n.order = make([]string, 0)
	n.data = make(map[string]any)

	jsonDecoder := json.NewDecoder(bytes.NewReader(data))

	// Consume the opening brace.
	if _, err := jsonDecoder.Token(); err != nil {
		return err
	}

	for jsonDecoder.More() {
		keyToken, err := jsonDecoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %T", keyToken)
		}

		var value json.RawMessage
		if err := jsonDecoder.Decode(&value); err != nil {
			return err
		}

		var parsed any
		if err := json.Unmarshal(value, &parsed); err != nil {
			return err
		}

		if _, ok := parsed.(map[string]any); ok {
			nested := New()
			if err := nested.UnmarshalJSON(value); err != nil {
				return err
			}
			n.Set(key, nested)
		} else {
			n.Set(key, parsed)
		}
	}

	// Consume the closing brace.
	if _, err := jsonDecoder.Token(); err != nil {
		return err
	}

	return nil
}
