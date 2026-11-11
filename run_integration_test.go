//go:build integration

package fgamigrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hierarchy40/fga-migrator/fgatest"
	"github.com/openfga/go-sdk/client"
)

func verifyByStoreID(ctx context.Context, t *testing.T, storeID string, required int) {
	t.Helper()
	fga := fgatest.Client(t)
	if err := fga.SetStoreId(storeID); err != nil {
		t.Fatal(err)
	}
	if err := VerifySchemaVersion(ctx, fga, required); err != nil {
		t.Fatalf("consumer verify: %v", err)
	}
}

func TestRunCreatesStoreAndIsRepeatable(t *testing.T) {
	ctx := testContext(t)
	s := mustMigrations(t, baseline(), addWriter())
	cfg := Config{APIURL: fgatest.APIURL(), StoreName: fgatest.StoreName(t)}

	if err := Run(ctx, cfg, s, time.Second); err != nil {
		t.Fatalf("first run: %v", err)
	}
	storeID, err := FindStore(ctx, fgatest.Client(t), cfg.StoreName)
	if err != nil || storeID == "" {
		t.Fatalf("store not created: %q, %v", storeID, err)
	}
	if err := Run(ctx, cfg, s, time.Second); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if again, _ := FindStore(ctx, fgatest.Client(t), cfg.StoreName); again != storeID {
		t.Fatalf("second run resolved store %q, want %q", again, storeID)
	}
	verifyByStoreID(ctx, t, storeID, 2)
	if err := Run(ctx, Config{APIURL: fgatest.APIURL(), StoreID: storeID}, s, time.Second); err != nil {
		t.Fatalf("run by store id: %v", err)
	}
}

func TestRunAcceptsTrailingSlashInAPIURL(t *testing.T) {
	ctx := testContext(t)
	s := mustMigrations(t, baseline())
	cfg := Config{APIURL: fgatest.APIURL() + "/", StoreName: fgatest.StoreName(t)}

	if err := Run(ctx, cfg, s, time.Second); err != nil {
		t.Fatalf("run: %v", err)
	}
	storeID, err := FindStore(ctx, fgatest.Client(t), cfg.StoreName)
	if err != nil || storeID == "" {
		t.Fatalf("store not created: %q, %v", storeID, err)
	}
	verifyByStoreID(ctx, t, storeID, 1)
}

func TestRunFailsOnUnknownStoreID(t *testing.T) {
	ctx := testContext(t)
	err := Run(ctx, Config{APIURL: fgatest.APIURL(), StoreID: "01HXXXXXXXXXXXXXXXXXXXXXXX"}, mustMigrations(t, baseline()), time.Second)
	if err == nil {
		t.Fatal("run accepted an unknown store id")
	}
}

func TestRunWaitTimesOutWhenUnreachable(t *testing.T) {
	ctx := testContext(t)
	start := time.Now()
	err := Run(ctx, Config{APIURL: "http://127.0.0.1:1", StoreName: "x"}, mustMigrations(t, baseline()), 3*time.Second)
	if err == nil || !strings.Contains(err.Error(), "not reachable") {
		t.Fatalf("err = %v, want not reachable", err)
	}
	if elapsed := time.Since(start); elapsed < 3*time.Second || elapsed > 20*time.Second {
		t.Fatalf("wait took %s, want about 3s", elapsed)
	}
}

func TestFindStore(t *testing.T) {
	ctx := testContext(t)
	_, storeID := fgatest.NewStore(ctx, t)
	fga := fgatest.Client(t)
	res, err := fga.GetStore(ctx).Options(client.ClientGetStoreOptions{StoreId: &storeID}).Execute()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := FindStore(ctx, fga, res.Name); err != nil || got != storeID {
		t.Fatalf("FindStore(%q) = %q, %v; want %q", res.Name, got, err, storeID)
	}
	if got, err := FindStore(ctx, fga, "no-such-store-"+storeID); err != nil || got != "" {
		t.Fatalf("FindStore(unknown) = %q, %v; want empty", got, err)
	}
}

func TestRunRejectsInvalidMigrationsBeforeConnecting(t *testing.T) {
	err := Run(testContext(t), Config{APIURL: "http://127.0.0.1:1", StoreName: "x"}, NewMigrations(), time.Minute)
	if err == nil || !strings.Contains(err.Error(), "invalid migrations") {
		t.Fatalf("err = %v, want invalid migrations", err)
	}
}
