package fgamigrator

import (
	_ "embed"
	"testing"
)

var (
	//go:embed testdata/legacy.fga
	legacyModel string
	//go:embed testdata/v1.fga
	v1Model string
	//go:embed testdata/v2.fga
	v2Model string
	//go:embed testdata/v3.fga
	v3Model string
)

func baseline() Migration {
	return Migration{Version: 1, Name: "baseline", Model: v1Model}
}

func mustSet(t *testing.T, ms ...Migration) *Set {
	t.Helper()
	s, err := NewSet(len(ms), ms...)
	if err != nil {
		t.Fatalf("new set: %v", err)
	}
	return s
}
