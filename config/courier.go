package config

import (
	"github.com/koding/multiconfig"
)

// Courier is our top level configuration object
type Courier struct {
	// Backend is the backend that will be used by courier (currently only rapidpro is supported)
	Backend string `default:"rapidpro"`

	// SentryDSN is the DSN used for logging errors to Sentry
	SentryDSN string `default:""`

	// Domain is the domain courier is exposed on
	Domain string `default:"localhost"`

	// Port is the port courier will listen on
	Port int `default:"8080"`

	// DB is a URL describing how to connect to our database
	DB string `default:"postgres://courier@localhost/courier?sslmode=disable"`

	// Redis is a URL describing how to connect to Redis
	Redis string `default:"redis://localhost:6379/0"`

	// SpoolDir is the local directory where courier will write statuses or msgs that need to be retried (needs to be writable)
	SpoolDir string `default:"/var/spool/courier"`

	// S3Endpoint is the S3 endpoint we will write attachments to
	S3Endpoint string `default:"https://s3.amazonaws.com"`

	// S3Region is the S3 region we will write attachments to
	S3Region string `default:"us-east-1"`

	// S3DisableSSL should always be set to False unless you're hosting an S3 compatible service within a secure internal network
	S3DisableSSL bool

	// S3ForcePathStyle will generally need to default to False unless you're hosting an S3 compatible service
	S3ForcePathStyle bool

	// S3MediaBucket is the S3 bucket we will write attachments to
	S3MediaBucket string `default:"courier-media"`

	// S3MediaPrefix is the prefix that will be added to attachment filenames
	S3MediaPrefix string `default:"/media/"`

	// AWSAccessKeyID is the access key id to use when authenticating S3
	AWSAccessKeyID string `default:"missing_aws_access_key_id"`

	// AWSAccessKeyID is the secret access key id to use when authenticating S3
	AWSSecretAccessKey string `default:"missing_aws_secret_access_key"`

	// MaxWorkers it the maximum number of go routines that will be used for sending (set to 0 to disable sending)
	MaxWorkers int `default:"32"`

	// LibratoUsername is the username that will be used to authenticate to Librato
	LibratoUsername string `default:""`

	// LibratoToken is the token that will be used to authenticate to Librato
	LibratoToken string `default:""`

	// StatusUsername is the username that is needed to authenticate against the /status endpoint
	StatusUsername string `default:""`

	// StatusPassword is the password that is needed to authenticate against the /status endpoint
	StatusPassword string `default:""`

	// LogLevel controls the logging level courier uses
	LogLevel string `default:"error"`

	// IgnoreDeliveryReports controls whether we ignore delivered status reports (errors will still be handled)
	IgnoreDeliveryReports bool `default:"false"`

	// IncludeChannels is the list of channels to enable, empty means include all
	IncludeChannels []string

	// ExcludeChannels is the list of channels to exclude, empty means exclude none
	ExcludeChannels []string

	// Version is the version that will be sent in headers
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
