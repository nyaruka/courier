package handlers

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/schema"
	"github.com/nyaruka/courier/utils"
)

const maxBodyReadBytes = 1024 * 1024 // 1MB

var (
	decoder = schema.NewDecoder()
)

func init() {
	decoder.IgnoreUnknownKeys(true)
	decoder.SetAliasTag("name")
}

// DecodeAndValidateForm takes the passed in form and attempts to parse and validate it from the
// URL query parameters as well as any POST parameters of the passed in request
func DecodeAndValidateForm(form any, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	err = decoder.Decode(form, r.Form)
	if err != nil {
		return err
	}

	// check our input is valid
	err = utils.Validate(form)
	if err != nil {
		return err
	}

	return nil
}

// DecodeAndValidateJSON takes the passed in envelope and tries to unmarshal it from the body
// of the passed in request, then validating it
func DecodeAndValidateJSON(envelope any, r *http.Request) error {
	body, err := ReadBody(r, maxBodyReadBytes)
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	// try to decode our envelope
	if err = json.Unmarshal(body, envelope); err != nil {
		return fmt.Errorf("unable to parse request JSON: %s", err)
	}

	// check our input is valid
	err = utils.Validate(envelope)
	if err != nil {
		return fmt.Errorf("request JSON doesn't match required schema: %s", err)
	}

	return nil
}

// DecodeAndValidateXML takes the passed in envelope and tries to unmarshal it from the body
// of the passed in request, then validating it
func DecodeAndValidateXML(envelope any, r *http.Request) error {
	body, err := ReadBody(r, maxBodyReadBytes)
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	// try to decode our envelope
	if err = xml.Unmarshal(body, envelope); err != nil {
		return fmt.Errorf("unable to parse request XML: %s", err)
	}

	// check our input is valid
	err = utils.Validate(envelope)
	if err != nil {
		return fmt.Errorf("request XML doesn't match required schema: %s", err)
	}

	return nil
}

// ReadBody of a HTTP request up to limit bytes
func ReadBody(r *http.Request, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, limit))

	// reset body so it can be read again
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	return body, err

}
