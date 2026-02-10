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
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan JSONMap: expected []byte, got %T", value)
	}
	return json.Unmarshal(bytes, j)
}
