package runtime

import (
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/url"
	"os"

	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/ezconf"
	"github.com/nyaruka/gocommon/httpx"
)

// Config is our top level configuration object
type Config struct {
	DB        string `validate:"url,startswith=postgres:"   help:"URL for your Postgres database"`
	Valkey    string `validate:"url,startswith=valkey:"     help:"URL for your Valkey instance"`
	SpoolDir  string `help:"the local directory where courier will write statuses or msgs that need to be retried (needs to be writable)"`
	SentryDSN string `help:"the DSN used for logging errors to Sentry"`

	Domain          string `help:"the domain courier is exposed on"`
	PublicAddress   string `help:"the network interface address our public web server will bind to"`
	PublicPort      int    `help:"the port our public web server will listen on"`
	InternalAddress string `help:"the network interface address our internal web server will bind to"`
	InternalPort    int    `help:"the port our internal web server will listen on"`

	AWSAccessKeyID     string `help:"access key ID to use for AWS services"`
	AWSSecretAccessKey string `help:"secret access key to use for AWS services"`
	AWSRegion          string `help:"region to use for AWS services, e.g. us-east-1"`

	MetricsReporting    string `validate:"eq=off|eq=basic|eq=advanced"     help:"the level of metrics reporting"`
	CloudwatchNamespace string `help:"the namespace to use for cloudwatch metrics"`
	DeploymentID        string `help:"the deployment identifier to use for metrics"`
	InstanceID          string `help:"the instance identifier to use for metrics"`

	DynamoEndpoint    string `help:"DynamoDB service endpoint, e.g. https://dynamodb.us-east-1.amazonaws.com"`
	DynamoTablePrefix string `help:"prefix to use for DynamoDB tables"`

	S3Endpoint          string `help:"S3 service endpoint, e.g. https://s3.amazonaws.com"`
	S3AttachmentsBucket string `help:"S3 bucket to write attachments to"`
	S3PathStyle         bool   `help:"S3 should use path style URLs"`

	FacebookApplicationSecret    string `help:"the Facebook app secret"`
	FacebookWebhookSecret        string `help:"the secret for Facebook webhook URL verification"`
	WhatsappAdminSystemUserToken string `help:"the token of the admin system user for WhatsApp"`

	DisallowedNetworks []string   `help:"list of IP addresses and networks (CIDR notation) which we disallow making outgoing HTTP requests to"`
	SendProxyURL       string     `validate:"omitempty,http_url" help:"optional URL of a forward HTTP proxy for handlers that send to user-configured URLs"`
	MediaDomain        string     `help:"the domain on which we'll try to resolve outgoing media URLs"`
	MaxWorkers         int        `help:"the maximum number of go routines that will be used for sending (set to 0 to disable sending)"`
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
		DB:       "postgres://temba:temba@postgres/temba?sslmode=disable",
		Valkey:   "valkey://valkey:6379/15",
		SpoolDir: "/var/spool/courier",

		Domain:          "localhost",
		PublicAddress:   "",
		PublicPort:      8080,
		InternalAddress: "localhost",
		InternalPort:    8081,

		AWSAccessKeyID:     "",
		AWSSecretAccessKey: "",
		AWSRegion:          "us-east-1",

		MetricsReporting:    "off",
		CloudwatchNamespace: "Courier",
		DeploymentID:        "dev",
		InstanceID:          hostname,

		DynamoEndpoint:    "", // let library generate it
		DynamoTablePrefix: "Temba",

		S3Endpoint:          "https://s3.amazonaws.com",
		S3AttachmentsBucket: "temba-attachments",
		S3PathStyle:         false,

		FacebookApplicationSecret:    "missing_facebook_app_secret",
		FacebookWebhookSecret:        "missing_facebook_webhook_secret",
		WhatsappAdminSystemUserToken: "missing_whatsapp_admin_system_user_token",

		DisallowedNetworks: []string{`127.0.0.0/8`, `::1`, `fe80::/10`, `fc00::/7`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `100.64.0.0/10`, `169.254.0.0/16`, `0.0.0.0/8`},
		MaxWorkers:         32,
		LogLevel:           slog.LevelWarn,
		Version:            "Dev",
	}
}

func LoadConfig() *Config {
	cfg := NewDefaultConfig()
	loader := ezconf.NewLoader(cfg, "courier", "Courier - A fast message broker for SMS and IP messages", []string{"config.toml"})
	loader.MustLoad()

	// ensure config is valid
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %s", err)
	}

	return cfg
}

// Validate validates the config
func (c *Config) Validate() error {
	if err := utils.Validate(c); err != nil {
		return err
	}

	if _, _, err := c.ParseDisallowedNetworks(); err != nil {
		return fmt.Errorf("unable to parse 'DisallowedNetworks': %w", err)
	}

	if _, err := c.ParseSendProxyURL(); err != nil {
		return fmt.Errorf("unable to parse 'SendProxyURL': %w", err)
	}
	return nil
}

// ParseDisallowedNetworks parses the list of IPs and IP networks (written in CIDR notation)
func (c *Config) ParseDisallowedNetworks() ([]net.IP, []*net.IPNet, error) {
	return httpx.ParseNetworks(c.DisallowedNetworks...)
}

// ParseSendProxyURL parses SendProxyURL. Returns (nil, nil) when SendProxyURL is empty.
func (c *Config) ParseSendProxyURL() (*url.URL, error) {
	if c.SendProxyURL == "" {
		return nil, nil
	}
	return url.Parse(c.SendProxyURL)
}
