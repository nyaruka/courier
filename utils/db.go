package utils

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// NewNullMap creates a new null map with the passed in map
func NewNullMap(validMap map[string]interface{}) NullMap {
	return NullMap{Map: validMap, Valid: true}
}

// NullMap is a one level deep dictionary that is represented as JSON in the database
type NullMap struct {
	Map   map[string]interface{}
	Valid bool
}

// Scan implements the Scanner interface for decoding from a database
func (n *NullMap) Scan(src interface{}) error {
	if src == nil {
		return nil
	}

	var source []byte
	switch src.(type) {
	case string:
		source = []byte(src.(string))
	case []byte:
		source = src.([]byte)
	default:
		return errors.New("Incompatible type for NullDict")
	}

	// 0 length is same as nil
	if len(source) == 0 {
		return nil
	}

	n.Map = make(map[string]interface{})
	n.Valid = true
	return json.Unmarshal(source, &n.Map)
}

// Value implements the driver Valuer interface
func (n *NullMap) Value() (driver.Value, error) {
	if n == nil {
		return nil, nil
	}

	if !n.Valid {
		return nil, nil
	}

	if len(n.Map) == 0 {
		return nil, nil
	}
	return json.Marshal(n.Map)
}

// MarshalJSON decodes our dictionary from the passed in bytes
func (n *NullMap) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return json.Marshal(nil)
	}
	return json.Marshal(n.Map)
}

// UnmarshalJSON sets our dict from the passed in data
func (n *NullMap) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	n.Map = make(map[string]interface{})
	n.Valid = true
	return json.Unmarshal(data, &n.Map)
}
