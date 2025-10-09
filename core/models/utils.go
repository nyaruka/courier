package models

import (
	"database/sql/driver"
	"errors"
	"strings"
)

// StringArray is a string array that can be scanned from postgres arrays
type StringArray []string

// Scan implements the sql.Scanner interface
func (a *StringArray) Scan(value any) error {
	if value == nil {
		*a = nil
		return nil
	}

	switch v := value.(type) {
	case string:
		// Handle pgx string representation of array
		if v == "{}" || v == "" {
			*a = []string{}
			return nil
		}

		// Remove braces and split by comma
		if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
			v = v[1 : len(v)-1] // Remove { and }
		}

		if v == "" {
			*a = []string{}
			return nil
		}

		*a = strings.Split(v, ",")
		return nil
	case []any:
		// Handle array of interfaces
		result := make([]string, len(v))
		for i, item := range v {
			if item == nil {
				result[i] = ""
			} else {
				result[i] = item.(string)
			}
		}
		*a = result
		return nil
	}

	return errors.New("cannot scan into StringArray")
}

// Value implements the driver.Valuer interface
func (a StringArray) Value() (driver.Value, error) {
	if a == nil {
		return nil, nil
	}

	if len(a) == 0 {
		return "{}", nil
	}

	return "{" + strings.Join(a, ",") + "}", nil
}
