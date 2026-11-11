package fgamigrator

import (
	_ "embed"
	"testing"

	openfga "github.com/openfga/go-sdk"
)

var (
	//go:embed testdata/v1.fga
	v1Model string
	//go:embed testdata/v2.fga
	v2Model string
	//go:embed testdata/v3.fga
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

func mustSet(t *testing.T, ms ...Migration) *Set {
	t.Helper()
	s, err := NewSet(len(ms), ms...)
	if err != nil {
		t.Fatalf("new set: %v", err)
	}
	return s
}
