package runtime

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"

	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/ezconf"
	"github.com/nyaruka/gocommon/httpx"
)

// Config is our top level configuration object
type Config struct {
	DB       string `validate:"url,startswith=postgres:"   help:"URL for your Postgres database"`
	Valkey   string `validate:"url,startswith=valkey:|startswith=valkeys:" help:"URL for your Valkey instance, valkeys:// for TLS"`
	SpoolDir string `help:"the local directory where courier will write statuses or msgs that need to be retried (needs to be writable)"`

	Domain          string `help:"the domain courier is exposed on"`
	InternetAddress string `help:"the address our internet facing web server will bind to, empty means all interfaces"`
	InternetPort    int    `help:"the port our internet facing web server will listen on"`
	InternalAddress string `help:"the address our internal web server will bind to, empty means all interfaces"`
	InternalPort    int    `help:"the port our internal web server will listen on"`

	MetricsReporting    string `validate:"eq=off|eq=basic|eq=advanced"     help:"the level of metrics reporting"`
	CloudwatchNamespace string `help:"the namespace to use for cloudwatch metrics"`
	DeploymentID        string `help:"the deployment identifier to use for metrics"`

	DynamoEndpoint    string `help:"DynamoDB service endpoint, e.g. https://dynamodb.us-east-1.amazonaws.com"`
	DynamoTablePrefix string `help:"prefix to use for DynamoDB tables"`

	S3Endpoint          string `help:"S3 service endpoint, e.g. https://s3.amazonaws.com"`
	S3AttachmentsBucket string `help:"S3 bucket to write attachments to"`
	S3PathStyle         bool   `help:"S3 should use path style URLs"`

	CentrifugoEndpoint string `validate:"url" help:"the endpoint of the Centrifugo server"`
	CentrifugoKey      string `help:"the API key for the Centrifugo server"`

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

	// parsed values that can't be set directly
	DisallowedIPs      []net.IP
	DisallowedNets     []*net.IPNet
	SendProxyURLParsed *url.URL
}

// NewDefaultConfig returns a new default configuration object
func NewDefaultConfig() *Config {
	return &Config{
		DB:       "postgres://temba:temba@postgres/temba?sslmode=disable",
		Valkey:   "valkey://valkey:6379/15",
		SpoolDir: "./_spool",

		Domain:          "localhost",
		InternetAddress: "",
		InternetPort:    8080,
		InternalAddress: "",
		InternalPort:    8081,

		MetricsReporting:    "off",
		CloudwatchNamespace: "Courier",
		DeploymentID:        "dev",

		DynamoEndpoint:    "", // let library generate it
		DynamoTablePrefix: "Temba",

		S3Endpoint:          "https://s3.amazonaws.com",
		S3AttachmentsBucket: "temba-attachments",
		S3PathStyle:         false,

		CentrifugoEndpoint: "http://localhost:8000/api",

		FacebookApplicationSecret:    "missing_facebook_app_secret",
		FacebookWebhookSecret:        "missing_facebook_webhook_secret",
		WhatsappAdminSystemUserToken: "missing_whatsapp_admin_system_user_token",

		DisallowedNetworks: []string{`127.0.0.0/8`, `::1`, `fe80::/10`, `fc00::/7`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `100.64.0.0/10`, `169.254.0.0/16`, `0.0.0.0/8`},
		MaxWorkers:         32,
		LogLevel:           slog.LevelWarn,
		Version:            "Dev",
	}
}

// LoadConfig loads configuration from a config file, environment variables and command line args, on top of the
// given base config, e.g. NewDefaultConfig().
func LoadConfig(cfg *Config, args ...string) (*Config, error) {
	loader := ezconf.NewLoader(cfg, "courier", "Courier - A fast message broker for SMS and IP messages", []string{"config.toml"})
	if len(args) > 0 { // allow tests to pass in args
		loader.SetArgs(args...)
	}
	if err := loader.Load(); err != nil {
		// Load never writes to stdout or stderr itself, so a request for usage comes back as ErrHelp for us to
		// act on here, where we still have the loader to show it with. The sentinel is passed up so that the
		// caller can tell an explicit -help from a genuine config failure.
		if errors.Is(err, ezconf.ErrHelp) {
			loader.Usage()
		}
		return nil, err
	}

	if err := cfg.Parse(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// Parse validates the config and fills in the values which can't be used in the form they're configured in. It's
// called by LoadConfig, and a config built by other means (e.g. NewDefaultConfig in a test) must be parsed before
// being handed to NewRuntime - the values it fills in have no meaningful zero value, so skipping it would silently
// leave the SSRF blocklist empty rather than fail.
func (c *Config) Parse() error {
	if err := utils.Validate(c); err != nil {
		return err
	}

	ips, nets, err := httpx.ParseNetworks(c.DisallowedNetworks...)
	if err != nil {
		return fmt.Errorf("unable to parse 'DisallowedNetworks': %w", err)
	}
	c.DisallowedIPs, c.DisallowedNets = ips, nets

	// the validator has already enforced that this is an http(s) URL if set. Cleared rather than left alone when
	// unset, so that parsing twice can't leave a stale URL behind - same as the networks above.
	c.SendProxyURLParsed = nil
	if c.SendProxyURL != "" {
		u, err := url.Parse(c.SendProxyURL)
		if err != nil {
			return fmt.Errorf("unable to parse 'SendProxyURL': %w", err)
		}
		c.SendProxyURLParsed = u
	}

	return nil
}
