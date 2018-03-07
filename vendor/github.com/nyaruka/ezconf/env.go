package ezconf

import (
	"fmt"
	"os"
	"strings"
)

func parseEnv(name string, fields *ezFields) map[string]ezValue {
	values := make(map[string]ezValue)
	for _, snake := range fields.keys {
		env := strings.ToUpper(fmt.Sprintf("%s_%s", name, snake))
		value := os.Getenv(env)
		if value != "" {
			values[snake] = ezValue{env, value}
		}
	}
	return values
}

func buildEnvUsage(name string, fields *ezFields) string {
	usage := strings.Builder{}
	usage.WriteString("Environment variables:\n")

	for _, snake := range fields.keys {
		f := fields.fields[snake]

		env := strings.ToUpper(fmt.Sprintf("%s_%s", name, snake))
		switch f.Value().(type) {
		case int, int8, int16, int32, int64:
			usage.WriteString(fmt.Sprintf("    % 40s - int\n", env))

		case uint, uint8, uint16, uint32, uint64:
			usage.WriteString(fmt.Sprintf("    % 40s - uint\n", env))

		case float32, float64:
			usage.WriteString(fmt.Sprintf("    % 40s - float\n", env))

		case bool:
			usage.WriteString(fmt.Sprintf("    % 40s - bool\n", env))

		case string:
			usage.WriteString(fmt.Sprintf("    % 40s - string\n", env))
		}
	}
	return usage.String()
}
