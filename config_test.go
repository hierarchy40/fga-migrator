package fgamigrator

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/openfga/go-sdk/credentials"
)

func TestLoadConfigReadsFGAVariables(t *testing.T) {
	t.Setenv("FGA_API_URL", "http://fga:8080")
	t.Setenv("FGA_STORE_NAME", "store")
	t.Setenv("FGA_CLIENT_CREDENTIALS_CLIENT_ID", "client")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "http://fga:8080" || cfg.StoreName != "store" || cfg.ClientCredentialsClientID != "client" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.AuthMethod != credentials.CredentialsMethodNone {
		t.Fatalf("AuthMethod = %q, want none", cfg.AuthMethod)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("NAMESPACE", "app-dev")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "http://openfga:8080" {
		t.Fatalf("APIURL = %q", cfg.APIURL)
	}
	if cfg.StoreName != "app-dev" {
		t.Fatalf("StoreName = %q, want the NAMESPACE fallback", cfg.StoreName)
	}
}

func TestAPIURLLosesTrailingSlashes(t *testing.T) {
	t.Setenv("FGA_API_URL", "http://fga:8080//")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIURL != "http://fga:8080" {
		t.Fatalf("APIURL = %q", cfg.APIURL)
	}
	if got := (Config{APIURL: "http://fga:8080/"}).WithDefaults().APIURL; got != "http://fga:8080" {
		t.Fatalf("WithDefaults APIURL = %q", got)
	}
}

func TestStoreIDSkipsNamespaceFallback(t *testing.T) {
	t.Setenv("NAMESPACE", "app-dev")
	if got := (Config{StoreID: "id"}).WithDefaults().StoreName; got != "" {
		t.Fatalf("StoreName = %q, want empty", got)
	}
}

func TestConfigEmbedsInServiceConfig(t *testing.T) {
	t.Setenv("FGA_STORE_ID", "id")
	t.Setenv("FGA_MODEL_ID", "model")
	var service struct {
		Config
		ModelID string `envconfig:"FGA_MODEL_ID"`
	}
	var named struct{ FGA Config }
	if err := envconfig.Process("", &service); err != nil {
		t.Fatal(err)
	}
	if err := envconfig.Process("", &named); err != nil {
		t.Fatal(err)
	}
	if service.StoreID != "id" || service.ModelID != "model" || named.FGA.StoreID != "id" {
		t.Fatalf("service = %+v, named = %q", service, named.FGA.StoreID)
	}
}
