package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONMap is a custom type for JSONB columns.
type JSONMap map[string]string

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = make(JSONMap)
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("failed to scan JSONMap: unsupported type %T", value)
	}
	return json.Unmarshal(bytes, j)
}

// StringSlice is a custom type for JSONB string array columns.
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("failed to scan StringSlice: unsupported type %T", value)
	}
	return json.Unmarshal(b, s)
}
