package ezconf

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

func parseFlags(fs *flag.FlagSet, args []string) (map[string]ezValue, error) {
	values := make(map[string]ezValue)

	err := fs.Parse(args)
	if err != nil {
		return nil, err
	}

	// visit all our flags, populate a value for every value that isn't the default
	fs.Visit(func(flag *flag.Flag) {
		snake := strings.Replace(flag.Name, "-", "_", -1)
		if snake != "help" && snake != "debug_conf" {
			values[snake] = ezValue{flag.Name, flag.Value.String()}
		}
	})
	return values, nil
}

func buildFlags(name string, description string, fields *ezFields, errorHandling flag.ErrorHandling) *flag.FlagSet {
	flags := flag.NewFlagSet(name, errorHandling)

	// override our usage so we print out our description as well as our environment variables
	flags.Usage = func() {
		if description != "" {
			fmt.Fprint(flags.Output(), description)
			fmt.Fprint(flags.Output(), "\n\n")
		}
		fmt.Fprintf(flags.Output(), "Usage of %s:\n", name)
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output())
		fmt.Fprint(flags.Output(), buildEnvUsage(name, fields))
	}

	// add our default help and debug-conf flags
	flags.Bool("help", false, "print usage information")
	flags.Bool("debug-conf", false, "print where config values are coming from")

	// build a flag for each supported field
	for _, name := range fields.keys {
		f := fields.fields[name]

		// change underscores to dashes for flags
		flagName := strings.Replace(name, "_", "-", -1)
		help := f.Tag("help")
		if help == "" {
			help = fmt.Sprintf("set value for %s", name)
		}

		switch v := f.Value().(type) {
		case int:
			flags.Int(flagName, v, help)
		case int8:
			flags.Int64(flagName, int64(v), help)
		case int16:
			flags.Int64(flagName, int64(v), help)
		case int32:
			flags.Int64(flagName, int64(v), help)
		case int64:
			flags.Int64(flagName, v, help)

		case uint:
			flags.Uint(flagName, v, help)
		case uint8:
			flags.Uint64(flagName, uint64(v), help)
		case uint16:
			flags.Uint64(flagName, uint64(v), help)
		case uint32:
			flags.Uint64(flagName, uint64(v), help)
		case uint64:
			flags.Uint64(flagName, v, help)

		case float32:
			flags.Float64(flagName, float64(v), help)
		case float64:
			flags.Float64(flagName, v, help)

		case bool:
			flags.Bool(flagName, v, help)

		case string:
			flags.String(flagName, f.Value().(string), help)

		case time.Duration:
			flags.String(flagName, v.String(), help)
		}
	}

	return flags
}
