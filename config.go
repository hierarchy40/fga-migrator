package fgamigrator

import (
	"os"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/openfga/go-sdk/client"
	"github.com/openfga/go-sdk/credentials"
)

// Config holds the FGA_* connection settings; embed it or fill it by hand.
type Config struct {
	APIURL                          string                        `envconfig:"FGA_API_URL" default:"http://openfga:8080"`
	StoreID                         string                        `envconfig:"FGA_STORE_ID"`
	StoreName                       string                        `envconfig:"FGA_STORE_NAME"`
	AuthMethod                      credentials.CredentialsMethod `envconfig:"FGA_AUTH_METHOD"`
	APIToken                        string                        `envconfig:"FGA_API_TOKEN"`
	ClientCredentialsAPITokenIssuer string                        `envconfig:"FGA_CLIENT_CREDENTIALS_API_TOKEN_ISSUER"`
	ClientCredentialsAPIAudience    string                        `envconfig:"FGA_CLIENT_CREDENTIALS_API_AUDIENCE"`
	ClientCredentialsClientID       string                        `envconfig:"FGA_CLIENT_CREDENTIALS_CLIENT_ID"`
	ClientCredentialsClientSecret   string                        `envconfig:"FGA_CLIENT_CREDENTIALS_CLIENT_SECRET"`
	ClientCredentialsScopes         string                        `envconfig:"FGA_CLIENT_CREDENTIALS_SCOPES"`
}

// LoadConfig reads the FGA_* variables and applies WithDefaults.
func LoadConfig() (Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, err
	}
	return cfg.WithDefaults(), nil
}

// WithDefaults trims the API URL and applies the NAMESPACE store name and "none" auth fallbacks.
func (c Config) WithDefaults() Config {
	// the SDK appends paths verbatim and OpenFGA answers //stores with 404
	c.APIURL = strings.TrimRight(c.APIURL, "/")
	if c.StoreID == "" && c.StoreName == "" {
		c.StoreName = os.Getenv("NAMESPACE")
	}
	if c.AuthMethod == "" {
		c.AuthMethod = credentials.CredentialsMethodNone
	}
	return c
}

// NewClient returns a client bound to no store and no model.
func (c Config) NewClient() (*client.OpenFgaClient, error) {
	c = c.WithDefaults()
	return client.NewSdkClient(&client.ClientConfiguration{
		ApiUrl: c.APIURL,
		Credentials: &credentials.Credentials{
			Method: c.AuthMethod,
			Config: &credentials.Config{
				ApiToken:                        c.APIToken,
				ClientCredentialsApiTokenIssuer: c.ClientCredentialsAPITokenIssuer,
				ClientCredentialsApiAudience:    c.ClientCredentialsAPIAudience,
				ClientCredentialsClientId:       c.ClientCredentialsClientID,
				ClientCredentialsClientSecret:   c.ClientCredentialsClientSecret,
				ClientCredentialsScopes:         c.ClientCredentialsScopes,
			},
		},
	})
}
