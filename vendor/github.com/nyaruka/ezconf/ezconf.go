package ezconf

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/fatih/structs"
)

// CamelToSnake convers a CamelCase strings to a snake_case using the following algorithm:
//  1) for every transition from upper->lowercase insert an underscore before the uppercase character
//  2) for every transition fro lowercase->uppercase insert an underscore before the uppercase
//  3) lowercase resulting string
//
//  Examples:
//      CamelCase -> camel_case
//      AWSConfig -> aws_config
//      IPAddress -> ip_address
//      S3MediaPrefix -> s3_media_prefix
//      Route53Region -> route53_region
//      CamelCaseA -> camel_case_a
//      CamelABCCaseDEF -> camel_abc_case_def
//
func CamelToSnake(camel string) string {
	snakes := make([]string, 0, 4)
	snake := strings.Builder{}
	runes := []rune(camel)

	// two transitions:
	//    we are upper, next is lower
	//    we are lower, next is upper
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		hasNext := i+1 < len(runes)

		if snake.Len() == 0 {
			// no snake, just append to it
			snake.WriteRune(r)
		} else if r == '_' {
			// if we are at an underscore, that's a boundary, create a snake but ignore the underscore
			snakes = append(snakes, snake.String())
			snake.Reset()
		} else if unicode.IsLower(r) && hasNext && unicode.IsUpper(runes[i+1]) {
			// if we are lowercase and the next item is uppercase, that's a transtion
			snake.WriteRune(r)
			snakes = append(snakes, snake.String())
			snake.Reset()
		} else if unicode.IsUpper(r) && hasNext && unicode.IsLower(runes[i+1]) {
			// if we are uppercase and the next item is lowercase, that's a transition
			snakes = append(snakes, snake.String())
			snake.Reset()
			snake.WriteRune(r)
		} else {
			// otherwise, add to our current snake
			snake.WriteRune(r)
		}
	}

	// if we have a trailing snake, add it
	if snake.Len() > 0 {
		snakes = append(snakes, snake.String())
	}

	// join everything together with _ and lowercase
	return strings.ToLower(strings.Join(snakes, "_"))
}

// EZLoader allows you to load your configuration from four sources, in order of priority (later overrides ealier):
//  1. The default values of your configuration struct
//  2. TOML files you specify (optional)
//  3. Set environment variables
//  4. Command line parameters
//
type EZLoader struct {
	name        string
	description string
	config      interface{}
	files       []string

	// overriden in tests
	args []string

	// we hang onto this to print usage where needed
	flags *flag.FlagSet
}

// NewLoader creates a new EZLoader for the passed in configuration. `config` should be a pointer to a struct.
// `name` and `description` are used to build environment variables and help parameters. The list of files
// can be nil, or can contain optional files to read TOML configuration from in priority order. The first file
// found and parsed will end parsing of others, but there is no requirement that any file is found.
//
func NewLoader(config interface{}, name string, description string, files []string) *EZLoader {
	return &EZLoader{
		name:        name,
		description: description,
		config:      config,
		files:       files,
		args:        os.Args[1:],
	}
}

// MustLoad loads our configuration from our sources in the order of:
//   1. TOML files
//   2. Environment variables
//   3. Command line parameters
//
// If any error is encountered, the program will exit reporting the error and showing usage.
//
func (ez *EZLoader) MustLoad() {
	err := ez.Load()
	if err != nil {
		fmt.Printf("Error while reading configuration: %s\n\n", err.Error())
		ez.flags.Usage()
		os.Exit(1)
	}
}

// Load loads our configuration from our sources in the order of:
//   1. TOML files
//   2. Environment variables
//   3. Command line parameters
//
// If any error is encountered it is returned for the caller to process.
//
func (ez *EZLoader) Load() error {
	// first build our mapping of name snake_case -> structs.Field
	fields, err := buildFields(ez.config)
	if err != nil {
		return err
	}

	// build our flags
	ez.flags = buildFlags(ez.name, ez.description, fields, flag.ExitOnError)

	// parse them
	flagValues, err := parseFlags(ez.flags, ez.args)
	if err != nil {
		return err
	}

	// if they asked for usage, show it
	if ez.flags.Lookup("help").Value.String() == "true" {
		ez.flags.Usage()
		os.Exit(1)
	}

	// if they asked for config debug, show it
	debug := false
	if ez.flags.Lookup("debug-conf").Value.String() == "true" {
		debug = true
	}

	if debug {
		printFields("Default overridable values:", fields)
	}

	// read any found file into our config
	err = parseTOMLFiles(ez.config, ez.files, debug)
	if err != nil {
		return err
	}

	if debug {
		printFields("Overridable values after TOML parsing:", fields)
	}

	// parse our environment
	envValues := parseEnv(ez.name, fields)
	err = setValues(fields, envValues)
	if err != nil {
		return err
	}

	// set our flag values
	err = setValues(fields, flagValues)
	if err != nil {
		return err
	}

	if debug {
		printValues("Command line overrides:", flagValues)
		printValues("Environment overrides:", envValues)
		printFields("Final top level values:", fields)
	}

	return nil
}

