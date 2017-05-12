package utils

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// NullDict is a one level deep dictionary that is represented as JSON in the database
// and a string map as JSON
type NullDict struct {
	Dict  map[string]string
	Valid bool
}

// Scan implements the Scanner interface.
func (n NullDict) Scan(src interface{}) error {
	if src == nil {
		n.Valid = false
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

	n.Dict = make(map[string]string)
	n.Valid = true
	return json.Unmarshal(source, n.Dict)
}

// Value implements the driver Valuer interface.
func (n NullDict) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return json.Marshal(n.Dict)
}

// MarshalJSON returns the *j as the JSON encoding of our dict
func (n NullDict) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return nil, nil
	}
	return json.Marshal(n.Dict)
}

// UnmarshalJSON sets our dict from the passed in data
func (n NullDict) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	n.Dict = make(map[string]string)
	n.Valid = true
	return json.Unmarshal(data, n.Dict)
}
