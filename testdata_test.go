package fgamigrator

import (
	_ "embed"
	"testing"

	openfga "github.com/openfga/go-sdk"
)

var (
	//go:embed testdata/migrations/0001_baseline.fga
	v1Model string
	//go:embed testdata/migrations/0002_add_writer.fga
	v2Model string
	//go:embed testdata/migrations/0003_drop_editor.fga
	v3Model string
)

const handWrittenMarker = `
type schema_version
  relations
    define applied: [schema_version]
`

func mustEffective(t *testing.T, dsl string) *openfga.AuthorizationModel {
	t.Helper()
	model, err := effectiveModel(dsl)
	if err != nil {
		t.Fatalf("effective model: %v", err)
	}
	return model
}

func baseline() Migration {
	return Migration{Version: 1, Name: "baseline", Model: v1Model}
}

func addWriter() Migration {
	return Migration{Version: 2, Name: "add_writer", Model: v2Model}
}

func dropEditor() Migration {
	return Migration{Version: 3, Name: "drop_editor", Model: v3Model}
}

func mustMigrations(t *testing.T, ms ...Migration) *Migrations {
	t.Helper()
	migrations := NewMigrations()
	for _, m := range ms {
		if err := migrations.Add(m); err != nil {
			t.Fatalf("add migration: %v", err)
		}
	}
	if err := migrations.Validate(); err != nil {
		t.Fatalf("validate migrations: %v", err)
	}
	return migrations
}
