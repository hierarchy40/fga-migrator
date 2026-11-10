package fgamigrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openfga/go-sdk/client"
)

// Migrate applies the pending migrations one at a time; fga must be bound to a store but not to a model.
func (s *Set) Migrate(ctx context.Context, fga *client.OpenFgaClient) error {
	if modelID, err := fga.GetAuthorizationModelId(); err != nil || modelID != "" {
		return errors.Join(errors.New("migrate needs a client not pinned to an authorization model"), err)
	}
	stored, err := StoredSchemaVersion(ctx, fga)
	if err != nil {
		return fmt.Errorf("read openfga schema version: %w", err)
	}
	if stored > s.required {
		return fmt.Errorf("openfga schema version %d is ahead of required version %d; refusing to downgrade", stored, s.required)
	}
	todo := s.pending(stored)
	slog.Info("openfga migrations", "stored", stored, "required", s.required, "pending", len(todo))

	for _, m := range todo {
		if err := apply(ctx, fga, m); err != nil {
			return fmt.Errorf("migration %d %s: %w", m.Version, m.Name, err)
		}
	}
	// repairs a drifted model at the required version
	return writeModelIfChanged(ctx, fga, s.Model())
}

func apply(ctx context.Context, fga *client.OpenFgaClient, m Migration) error {
	slog.Info("applying migration", "version", m.Version, "name", m.Name)
	if m.Before != nil {
		if err := m.Before(ctx, fga); err != nil {
			return fmt.Errorf("before-model step: %w", err)
		}
	}
	if err := writeModelIfChanged(ctx, fga, m.Model); err != nil {
		return err
	}
	if m.After != nil {
		if err := m.After(ctx, fga); err != nil {
			return fmt.Errorf("after-model step: %w", err)
		}
	}
	return stampMarker(ctx, fga, m.Version)
}

func writeModelIfChanged(ctx context.Context, fga *client.OpenFgaClient, dsl string) error {
	latest, err := LatestModel(ctx, fga)
	if err != nil {
		return fmt.Errorf("read latest authorization model: %w", err)
	}
	if latest != nil {
		same, err := sameModel(latest, dsl)
		if err != nil {
			return fmt.Errorf("compare authorization models: %w", err)
		}
		if same {
			return nil
		}
	}
	model, err := dslToModel(dsl)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
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