func printFields(header string, fields *ezFields) {
	fmt.Printf("CONF: %s\n", header)
	for _, k := range fields.keys {
		field := fields.fields[k]
		fmt.Printf("CONF: % 40s = %v\n", field.Name(), field.Value())
	}
	fmt.Println()
}

func printValues(header string, values map[string]ezValue) {
	fmt.Printf("CONF: %s\n", header)
	for _, v := range values {
		fmt.Printf("CONF: % 40s = %s\n", v.rawKey, v.value)
	}
	fmt.Println()
}

func setValues(fields *ezFields, values map[string]ezValue) error {
	// iterates all passed in values, attempting to set them, returning an error if
	// there are any type mismatches
	for name, cValue := range values {
		value := cValue.value

		f, found := fields.fields[name]
		if !found {
			return fmt.Errorf("unknown key '%s' for value '%s'", name, value)
		}

		switch f.Value().(type) {
		case int:
			i, err := strconv.ParseInt(value, 10, strconv.IntSize)
			if err != nil {
				return err
			}
			f.Set(int(i))
		case int8:
			i, err := strconv.ParseInt(value, 10, 8)
			if err != nil {
				return err
			}
			f.Set(int8(i))
		case int16:
			i, err := strconv.ParseInt(value, 10, 16)
			if err != nil {
				return err
			}
			f.Set(int16(i))
		case int32:
			i, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return err
			}
			f.Set(int32(i))
		case int64:
			i, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			f.Set(int64(i))

		case uint:
			i, err := strconv.ParseUint(value, 10, strconv.IntSize)
			if err != nil {
				return err
			}
			f.Set(uint(i))

		case uint8:
			i, err := strconv.ParseUint(value, 10, 8)
			if err != nil {
				return err
			}
			f.Set(uint8(i))
		case uint16:
			i, err := strconv.ParseUint(value, 10, 16)
			if err != nil {
				return err
			}
			f.Set(uint16(i))
		case uint32:
			i, err := strconv.ParseUint(value, 10, 32)
			if err != nil {
				return err
			}
			f.Set(uint32(i))
		case uint64:
			i, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return err
			}
			f.Set(uint64(i))

		case float32:
			d, err := strconv.ParseFloat(value, 32)
			if err != nil {
				return err
			}
			f.Set(float32(d))
		case float64:
			d, err := strconv.ParseFloat(value, 32)
			if err != nil {
				return err
			}
			f.Set(float64(d))

		case bool:
			b, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			f.Set(b)

		case string:
			f.Set(value)
		}
	}
	return nil
}

func buildFields(config interface{}) (*ezFields, error) {
	fields := make(map[string]*structs.Field)
	s := structs.New(config)
	for _, f := range s.Fields() {
		if f.IsExported() {
			switch f.Value().(type) {
			case int, int8, int16, int32, int64,
				uint, uint8, uint16, uint32, uint64,
				float32, float64,
				bool,
				string:
				name := CamelToSnake(f.Name())
				dupe, found := fields[name]
				if found {
					return nil, fmt.Errorf("%s name collides with %s", dupe.Name(), f.Name())
				}
				fields[name] = f
			}
		}
	}

	// build our keys and sort them
	keys := make([]string, 0)
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return &ezFields{keys, fields}, nil
}

// utility struct for holding the snaked key, raw key (env all caps or flag) along with a read value
type ezValue struct {
	rawKey string
	value  string
}

// utility struct that holds our fields and an ordered list of the keys for predictable iteration
type ezFields struct {
	keys   []string
	fields map[string]*structs.Field
}
