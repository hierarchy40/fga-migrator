package testmigrations

import (
	"os"
	"testing"
)

func TestNumberedFilesRegisterTheirSteps(t *testing.T) {
	if err := Migrations.Validate(); err == nil {
		t.Fatal("steps without models passed validation")
	}
	if err := Migrations.Discover(os.DirFS("../../testdata/migrations")); err != nil {
		t.Fatal(err)
	}
	ms, err := Migrations.Sorted()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 {
		t.Fatalf("%d migrations, want 3", len(ms))
	}
	steps := [][2]bool{{false, false}, {false, true}, {true, false}}
	names := []string{"baseline", "add_writer", "drop_editor"}
	for i, m := range ms {
		if m.Name != names[i] || (m.Before != nil) != steps[i][0] || (m.After != nil) != steps[i][1] {
			t.Errorf("migration %d %s: before=%v after=%v", m.Version, m.Name, m.Before != nil, m.After != nil)
		}
	}
}
