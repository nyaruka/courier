package config

import "github.com/koding/multiconfig"

// NewTest returns a new instance of our config initialized just from our defaults as defined above
func NewTest() *Courier {
	loader := &multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(&multiconfig.TagLoader{}),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}

	config := &Courier{}
	loader.Load(config)

	// hardcode our test base URL
	config.BaseURL = "http://courier.test"
	return config
}
