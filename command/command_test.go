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

func testSet() *fgamigrator.Set {
	return fgamigrator.MustSet(2,
		fgamigrator.Migration{Version: 1, Name: "baseline", Model: model},
		fgamigrator.Migration{Version: 2, Name: "noop", Model: model},
	)
}

func TestSchemaVersionPrintsRequiredVersion(t *testing.T) {
	var out bytes.Buffer
	if err := runSchemaVersion(nil, &out, testSet()); err != nil {
		t.Fatal(err)
	}
	if out.String() != "2\n" {
		t.Fatalf("output = %q, want 2", out.String())
	}
}

func TestSchemaVersionPrintsModelDigest(t *testing.T) {
	var out bytes.Buffer
	if err := runSchemaVersion([]string{"-model-digest"}, &out, testSet()); err != nil {
		t.Fatal(err)
	}
	want, err := testSet().ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != want || !strings.HasPrefix(got, "sha256:") {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestSchemaVersionRejectsUnknownFlags(t *testing.T) {
	var out bytes.Buffer
	if err := runSchemaVersion([]string{"-nope"}, &out, testSet()); err == nil {
		t.Fatal("unknown flag accepted")
	}
}
