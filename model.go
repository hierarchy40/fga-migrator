package fgamigrator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	openfga "github.com/openfga/go-sdk"
	language "github.com/openfga/language/pkg/go/transformer"
)

const markerDSL = `model
  schema 1.1

type ` + markerType + `
  relations
    define ` + markerRelation + `: [` + markerType + `]
`

var markerDefinition = mustMarkerDefinition()

func mustMarkerDefinition() openfga.TypeDefinition {
	model, err := dslToModel(markerDSL)
	if err != nil {
		panic(err)
	}
	return model.TypeDefinitions[0]
}

// effectiveModel is the caller's model plus the marker type.
func effectiveModel(dsl string) (*openfga.AuthorizationModel, error) {
	model, err := dslToModel(dsl)
	if err != nil {
		return nil, err
	}
	if hasType(model, markerType) {
		return nil, fmt.Errorf("model declares the reserved %s type: remove it, the migrator adds it", markerType)
	}
	model.TypeDefinitions = append(model.TypeDefinitions, markerDefinition)
	return model, nil
}

func modelDigest(model *openfga.AuthorizationModel) (string, error) {
	normalised, err := normalisedDSL(model)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(normalised))), nil
}

func normaliseDSL(dsl string) (string, error) {
	jsonString, err := language.TransformDSLToJSON(dsl)
	if err != nil {
		return "", err
	}
	normalised, err := language.TransformJSONStringToDSL(jsonString)
	if err != nil {
		return "", err
	}
	return *normalised, nil
}

func dslToModel(dsl string) (*openfga.AuthorizationModel, error) {
	jsonString, err := language.TransformDSLToJSON(dsl)
	if err != nil {
		return nil, err
	}
	var model openfga.AuthorizationModel
	if err := json.Unmarshal([]byte(jsonString), &model); err != nil {
		return nil, err
	}
	return &model, nil
}

func modelToDSL(model *openfga.AuthorizationModel) (string, error) {
	raw, err := model.MarshalJSON()
	if err != nil {
		return "", err
	}
	dsl, err := language.TransformJSONStringToDSL(string(raw))
	if err != nil {
		return "", err
	}
	return *dsl, nil
}

func normalisedDSL(model *openfga.AuthorizationModel) (string, error) {
	dsl, err := modelToDSL(model)
	if err != nil {
		return "", err
	}
	return normaliseDSL(dsl)
}

func sameModel(a, b *openfga.AuthorizationModel) (bool, error) {
	left, err := normalisedDSL(a)
	if err != nil {
		return false, err
	}
	right, err := normalisedDSL(b)
	if err != nil {
		return false, err
	}
	return left == right, nil
}
