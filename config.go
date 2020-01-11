package courier

import "github.com/nyaruka/ezconf"

// Config is our top level configuration object
type Config struct {
	Backend            string `help:"the backend that will be used by courier (currently only rapidpro is supported)"`
	SentryDSN          string `help:"the DSN used for logging errors to Sentry"`
	Domain             string `help:"the domain courier is exposed on"`
	Address            string `help:"the network interface address courier will bind to"`
	Port               int    `help:"the port courier will listen on"`
	DB                 string `help:"URL describing how to connect to the RapidPro database"`
	Redis              string `help:"URL describing how to connect to Redis"`
	SpoolDir           string `help:"the local directory where courier will write statuses or msgs that need to be retried (needs to be writable)"`
	S3Endpoint         string `help:"the S3 endpoint we will write attachments to"`
	S3Region           string `help:"the S3 region we will write attachments to"`
	S3MediaBucket      string `help:"the S3 bucket we will write attachments to"`
	S3MediaPrefix      string `help:"the prefix that will be added to attachment filenames"`
	S3DisableSSL       bool   `help:"whether we disable SSL when accessing S3. Should always be set to False unless you're hosting an S3 compatible service within a secure internal network"`
	S3ForcePathStyle   bool   `help:"whether we force S3 path style. Should generally need to default to False unless you're hosting an S3 compatible service"`
	AWSAccessKeyID     string `help:"the access key id to use when authenticating S3"`
	AWSSecretAccessKey string `help:"the secret access key id to use when authenticating S3"`
	MaxWorkers         int    `help:"the maximum number of go routines that will be used for sending (set to 0 to disable sending)"`
	LibratoUsername    string `help:"the username that will be used to authenticate to Librato"`
	LibratoToken       string `help:"the token that will be used to authenticate to Librato"`
	StatusUsername     string `help:"the username that is needed to authenticate against the /status endpoint"`
	StatusPassword     string `help:"the password that is needed to authenticate against the /status endpoint"`
	LogLevel           string `help:"the logging level courier should use"`
	Version            string `help:"the version that will be used in request and response headers"`

	// IncludeChannels is the list of channels to enable, empty means include all
	IncludeChannels []string

	// ExcludeChannels is the list of channels to exclude, empty means exclude none
	ExcludeChannels []string
}

// NewConfig returns a new default configuration object
func NewConfig() *Config {
	return &Config{
		Backend:            "rapidpro",
		Domain:             "localhost",
		Address:            "",
		Port:               8080,
		DB:                 "postgres://courier@localhost/courier?sslmode=disable",
		Redis:              "redis://localhost:6379/0",
		SpoolDir:           "/var/spool/courier",
		S3Endpoint:         "https://s3.amazonaws.com",
		S3Region:           "us-east-1",
		S3MediaBucket:      "courier-media",
		S3MediaPrefix:      "/media/",
		S3DisableSSL:       false,
		S3ForcePathStyle:   false,
		AWSAccessKeyID:     "missing_aws_access_key_id",
		AWSSecretAccessKey: "missing_aws_secret_access_key",
		MaxWorkers:         32,
		LogLevel:           "error",
		Version:            "Dev",
	}
}

// LoadConfig loads our configuration from the passed in filename
func LoadConfig(filename string) *Config {
	config := NewConfig()
	loader := ezconf.NewLoader(
		config,
		"courier", "Courier - A fast message broker for SMS and IP messages",
		[]string{filename},
	)

	loader.MustLoad()
	return config
}
