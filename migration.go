// Package fgamigrator versions an OpenFGA store with numbered migrations and a schema_version marker.
package fgamigrator

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/openfga/go-sdk/client"
)

// Step must be idempotent: a crash before its version is stamped re-runs it.
type Step func(ctx context.Context, fga *client.OpenFgaClient) error

// Migration is one schema version: its full model snapshot and optional data steps.
type Migration struct {
	Version int
	Name    string
	Model   string // full DSL as of this version
	Before  Step   // against the model stored before this version
	After   Step   // against this version's Model
}

// Set is a validated, ordered list of migrations; build it with NewSet.
type Set struct {
	required   int
	migrations []Migration
}

// NewSet validates the migrations against the required schema version.
func NewSet(required int, ms ...Migration) (*Set, error) {
	sorted := slices.Clone(ms)
	slices.SortStableFunc(sorted, func(a, b Migration) int { return cmp.Compare(a.Version, b.Version) })
	if err := validate(sorted, required); err != nil {
		return nil, err
	}
	return &Set{required: required, migrations: sorted}, nil
}

// MustSet is NewSet that panics on an invalid set.
func MustSet(required int, ms ...Migration) *Set {
	s, err := NewSet(required, ms...)
	if err != nil {
		panic(err)
	}
	return s
}

// Required is the schema version the set migrates to.
func (s *Set) Required() int { return s.required }

// Migrations returns a copy in ascending version order.
func (s *Set) Migrations() []Migration { return slices.Clone(s.migrations) }

// Model is the DSL of the highest version.
func (s *Set) Model() string { return s.migrations[len(s.migrations)-1].Model }

// ModelDigest is sha256:<hex> of the normalised Model, blind to comments and whitespace.
func (s *Set) ModelDigest() (string, error) { return modelDigest(s.Model()) }

func (s *Set) pending(stored int) []Migration {
	var out []Migration
	for _, m := range s.migrations {
		if m.Version > stored {
			out = append(out, m)
		}
	}
	return out
}

func validate(ms []Migration, required int) error {
	if len(ms) == 0 {
		return errors.New("no migrations: version 1 must declare the baseline model")
	}
	for i, m := range ms {
		if m.Version != i+1 {
			return fmt.Errorf("migration %q has version %d, want %d: versions must be contiguous from 1", m.Name, m.Version, i+1)
		}
		if m.Name == "" {
			return fmt.Errorf("migration %d has no name", m.Version)
		}
		if m.Model == "" {
			return fmt.Errorf("migration %d %s has no model: every version declares its full model snapshot", m.Version, m.Name)
		}
		model, err := dslToModel(m.Model)
		if err != nil {
			return fmt.Errorf("migration %d %s model: %w", m.Version, m.Name, err)
		}
		if !hasMarkerRelation(model) {
			return fmt.Errorf("migration %d %s model lacks the %s type with relation %s", m.Version, m.Name, markerType, markerRelation)
		}
	}
	if len(ms) != required {
		return fmt.Errorf("highest migration version is %d, want required schema version %d", len(ms), required)
	}
	return nil
}
