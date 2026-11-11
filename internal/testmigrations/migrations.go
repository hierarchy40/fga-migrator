// Package testmigrations registers steps from numbered files, the way a caller does.
package testmigrations

import (
	"context"

	fgamigrator "github.com/hierarchy40/fga-migrator"
	"github.com/openfga/go-sdk/client"
)

// Migrations has steps only; its test discovers the models.
var Migrations = fgamigrator.NewMigrations()

func noop(context.Context, *client.OpenFgaClient) error { return nil }
