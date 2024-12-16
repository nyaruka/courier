package courier

import (
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/ezconf"
	"github.com/nyaruka/gocommon/httpx"
)

// Config is our top level configuration object
type Config struct {
	Backend   string `help:"the backend that will be used by courier (currently only rapidpro is supported)"`
	SentryDSN string `help:"the DSN used for logging errors to Sentry"`
	Domain    string `help:"the domain courier is exposed on"`
	Address   string `help:"the network interface address courier will bind to"`
	Port      int    `help:"the port courier will listen on"`
	DB        string `validate:"url,startswith=postgres:"   help:"URL for your Postgres database"`
	Redis     string `validate:"url,startswith=redis:"      help:"URL for your Redis instance"`
	SpoolDir  string `help:"the local directory where courier will write statuses or msgs that need to be retried (needs to be writable)"`

	AWSAccessKeyID     string `help:"access key ID to use for AWS services"`
	AWSSecretAccessKey string `help:"secret access key to use for AWS services"`
	AWSRegion          string `help:"region to use for AWS services, e.g. us-east-1"`

	CloudwatchNamespace string `help:"the namespace to use for cloudwatch metrics"`
	DeploymentID        string `help:"the deployment identifier to use for metrics"`
	InstanceID          string `help:"the instance identifier to use for metrics"`

	DynamoEndpoint    string `help:"DynamoDB service endpoint, e.g. https://dynamodb.us-east-1.amazonaws.com"`
	DynamoTablePrefix string `help:"prefix to use for DynamoDB tables"`

	S3Endpoint          string `help:"S3 service endpoint, e.g. https://s3.amazonaws.com"`
	S3AttachmentsBucket string `help:"S3 bucket to write attachments to"`
	S3Minio             bool   `help:"S3 is actually Minio or other compatible service"`

	FacebookApplicationSecret    string `help:"the Facebook app secret"`
	FacebookWebhookSecret        string `help:"the secret for Facebook webhook URL verification"`
	WhatsappAdminSystemUserToken string `help:"the token of the admin system user for WhatsApp"`

	DisallowedNetworks string     `help:"comma separated list of IP addresses and networks which we disallow fetching attachments from"`
	MediaDomain        string     `help:"the domain on which we'll try to resolve outgoing media URLs"`
	MaxWorkers         int        `help:"the maximum number of go routines that will be used for sending (set to 0 to disable sending)"`
	LibratoUsername    string     `help:"the username that will be used to authenticate to Librato"`
	LibratoToken       string     `help:"the token that will be used to authenticate to Librato"`
	StatusUsername     string     `help:"the username that is needed to authenticate against the /status endpoint"`
	StatusPassword     string     `help:"the password that is needed to authenticate against the /status endpoint"`
	AuthToken          string     `help:"the authentication token need to access non-channel endpoints"`
	LogLevel           slog.Level `help:"the logging level courier should use"`
	Version            string     `help:"the version that will be used in request and response headers"`

	// IncludeChannels is the list of channels to enable, empty means include all
	IncludeChannels []string

	// ExcludeChannels is the list of channels to exclude, empty means exclude none
	ExcludeChannels []string
}

// NewDefaultConfig returns a new default configuration object
func NewDefaultConfig() *Config {
	hostname, _ := os.Hostname()
	return &Config{
		Backend:  "rapidpro",
		Domain:   "localhost",
		Address:  "",
		Port:     8080,
		DB:       "postgres://temba:temba@localhost/temba?sslmode=disable",
		Redis:    "redis://localhost:6379/15",
		SpoolDir: "/var/spool/courier",

		AWSAccessKeyID:     "",
		AWSSecretAccessKey: "",
		AWSRegion:          "us-east-1",

		CloudwatchNamespace: "Temba",
		DeploymentID:        "dev",
		InstanceID:          hostname,

		DynamoEndpoint:    "", // let library generate it
		DynamoTablePrefix: "Temba",

		S3Endpoint:          "https://s3.amazonaws.com",
		S3AttachmentsBucket: "temba-attachments",
		S3Minio:             false,

		FacebookApplicationSecret:    "missing_facebook_app_secret",
		FacebookWebhookSecret:        "missing_facebook_webhook_secret",
		WhatsappAdminSystemUserToken: "missing_whatsapp_admin_system_user_token",

		DisallowedNetworks: `127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,fe80::/10`,
		MaxWorkers:         32,
		LogLevel:           slog.LevelWarn,
		Version:            "Dev",
	}
}

func LoadConfig() *Config {
	config := NewDefaultConfig()
	loader := ezconf.NewLoader(config, "courier", "Courier - A fast message broker for SMS and IP messages", []string{"config.toml"})
	loader.MustLoad()

	// ensure config is valid
	if err := config.Validate(); err != nil {
		log.Fatalf("invalid config: %s", err)
	}

	return config
}

// Validate validates the config
func (c *Config) Validate() error {
	if err := utils.Validate(c); err != nil {
		return err
	}

	if _, _, err := c.ParseDisallowedNetworks(); err != nil {
		return fmt.Errorf("unable to parse 'DisallowedNetworks': %w", err)
	}
	return nil
}

// ParseDisallowedNetworks parses the list of IPs and IP networks (written in CIDR notation)
func (c *Config) ParseDisallowedNetworks() ([]net.IP, []*net.IPNet, error) {
	addrs, err := csv.NewReader(strings.NewReader(c.DisallowedNetworks)).Read()
	if err != nil && err != io.EOF {
		return nil, nil, err
	}

	return httpx.ParseNetworks(addrs...)
}
