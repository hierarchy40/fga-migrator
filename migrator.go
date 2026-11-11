package fgamigrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

// Migrator runs a caller's migrations against one store.
type Migrator struct {
	fga        *client.OpenFgaClient
	migrations *Migrations
}

// NewMigrator needs a client bound to a store but not to a model.
func NewMigrator(fga *client.OpenFgaClient, migrations *Migrations) *Migrator {
	return &Migrator{fga: fga, migrations: migrations}
}

// Migrate applies the pending migrations one at a time, in ascending version order.
func (m *Migrator) Migrate(ctx context.Context) error {
	entries, err := m.migrations.sorted()
	if err != nil {
		return err
	}
	if modelID, err := m.fga.GetAuthorizationModelId(); err != nil || modelID != "" {
		return errors.Join(errors.New("migrate needs a client not pinned to an authorization model"), err)
	}
	stored, err := StoredSchemaVersion(ctx, m.fga)
	if err != nil {
		return fmt.Errorf("read openfga schema version: %w", err)
	}
	required := len(entries)
	if stored > required {
		return fmt.Errorf("openfga schema version %d is ahead of required version %d; refusing to downgrade", stored, required)
	}
	slog.Info("openfga migrations", "stored", stored, "required", required, "pending", required-stored)

	for _, e := range entries[stored:] {
		if err := m.apply(ctx, e); err != nil {
			return fmt.Errorf("migration %d %s: %w", e.Version, e.Name, err)
		}
	}
	// repairs a drifted model at the required version
	return writeModelIfChanged(ctx, m.fga, entries[required-1].model)
}

// Status lists every migration with whether the store has it applied.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	entries, err := m.migrations.sorted()
	if err != nil {
		return nil, err
	}
	stored, err := StoredSchemaVersion(ctx, m.fga)
	if err != nil {
		return nil, fmt.Errorf("read openfga schema version: %w", err)
	}
	out := make([]MigrationStatus, 0, len(entries))
	for _, e := range entries {
		out = append(out, MigrationStatus{Version: e.Version, Name: e.Name, Applied: e.Version <= stored})
	}
	return out, nil
}

// Verify fails unless the store is at exactly the highest version.
func (m *Migrator) Verify(ctx context.Context) error {
	required, err := m.migrations.Required()
	if err != nil {
		return err
	}
	return VerifySchemaVersion(ctx, m.fga, required)
}

func (m *Migrator) apply(ctx context.Context, e *entry) error {
	slog.Info("applying migration", "version", e.Version, "name", e.Name)
	if e.Before != nil {
		if err := e.Before(ctx, m.fga); err != nil {
			return fmt.Errorf("before-model step: %w", err)
		}
	}
	if err := writeModelIfChanged(ctx, m.fga, e.model); err != nil {
		return err
	}
	if e.After != nil {
		if err := e.After(ctx, m.fga); err != nil {
			return fmt.Errorf("after-model step: %w", err)
		}
	}
	return stampMarker(ctx, m.fga, e.Version)
}

func writeModelIfChanged(ctx context.Context, fga *client.OpenFgaClient, model *openfga.AuthorizationModel) error {
	latest, err := LatestModel(ctx, fga)
	if err != nil {
		return fmt.Errorf("read latest authorization model: %w", err)
	}
	if latest != nil {
		same, err := sameModel(latest, model)
		if err != nil {
			return fmt.Errorf("compare authorization models: %w", err)
		}
		if same {
			return nil
		}
	}
	res, err := fga.WriteAuthorizationModel(ctx).Body(client.ClientWriteAuthorizationModelRequest{
		TypeDefinitions: model.TypeDefinitions,
		SchemaVersion:   model.SchemaVersion,
		Conditions:      model.Conditions,
	}).Execute()
	if err != nil {
		return fmt.Errorf("write authorization model: %w", err)
	}
	slog.Info("wrote authorization model", "id", res.AuthorizationModelId)
	return nil
}

func stampMarker(ctx context.Context, fga *client.OpenFgaClient, version int) error {
	_, err := fga.Write(ctx).
		Body(client.ClientWriteRequest{Writes: []client.ClientTupleKey{
			{User: markerUser(version), Relation: markerRelation, Object: markerObject},
		}}).
		Options(client.ClientWriteOptions{Conflict: client.ClientWriteConflictOptions{
			OnDuplicateWrites: client.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
		}}).
		Execute()
	if err != nil {
		return fmt.Errorf("stamp schema version marker %d: %w", version, err)
	}
	return nil
}
