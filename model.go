package fgamigrator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	openfga "github.com/openfga/go-sdk"
	language "github.com/openfga/language/pkg/go/transformer"
)

func modelDigest(dsl string) (string, error) {
	normalised, err := normaliseDSL(dsl)
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

func sameModel(stored *openfga.AuthorizationModel, dsl string) (bool, error) {
	storedDSL, err := modelToDSL(stored)
	if err != nil {
		return false, err
	}
	a, err := normaliseDSL(storedDSL)
	if err != nil {
		return false, err
	}
	b, err := normaliseDSL(dsl)
	if err != nil {
		return false, err
	}
	return a == b, nil
}
