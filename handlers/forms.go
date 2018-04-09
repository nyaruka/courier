package handlers

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/schema"
	validator "gopkg.in/go-playground/validator.v9"
)

var (
	decoder  = schema.NewDecoder()
	validate = validator.New()
)

func init() {
	decoder.IgnoreUnknownKeys(true)
	decoder.SetAliasTag("name")
}

// Validate validates the passe din struct using our shared validator instance
func Validate(form interface{}) error {
	return validate.Struct(form)
}

// DecodeAndValidateForm takes the passed in form and attempts to parse and validate it from the
// URL query parameters as well as any POST parameters of the passed in request
func DecodeAndValidateForm(form interface{}, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	err = decoder.Decode(form, r.Form)
	if err != nil {
		return err
	}

	// check our input is valid
	err = validate.Struct(form)
	if err != nil {
		return err
	}

	return nil
}

// DecodeAndValidateJSON takes the passed in envelope and tries to unmarshal it from the body
// of the passed in request, then validating it
func DecodeAndValidateJSON(envelope interface{}, r *http.Request) error {
	// read our body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 100000))
	defer r.Body.Close()
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	// try to decode our envelope
	if err = json.Unmarshal(body, envelope); err != nil {
		return fmt.Errorf("unable to parse request JSON: %s", err)
	}

	// check our input is valid
	err = validate.Struct(envelope)
	if err != nil {
		return fmt.Errorf("request JSON doesn't match required schema: %s", err)
	}

	return nil
}

// DecodeAndValidateXML takes the passed in envelope and tries to unmarshal it from the body
// of the passed in request, then validating it
func DecodeAndValidateXML(envelope interface{}, r *http.Request) error {
	// read our body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 100000))
	defer r.Body.Close()
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	// try to decode our envelope
	if err = xml.Unmarshal(body, envelope); err != nil {
		return fmt.Errorf("unable to parse request XML: %s", err)
	}

	// check our input is valid
	err = validate.Struct(envelope)
	if err != nil {
		return fmt.Errorf("request XML doesn't match required schema: %s", err)
	}

	return nil
}
