//go:build integration

package command

import (
	"context"
	"strings"
	"testing"
	"time"

	fgamigrator "github.com/hierarchy40/fga-migrator"
	"github.com/hierarchy40/fga-migrator/fgatest"
)

func TestMain(m *testing.M) { fgatest.Main(m) }

func TestMigrateReadsEnvironment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	name := fgatest.StoreName(t)
	t.Setenv("FGA_API_URL", fgatest.APIURL()+"/")
	t.Setenv("FGA_STORE_NAME", name)
	t.Setenv("FGA_WAIT_TIMEOUT", "5s")

	if err := runMigrate(ctx, testSet()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	fga := fgatest.Client(t)
	storeID, err := fgamigrator.FindStore(ctx, fga, name)
	if err != nil || storeID == "" {
		t.Fatalf("store not created: %q, %v", storeID, err)
	}
	if err := fga.SetStoreId(storeID); err != nil {
		t.Fatal(err)
	}
	if err := fgamigrator.VerifySchemaVersion(ctx, fga, 2); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateRejectsBadWaitTimeout(t *testing.T) {
	t.Setenv("FGA_WAIT_TIMEOUT", "soon")
	err := runMigrate(context.Background(), testSet())
	if err == nil || !strings.Contains(err.Error(), "config") {
		t.Fatalf("err = %v, want a config error", err)
	}
}
