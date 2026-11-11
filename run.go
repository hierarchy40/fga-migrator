package fgamigrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

const pollInterval = 2 * time.Second

// Run waits up to waitTimeout for OpenFGA, resolves or creates the store, migrates and verifies it.
func Run(ctx context.Context, cfg Config, migrations *Migrations, waitTimeout time.Duration) error {
	required, err := migrations.Required()
	if err != nil {
		return fmt.Errorf("invalid migrations: %w", err)
	}
	cfg = cfg.WithDefaults()
	fga, err := cfg.NewClient()
	if err != nil {
		return fmt.Errorf("build openfga client: %w", err)
	}
	if err := waitForOpenFGA(ctx, fga, waitTimeout); err != nil {
		return err
	}
	storeID, err := ensureStore(ctx, fga, cfg)
	if err != nil {
		return err
	}
	if err := fga.SetStoreId(storeID); err != nil {
		return err
	}
	migrator := NewMigrator(fga, migrations)
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migrations failed: %w", err)
	}
	if err := migrator.Verify(ctx); err != nil {
		return fmt.Errorf("schema version check failed: %w", err)
	}
	slog.Info("openfga store at required schema version", "store", storeID, "version", required)
	return nil
}

func waitForOpenFGA(ctx context.Context, fga *client.OpenFgaClient, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		_, err := fga.ListStores(ctx).Options(client.ClientListStoresOptions{PageSize: openfga.PtrInt32(1)}).Execute()
		if err == nil {
			return nil
		}
		slog.Info("waiting for openfga", "error", err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("openfga not reachable within %s: %w", timeout, errors.Join(ctx.Err(), err))
		case <-time.After(pollInterval):
		}
	}
}

func ensureStore(ctx context.Context, fga *client.OpenFgaClient, cfg Config) (string, error) {
	if cfg.StoreID != "" {
		if _, err := fga.GetStore(ctx).Options(client.ClientGetStoreOptions{StoreId: &cfg.StoreID}).Execute(); err != nil {
			return "", fmt.Errorf("read openfga store %s: %w", cfg.StoreID, err)
		}
		return cfg.StoreID, nil
	}
	if cfg.StoreName == "" {
		return "", errors.New("FGA_STORE_ID or FGA_STORE_NAME (or NAMESPACE) is required")
	}
	storeID, err := FindStore(ctx, fga, cfg.StoreName)
	if err != nil {
		return "", fmt.Errorf("find openfga store %q: %w", cfg.StoreName, err)
	}
	if storeID != "" {
		return storeID, nil
	}
	slog.Info("openfga store not found, creating", "name", cfg.StoreName)
	res, err := fga.CreateStore(ctx).Body(client.ClientCreateStoreRequest{Name: cfg.StoreName}).Execute()
	if err != nil {
		return "", fmt.Errorf("create openfga store %q: %w", cfg.StoreName, err)
	}
	return res.Id, nil
}
