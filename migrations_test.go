package fgamigrator

import (
	"context"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/openfga/go-sdk/client"
)

func noop(context.Context, *client.OpenFgaClient) error { return nil }

func wantError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v, want it to mention %q", err, want)
	}
}

func TestSortedOrdersByVersion(t *testing.T) {
	migrations := mustMigrations(t, dropEditor(), baseline(), addWriter())
	ms, err := migrations.Sorted()
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range ms {
		if m.Version != i+1 {
			t.Fatalf("migration %d has version %d", i, m.Version)
		}
	}
	if required, err := migrations.Required(); err != nil || required != 3 {
		t.Fatalf("required = %d, %v; want 3", required, err)
	}
	ms[0].Name = "mutated"
	if again, _ := migrations.Sorted(); again[0].Name != "baseline" {
		t.Fatal("Sorted exposes the collection's own migrations")
	}
}

func TestAddRejectsInvalidMigrations(t *testing.T) {
	cases := map[string]struct {
		m    Migration
		want string
	}{
		"version zero":    {Migration{Name: "baseline", Model: v1Model}, "versions start at 1"},
		"no name":         {Migration{Version: 2, Model: v1Model}, "has name"},
		"bad name":        {Migration{Version: 2, Name: "Add Writer", Model: v1Model}, "has name"},
		"no model":        {Migration{Version: 2, Name: "data_only"}, "no model"},
		"bad dsl":         {Migration{Version: 2, Name: "broken", Model: "model\n  schema 1.1\ntype"}, "model"},
		"declares marker": {Migration{Version: 2, Name: "marked", Model: v1Model + handWrittenMarker}, "reserved schema_version type"},
		"duplicate":       {Migration{Version: 1, Name: "baseline", Model: v1Model}, "already has a model"},
		"other name":      {Migration{Version: 1, Name: "again", Model: v1Model}, "belongs to both baseline and again"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			migrations := mustMigrations(t, baseline())
			wantError(t, migrations.Add(tc.m), tc.want)
			if err := migrations.Validate(); err != nil {
				t.Fatalf("a rejected migration left the collection invalid: %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidCollections(t *testing.T) {
	cases := map[string]struct {
		migrations *Migrations
		ms         []Migration
		want       string
	}{
		"empty":        {NewMigrations(), nil, "no migrations"},
		"gap":          {NewMigrations(), []Migration{baseline(), dropEditor()}, "contiguous"},
		"not from one": {NewMigrations(), []Migration{addWriter()}, "contiguous"},
		"behind":       {NewMigrations(WithRequired(2)), []Migration{baseline()}, "want required schema version 2"},
		"ahead":        {NewMigrations(WithRequired(1)), []Migration{baseline(), addWriter()}, "want required schema version 1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			for _, m := range tc.ms {
				if err := tc.migrations.Add(m); err != nil {
					t.Fatal(err)
				}
			}
			wantError(t, tc.migrations.Validate(), tc.want)
			if _, err := tc.migrations.Required(); err == nil {
				t.Fatal("Required answered for an invalid collection")
			}
			if _, err := tc.migrations.ModelDigest(); err == nil {
				t.Fatal("ModelDigest answered for an invalid collection")
			}
		})
	}
}

func TestWithRequiredAcceptsTheHighestVersion(t *testing.T) {
	migrations := NewMigrations(WithRequired(2))
	migrations.MustAdd(baseline())
	migrations.MustAdd(addWriter())
	if err := migrations.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMustAddPanicsOnInvalidMigration(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustAdd accepted a migration without a model")
		}
	}()
	NewMigrations().MustAdd(Migration{Version: 1, Name: "baseline"})
}

func TestDiscoverReadsNumberedModelFiles(t *testing.T) {
	migrations := NewMigrations()
	if err := migrations.Discover(os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	ms, err := migrations.Sorted()
	if err != nil {
		t.Fatal(err)
	}
	want := []Migration{baseline(), addWriter(), dropEditor()}
	if len(ms) != len(want) {
		t.Fatalf("discovered %d migrations, want %d", len(ms), len(want))
	}
	for i, m := range ms {
		if m.Version != want[i].Version || m.Name != want[i].Name || m.Model != want[i].Model {
			t.Fatalf("migration %d = %d %s, want %d %s with its model", i, m.Version, m.Name, want[i].Version, want[i].Name)
		}
	}
}

func TestDiscoverRejectsBadFiles(t *testing.T) {
	model := &fstest.MapFile{Data: []byte(v1Model)}
	cases := map[string]struct {
		fsys fstest.MapFS
		want string
	}{
		"unnumbered":      {fstest.MapFS{"core.fga": model}, "unsupported migration file name"},
		"upper case":      {fstest.MapFS{"0001_Baseline.fga": model}, "unsupported migration file name"},
		"version zero":    {fstest.MapFS{"0000_baseline.fga": model}, "versions start at 1"},
		"duplicate":       {fstest.MapFS{"0001_baseline.fga": model, "old/1_baseline.fga": model}, "already has a model"},
		"two names":       {fstest.MapFS{"0001_baseline.fga": model, "0001_initial.fga": model}, "belongs to both"},
		"empty":           {fstest.MapFS{"0001_baseline.fga": &fstest.MapFile{}}, "no model"},
		"declares marker": {fstest.MapFS{"0001_baseline.fga": &fstest.MapFile{Data: []byte(v1Model + handWrittenMarker)}}, "reserved schema_version type"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wantError(t, NewMigrations().Discover(tc.fsys), tc.want)
		})
	}
}

func TestDiscoverIgnoresOtherFiles(t *testing.T) {
	migrations := NewMigrations()
	err := migrations.Discover(fstest.MapFS{
		"0001_baseline.fga":  &fstest.MapFile{Data: []byte(v1Model)},
		"0002_add_writer.go": &fstest.MapFile{Data: []byte("package migrations")},
		"README.md":          &fstest.MapFile{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if required, err := migrations.Required(); err != nil || required != 1 {
		t.Fatalf("required = %d, %v; want 1", required, err)
	}
}

func TestRegisterFileMergesStepsWithTheDiscoveredModel(t *testing.T) {
	migrations := NewMigrations()
	if err := migrations.registerFile("/src/migrations/0002_add_writer.go", nil, noop); err != nil {
		t.Fatal(err)
	}
	wantError(t, migrations.Validate(), "contiguous")
	if err := migrations.Discover(os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	ms, err := migrations.Sorted()
	if err != nil {
		t.Fatal(err)
	}
	if ms[1].Before != nil || ms[1].After == nil || ms[1].Model != v2Model {
		t.Fatalf("migration 2 = %+v, want the model and the after step", ms[1])
	}
	if ms[0].After != nil || ms[2].After != nil {
		t.Fatal("steps leaked into another version")
	}
}

func TestRegisterFileRejectsBadFiles(t *testing.T) {
	cases := map[string]struct {
		file string
		want string
	}{
		"unnumbered":   {"/src/migrations/migrations.go", "unsupported migration file name"},
		"model file":   {"/src/migrations/0002_add_writer.fga", "want NNNN_name.go"},
		"second steps": {"/src/other/0002_add_writer.go", "already has steps"},
		"other name":   {"/src/migrations/0002_add_reader.go", "belongs to both add_writer and add_reader"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			migrations := NewMigrations()
			if err := migrations.registerFile("/src/migrations/0002_add_writer.go", noop, nil); err != nil {
				t.Fatal(err)
			}
			wantError(t, migrations.registerFile(tc.file, noop, nil), tc.want)
		})
	}
}

func TestStepsWithoutAModelFailValidation(t *testing.T) {
	migrations := mustMigrations(t, baseline())
	if err := migrations.registerFile("0002_backfill.go", nil, noop); err != nil {
		t.Fatal(err)
	}
	wantError(t, migrations.Validate(), "migration 2 backfill has no model: add 0002_backfill.fga")
}

func TestRegisterRejectsACallerThatIsNotANumberedFile(t *testing.T) {
	wantError(t, NewMigrations().Register(nil, noop), `unsupported migration file name "migrations_test.go"`)
	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister accepted a caller that is not a numbered file")
		}
	}()
	NewMigrations().MustRegister(nil, noop)
}

func TestModelIsTheHighestVersionWithTheMarkerType(t *testing.T) {
	model, err := mustMigrations(t, baseline(), addWriter()).Model()
	if err != nil {
		t.Fatal(err)
	}
	want, err := normaliseDSL(v2Model + handWrittenMarker)
	if err != nil {
		t.Fatal(err)
	}
	if model != want {
		t.Fatalf("model =\n%s\nwant\n%s", model, want)
	}
}
