package fgamigrator

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"

	openfga "github.com/openfga/go-sdk"
)

const (
	modelExt = ".fga"
	stepsExt = ".go"
)

var (
	fileNameRE = regexp.MustCompile(`^(\d+)_([0-9a-z_\-]+)(\.[a-z]+)$`)
	nameRE     = regexp.MustCompile(`^[0-9a-z_\-]+$`)
)

// Migrations is the caller's collection, filled from NNNN_name.fga and NNNN_name.go files.
type Migrations struct {
	entries  map[int]*entry
	required int
}

type entry struct {
	Migration
	model    *openfga.AuthorizationModel
	hasSteps bool
}

// MigrationsOption configures NewMigrations.
type MigrationsOption func(*Migrations)

// WithRequired fails validation unless the highest version equals version.
func WithRequired(version int) MigrationsOption {
	return func(m *Migrations) { m.required = version }
}

// NewMigrations returns an empty collection.
func NewMigrations(opts ...MigrationsOption) *Migrations {
	m := &Migrations{entries: map[int]*entry{}}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Add registers a whole migration without reading file names.
func (m *Migrations) Add(migration Migration) error {
	if err := m.setModel(migration.Version, migration.Name, migration.Model); err != nil {
		return err
	}
	if migration.Before == nil && migration.After == nil {
		return nil
	}
	return m.setSteps(migration.Version, migration.Name, migration.Before, migration.After)
}

// MustAdd is Add that panics on error.
func (m *Migrations) MustAdd(migration Migration) {
	if err := m.Add(migration); err != nil {
		panic(err)
	}
}

// Discover registers the model of every NNNN_name.fga file in fsys.
func (m *Migrations) Discover(fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path.Ext(file) != modelExt {
			return nil
		}
		version, name, err := parseFileName(path.Base(file), modelExt)
		if err != nil {
			return err
		}
		dsl, err := fs.ReadFile(fsys, file)
		if err != nil {
			return err
		}
		return m.setModel(version, name, string(dsl))
	})
}

// MustDiscover is Discover that panics on error.
func (m *Migrations) MustDiscover(fsys fs.FS) {
	if err := m.Discover(fsys); err != nil {
		panic(err)
	}
}

// Register adds the steps of the migration named by the calling NNNN_name.go file.
func (m *Migrations) Register(before, after Step) error {
	_, file, _, _ := runtime.Caller(1)
	return m.registerFile(file, before, after)
}

// MustRegister is Register that panics on error.
func (m *Migrations) MustRegister(before, after Step) {
	_, file, _, _ := runtime.Caller(1)
	if err := m.registerFile(file, before, after); err != nil {
		panic(err)
	}
}

func (m *Migrations) registerFile(file string, before, after Step) error {
	version, name, err := parseFileName(filepath.Base(file), stepsExt)
	if err != nil {
		return err
	}
	return m.setSteps(version, name, before, after)
}

// Validate checks the collection as a whole; call it once every file has registered.
func (m *Migrations) Validate() error {
	_, err := m.sorted()
	return err
}

// Sorted returns the validated migrations in ascending version order.
func (m *Migrations) Sorted() ([]Migration, error) {
	entries, err := m.sorted()
	if err != nil {
		return nil, err
	}
	out := make([]Migration, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Migration)
	}
	return out, nil
}

// Required is the highest version, the one a migrated store is at.
func (m *Migrations) Required() (int, error) {
	entries, err := m.sorted()
	if err != nil {
		return 0, err
	}
	return len(entries), nil
}

// Model is the normalised DSL of the highest version, including the schema_version type.
func (m *Migrations) Model() (string, error) {
	latest, err := m.latest()
	if err != nil {
		return "", err
	}
	return normalisedDSL(latest.model)
}

// ModelDigest is sha256:<hex> of Model, blind to comments and whitespace.
func (m *Migrations) ModelDigest() (string, error) {
	latest, err := m.latest()
	if err != nil {
		return "", err
	}
	return modelDigest(latest.model)
}

func (m *Migrations) latest() (*entry, error) {
	entries, err := m.sorted()
	if err != nil {
		return nil, err
	}
	return entries[len(entries)-1], nil
}

func (m *Migrations) sorted() ([]*entry, error) {
	if len(m.entries) == 0 {
		return nil, errors.New("no migrations: version 1 must declare the baseline model")
	}
	entries := make([]*entry, 0, len(m.entries))
	for _, e := range m.entries {
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(a, b *entry) int { return cmp.Compare(a.Version, b.Version) })
	for i, e := range entries {
		if e.Version != i+1 {
			return nil, fmt.Errorf("migration %d %s: versions must be contiguous from 1, want version %d", e.Version, e.Name, i+1)
		}
		if e.model == nil {
			return nil, fmt.Errorf("migration %d %s has no model: add %s", e.Version, e.Name, fileName(e.Version, e.Name, modelExt))
		}
	}
	if m.required != 0 && len(entries) != m.required {
		return nil, fmt.Errorf("highest migration version is %d, want required schema version %d", len(entries), m.required)
	}
	return entries, nil
}

func (m *Migrations) setModel(version int, name, dsl string) error {
	if dsl == "" {
		return fmt.Errorf("migration %d %s has no model: every version declares its full model snapshot", version, name)
	}
	model, err := effectiveModel(dsl)
	if err != nil {
		return fmt.Errorf("migration %d %s model: %w", version, name, err)
	}
	e, err := m.entry(version, name)
	if err != nil {
		return err
	}
	if e.model != nil {
		return fmt.Errorf("migration %d %s already has a model", version, name)
	}
	e.Model, e.model = dsl, model
	return nil
}

func (m *Migrations) setSteps(version int, name string, before, after Step) error {
	e, err := m.entry(version, name)
	if err != nil {
		return err
	}
	if e.hasSteps {
		return fmt.Errorf("migration %d %s already has steps", version, name)
	}
	e.Before, e.After, e.hasSteps = before, after, true
	return nil
}

func (m *Migrations) entry(version int, name string) (*entry, error) {
	if version < 1 {
		return nil, fmt.Errorf("migration %q has version %d: versions start at 1", name, version)
	}
	if !nameRE.MatchString(name) {
		return nil, fmt.Errorf("migration %d has name %q: want lowercase letters, digits, _ or -", version, name)
	}
	e, ok := m.entries[version]
	if !ok {
		e = &entry{Migration: Migration{Version: version, Name: name}}
		m.entries[version] = e
		return e, nil
	}
	if e.Name != name {
		return nil, fmt.Errorf("version %d belongs to both %s and %s", version, e.Name, name)
	}
	return e, nil
}

func parseFileName(base, ext string) (version int, name string, err error) {
	match := fileNameRE.FindStringSubmatch(base)
	if match == nil || match[3] != ext {
		return 0, "", fmt.Errorf("unsupported migration file name %q: want NNNN_name%s", base, ext)
	}
	version, err = strconv.Atoi(match[1])
	if err != nil {
		return 0, "", fmt.Errorf("unsupported migration file name %q: %w", base, err)
	}
	return version, match[2], nil
}

func fileName(version int, name, ext string) string {
	return fmt.Sprintf("%04d_%s%s", version, name, ext)
}
