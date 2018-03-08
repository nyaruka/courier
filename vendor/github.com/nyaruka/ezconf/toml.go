package ezconf

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/naoina/toml"
)

// Iterates the list of files, parsing the first that is found and loading the
// result into the passed in struct pointer. If no files are passed in or
// no files are found, this is a noop.
func parseTOMLFiles(config interface{}, files []string, debug bool) error {
	// search through our list of files, stopping when we find one
	for i, file := range files {
		toml, err := ioutil.ReadFile(file)
		if err != nil {
			// not finding a file is ok, we just move on
			if os.IsNotExist(err) {
				if debug {
					fmt.Printf("CONF: Skipping missing TOML file: %s\n", file)
				}
				continue
			}
			return err
		}
		if debug {
			fmt.Printf("CONF: Parsing TOML file: %s\n", file)
		}
		decoder := newDecoder(bytes.NewReader(toml))
		err = decoder.Decode(config)

		// if we can't parse this file as TOML, that's a nogo
		if err != nil {
			return err
		}
		if debug {
			for i = i + 1; i < len(files); i++ {
				fmt.Printf("CONF: Previous file found, skipping TOML file: %s\n", files[i])
			}
		}

		// we break at the first file we find
		break
	}

	return nil
}

type ezTOMLConfig struct {
	*toml.Config
}

// We build our own decoder that uses our own CamelToSnake and is a bit stricter with
// matching of fields in our TOML file. (they must match CamelToSnake)
func newDecoder(r io.Reader) *toml.Decoder {
	tomlConfig := &toml.Config{
		NormFieldName: camelNormalizer,
		FieldToKey:    camelKey,
	}
	return tomlConfig.NewDecoder(r)
}

// Satisfies the NormFieldName interface and is used to match TOML keys to struct fields.
// The function runs for both input keys and struct field names and should return a string
// that makes the two match.
func camelNormalizer(typ reflect.Type, keyOrField string) string {
	// TODO: honor `name` tag if present
	return CamelToSnake(keyOrField)
}

// Satisfies the FieldToKey interface and determines the TOML key of a struct field when encoding.
//
// Note that FieldToKey is not used for fields which define a TOML key through the struct tag.
func camelKey(typ reflect.Type, field string) string {
	return CamelToSnake(field)
}
