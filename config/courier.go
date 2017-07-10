package config

import (
	"github.com/koding/multiconfig"
)

// Courier is our top level configuration object
type Courier struct {
	Backend string `default:"rapidpro"`

	SentryDSN string `default:""`

	BaseURL  string `default:"https://localhost:8080"`
	Port     int    `default:"8080"`
	DB       string `default:"postgres://courier@localhost/courier?sslmode=disable"`
	Redis    string `default:"redis://localhost:6379/0"`
	SpoolDir string `default:"/var/spool/courier"`

	S3Region      string `default:"us-east-1"`
	S3MediaBucket string `default:"courier-media"`
	S3MediaPrefix string `default:"/media/"`

	AWSAccessKeyID     string `default:"missing_aws_access_key_id"`
	AWSSecretAccessKey string `default:"missing_aws_secret_access_key"`

	MaxWorkers int `default:"32"`

	RapidproHandleURL string `default:"https://app.rapidpro.io/handlers/mage/handle_message"`
	RapidproToken     string `default:"missing_rapidpro_token"`

	LogLevel string `default:"error"`

	IncludeChannels []string
	ExcludeChannels []string

	Version string `default:"Dev"`
}

// NewWithPath returns a new instance of Loader to read from the given configuration file using our config options
func NewWithPath(path string) *multiconfig.DefaultLoader {
	loaders := []multiconfig.Loader{}

	loaders = append(loaders, &multiconfig.TagLoader{})
	loaders = append(loaders, &multiconfig.TOMLLoader{Path: path})
	loaders = append(loaders, &multiconfig.EnvironmentLoader{CamelCase: true})
	loaders = append(loaders, &multiconfig.FlagLoader{CamelCase: true})
	loader := multiconfig.MultiLoader(loaders...)

	return &multiconfig.DefaultLoader{Loader: loader, Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{})}
}
