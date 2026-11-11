package fgamigrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

const (
	markerType     = "schema_version"
	markerRelation = "applied"
	markerObject   = markerType + ":history"
)

// LatestModel is nil when the store has no model.
func LatestModel(ctx context.Context, fga *client.OpenFgaClient) (*openfga.AuthorizationModel, error) {
	res, err := fga.ReadAuthorizationModels(ctx).
		Options(client.ClientReadAuthorizationModelsOptions{PageSize: openfga.PtrInt32(1)}).
		Execute()
	if err != nil {
		return nil, err
	}
	if len(res.AuthorizationModels) == 0 {
		return nil, nil
	}
	return &res.AuthorizationModels[0], nil
}

// StoredSchemaVersion is 0 while the latest model lacks the marker type.
func StoredSchemaVersion(ctx context.Context, fga *client.OpenFgaClient) (int, error) {
	model, err := LatestModel(ctx, fga)
	if err != nil {
		return 0, err
	}
	if model == nil || !hasType(model, markerType) {
		return 0, nil
	}

	version := 0
	relation, object := markerRelation, markerObject
	var token *string
	for {
		res, err := fga.Read(ctx).
			Body(client.ClientReadRequest{Relation: &relation, Object: &object}).
			Options(client.ClientReadOptions{ContinuationToken: token}).
			Execute()
		if err != nil {
			return 0, err
		}
		for _, tuple := range res.GetTuples() {
			n, err := parseMarker(tuple.Key.User)
			if err != nil {
				return 0, err
			}
			version = max(version, n)
		}
		next := res.GetContinuationToken()
		if next == "" {
			return version, nil
		}
		token = &next
	}
}

// VerifySchemaVersion fails unless the stored version equals required.
func VerifySchemaVersion(ctx context.Context, fga *client.OpenFgaClient, required int) error {
	version, err := StoredSchemaVersion(ctx, fga)
	if err != nil {
		return fmt.Errorf("read openfga schema version: %w", err)
	}
	if version != required {
		return fmt.Errorf("openfga schema version %d does not match required version %d; migrations may not have run", version, required)
	}
	return nil
}

func hasType(model *openfga.AuthorizationModel, typeName string) bool {
	for _, t := range model.TypeDefinitions {
		if t.Type == typeName {
			return true
		}
	}
	return false
}

func markerUser(version int) string {
	return markerType + ":" + strconv.Itoa(version)
}

func parseMarker(user string) (int, error) {
	id, ok := strings.CutPrefix(user, markerType+":")
	if !ok {
		return 0, fmt.Errorf("unexpected schema version marker %q", user)
	}
	n, err := strconv.Atoi(id)
	if err != nil {
		return 0, fmt.Errorf("unexpected schema version marker %q: %w", user, err)
	}
	return n, nil
}
