package command

import (
	"bytes"
	_ "embed"
	"strings"
	"testing"

	fgamigrator "github.com/hierarchy40/fga-migrator"
)

//go:embed testdata/model.fga
var model string

func testMigrations() *fgamigrator.Migrations {
	migrations := fgamigrator.NewMigrations()
	migrations.MustAdd(fgamigrator.Migration{Version: 1, Name: "baseline", Model: model})
	migrations.MustAdd(fgamigrator.Migration{Version: 2, Name: "noop", Model: model})
	return migrations
}

func TestSchemaVersionPrintsRequiredVersion(t *testing.T) {
	var out bytes.Buffer
	if err := runSchemaVersion(nil, &out, testMigrations()); err != nil {
		t.Fatal(err)
	}
	if out.String() != "2\n" {
		t.Fatalf("output = %q, want 2", out.String())
	}
}

func TestSchemaVersionPrintsModelDigest(t *testing.T) {
	var out bytes.Buffer
	if err := runSchemaVersion([]string{"-model-digest"}, &out, testMigrations()); err != nil {
		t.Fatal(err)
	}
	want, err := testMigrations().ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != want || !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSchemaVersionRejectsUnknownFlags(t *testing.T) {
	var out bytes.Buffer
	if err := runSchemaVersion([]string{"-nope"}, &out, testMigrations()); err == nil {
		t.Fatal("unknown flag accepted")
	}
}

func TestSchemaVersionRejectsInvalidMigrations(t *testing.T) {
	var out bytes.Buffer
	for _, args := range [][]string{nil, {"-model-digest"}} {
		if err := runSchemaVersion(args, &out, fgamigrator.NewMigrations()); err == nil {
			t.Fatalf("schemaversion %v accepted an empty collection", args)
		}
	}
}
