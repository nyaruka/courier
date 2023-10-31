package courier

import (
	"encoding/csv"
	"io"
	"net"
	"strings"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/ezconf"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/pkg/errors"
)

// Config is our top level configuration object
type Config struct {
	Backend   string `help:"the backend that will be used by courier (currently only rapidpro is supported)"`
	SentryDSN string `help:"the DSN used for logging errors to Sentry"`
	Domain    string `help:"the domain courier is exposed on"`
	Address   string `help:"the network interface address courier will bind to"`
	Port      int    `help:"the port courier will listen on"`
	DB        string `help:"URL describing how to connect to the RapidPro database"`
	Redis     string `help:"URL describing how to connect to Redis"`
	SpoolDir  string `help:"the local directory where courier will write statuses or msgs that need to be retried (needs to be writable)"`

	AWSAccessKeyID      string `help:"the access key id to use when authenticating S3"`
	AWSSecretAccessKey  string `help:"the secret access key id to use when authenticating S3"`
	AWSUseCredChain     bool   `help:"whether to use the AWS credentials chain. Defaults to false."`
	S3Endpoint          string `help:"the S3 endpoint we will write attachments to"`
	S3Region            string `help:"the S3 region we will write attachments to"`
	S3AttachmentsBucket string `help:"the S3 bucket we will write attachments to"`
	S3AttachmentsPrefix string `help:"the prefix that will be added to attachment filenames"`
	S3LogsBucket        string `help:"the S3 bucket we will write channel logs to"`
	S3DisableSSL        bool   `help:"whether we disable SSL when accessing S3. Should always be set to False unless you're hosting an S3 compatible service within a secure internal network"`
	S3ForcePathStyle    bool   `help:"whether we force S3 path style. Should generally need to default to False unless you're hosting an S3 compatible service"`

	FacebookApplicationSecret    string `help:"the Facebook app secret"`
	FacebookWebhookSecret        string `help:"the secret for Facebook webhook URL verification"`
	WhatsappAdminSystemUserToken string `help:"the token of the admin system user for WhatsApp"`

	DisallowedNetworks string `help:"comma separated list of IP addresses and networks which we disallow fetching attachments from"`
	MediaDomain        string `help:"the domain on which we'll try to resolve outgoing media URLs"`
	MaxWorkers         int    `help:"the maximum number of go routines that will be used for sending (set to 0 to disable sending)"`
	LibratoUsername    string `help:"the username that will be used to authenticate to Librato"`
	LibratoToken       string `help:"the token that will be used to authenticate to Librato"`
	StatusUsername     string `help:"the username that is needed to authenticate against the /status endpoint"`
	StatusPassword     string `help:"the password that is needed to authenticate against the /status endpoint"`
	AuthToken          string `help:"the authentication token need to access non-channel endpoints"`
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
		Backend:  "rapidpro",
		Domain:   "localhost",
		Address:  "",
		Port:     8080,
		DB:       "postgres://temba:temba@localhost/temba?sslmode=disable",
		Redis:    "redis://localhost:6379/15",
		SpoolDir: "/var/spool/courier",

		AWSAccessKeyID:      "",
		AWSSecretAccessKey:  "",
		AWSUseCredChain:     false,
		S3Endpoint:          "https://s3.amazonaws.com",
		S3Region:            "us-east-1",
		S3AttachmentsBucket: "courier-media",
		S3AttachmentsPrefix: "media/",
		S3LogsBucket:        "courier-logs",
		S3DisableSSL:        false,
		S3ForcePathStyle:    false,

		FacebookApplicationSecret:    "missing_facebook_app_secret",
		FacebookWebhookSecret:        "missing_facebook_webhook_secret",
		WhatsappAdminSystemUserToken: "missing_whatsapp_admin_system_user_token",

		DisallowedNetworks: `127.0.0.1,::1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,169.254.0.0/16,fe80::/10`,
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

// Validate validates the config
func (c *Config) Validate() error {
	if err := utils.Validate(c); err != nil {
		return err
	}

	if _, _, err := c.ParseDisallowedNetworks(); err != nil {
		return errors.Wrap(err, "unable to parse 'DisallowedNetworks'")
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
