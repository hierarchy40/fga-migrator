// Package fgamigrator versions an OpenFGA store with numbered migrations and a schema_version marker.
package fgamigrator

import (
	"context"

	"github.com/openfga/go-sdk/client"
)

// Step must be idempotent: a crash before its version is stamped re-runs it.
type Step func(ctx context.Context, fga *client.OpenFgaClient) error

// Migration is one schema version: its full model snapshot and optional data steps.
type Migration struct {
	Version int
	Name    string
	Model   string // full DSL as of this version, without the schema_version type
	Before  Step   // against the model stored before this version
	After   Step   // against this version's Model
}

// MigrationStatus says whether the store has a migration applied.
type MigrationStatus struct {
	Version int
	Name    string
	Applied bool
}
