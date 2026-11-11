# fga-migrator

Versioned migrations for an [OpenFGA](https://openfga.dev) store.

A module that owns an OpenFGA authorization model keeps one numbered model
snapshot per schema version, plus optional data steps in a Go file of the same
name. This library applies the pending versions, records the store's schema
version inside the store itself, and lets services refuse to start until the
store is at exactly the version they were built against.

```bash
go get github.com/hierarchy40/fga-migrator
```

## Packages

| Package | What it offers |
|---------|----------------|
| `fgamigrator` (module root) | `Migrations` (`NewMigrations`, `Discover`, `Register`, `Add`), `Migrator` (`Migrate`, `Status`, `Verify`), `Migration`, `Step`, `StoredSchemaVersion`, `VerifySchemaVersion`, `LatestModel`, `FindStore`, `Run`, and the `FGA_*` `Config` |
| `fgamigrator/command` | ready-made `main` bodies: `command.Migrate(migrations)` and `command.SchemaVersion(migrations)` |
| `fgamigrator/fgatest` | a testcontainers OpenFGA harness for integration tests |

## The version marker

The library adds one reserved type to every model it writes:

```
type schema_version
  relations
    define applied: [schema_version]
```

**Model files must not declare it.** A model that contains a `schema_version`
type is rejected when it is registered. The type is appended after the model's
own types, so a model that used to end with it keeps its digest.

Each applied version is one tuple: user `schema_version:<N>`, relation
`applied`, object `schema_version:history`. The stored version is the highest
`N`. It is `0` when the store has no model, when the latest model lacks the
`schema_version` type (even if marker tuples remain), or when there are no
marker tuples. Marker writes ignore duplicates.

## Declaring migrations

A version is the files that share one `NNNN_name` prefix:

```
migrations/
  migrations.go              the collection
  0001_baseline.fga          the model as of version 1
  0002_add_writer.fga        the model as of version 2
  0002_add_writer.go         the data steps of version 2
  0003_backfill_owners.fga   a copy of 0002_add_writer.fga
  0003_backfill_owners.go    the data steps of version 3
```

```go
// migrations.go
package migrations

import (
	"embed"

	fgamigrator "github.com/hierarchy40/fga-migrator"
)

// RequiredSchemaVersion is bumped together with every new migration.
const RequiredSchemaVersion = 3

var Migrations = fgamigrator.NewMigrations(fgamigrator.WithRequired(RequiredSchemaVersion))

//go:embed *.fga
var models embed.FS

func init() { Migrations.MustDiscover(models) }
```

```go
// 0002_add_writer.go
package migrations

func init() { Migrations.MustRegister(nil, copyEditorsToWriters) } // before, after
```

- `Discover(fsys)` registers the model of every `.fga` file in `fsys`, at any
  depth. Each must be named `NNNN_name.fga`; another `.fga` file is an error, so
  embed only the snapshots.
- `Register(before, after)` registers the steps of the version named by the
  **calling file**, which must be named `NNNN_name.go`. Either step may be `nil`.
- `Add(Migration{...})` registers a whole migration without reading file names.
- `MustDiscover`, `MustRegister` and `MustAdd` panic instead of returning the error.

`NNNN` is the version (leading zeros are optional) and `name` is lowercase
letters, digits, `_` or `-`. The files of one version must agree on the name.

**Every version has an `.fga` file**, including a data-only version: it copies
the previous snapshot (`0003` above). There is no "missing means unchanged"
shortcut, so a forgotten model is an error rather than a silent no-op, and every
version's model is visible in the source.

### Validation

Each file is checked as it registers: its name, that the version has no second
model or second set of steps, and that the model parses and does not declare
`schema_version`.

The collection as a whole is checked by `Validate()`, which `Migrate`, `Status`,
`Run`, `Required`, `Model` and `ModelDigest` all call first:

- there is at least one migration, and versions are contiguous from 1;
- every version has a model;
- with `WithRequired(n)`, the highest version equals `n`.

The required version is the highest registered version. `WithRequired` is
optional; it ties the collection to a constant that services can use without
running the collection's `init` functions first.

Call the accessors at run time, not from a package-level `var`: Go initialises
package variables before any `init` function, so the collection is still empty
then. A test that calls `Migrations.Validate()` covers the whole collection.

## How a run applies migrations

`NewMigrator(fga, migrations).Migrate(ctx)` needs a client bound to the store
and **not** pinned to an authorization model id (writes must validate against
the latest model). It:

1. validates the collection, reads the stored version and fails if it is above
   the required version (no downgrade);
2. applies each pending migration **one at a time, in ascending version order**:
   1. runs its `Before` step, against the model stored at that moment
      (the previous version's model, or none on a fresh store);
   2. writes **that migration's** model if the store's latest model differs
      (comparison is on the normalised DSL, so comment and whitespace edits do
      not count);
   3. runs its `After` step, against that migration's model;
   4. stamps that version's marker;
3. finally writes the highest version's model if the latest model still differs
   (repairs a drifted model at the required version).

A re-run at the required version with an unchanged model writes nothing.

`Before` is for reading data the new model drops; `After` is for writing tuples
only valid under the new model. A pure model change has neither, and no `.go` file.

`Migrator.Status(ctx)` lists every migration with whether the store has it
applied, and `Migrator.Verify(ctx)` fails unless the store is at the highest
version.

### Resumability and idempotency

Markers are stamped per migration, so a failed run is resumable: the versions
already stamped are skipped next time and the run continues at the failed one.

**Steps must be idempotent.** A crash between a step and its stamp re-runs that
whole migration; in that case its `Before` step may see that migration's own
model already written. A store whose model was overwritten by an older writer
(latest model without `schema_version`) reads as version `0` and replays every
migration from 1. Use the SDK's ignore-duplicate-writes and
ignore-missing-deletes options. Never edit or renumber a released migration; add
a new one.

## Adding a migration

1. Copy the highest snapshot to `NNNN_name.fga` with the next number and edit it
   (leave it unchanged for a data-only migration).
2. If the version moves data, add `NNNN_name.go` with an `init` that calls
   `Migrations.MustRegister(before, after)`.
3. Bump `RequiredSchemaVersion`, if the module keeps one.

## Commands

`command.Migrate(migrations)` is a whole migrate job: it reads `Config` from the
environment plus `FGA_WAIT_TIMEOUT` (Go duration, default `5m`) and calls
`fgamigrator.Run`, which waits for OpenFGA up to that timeout, resolves the
store by `FGA_STORE_ID` or else by `FGA_STORE_NAME` (falling back to
`NAMESPACE`) and creates it by name when missing, migrates, and verifies.
`Run` is the only function here that creates stores.

`command.SchemaVersion(migrations)` prints the required version, or with
`-model-digest` the `sha256:<hex>` digest of the highest version's model as it
is stored, `schema_version` type included, after a DSL → JSON → DSL round trip
(comments and whitespace do not change it). `Migrations.Model()` returns that
same normalised DSL.

```go
// cmd/migrate/main.go
func main() { command.Migrate(migrations.Migrations) }

// cmd/schemaversion/main.go
func main() { command.SchemaVersion(migrations.Migrations) }
```

## Verifying in a service

A service must not write models. At startup it resolves the store and checks the
version, then pins its client to the latest model:

```go
if err := fgamigrator.VerifySchemaVersion(ctx, fga, migrations.RequiredSchemaVersion); err != nil {
	return err // the migrate job has not run yet
}
latest, err := fgamigrator.LatestModel(ctx, fga)
```

`VerifySchemaVersion` requires the stored version to equal the required one.
`migrations.Migrations.Required()` gives the same number for a module without
the constant.

## Configuration

`Config` carries `envconfig` tags; embed it in a larger config struct or call
`LoadConfig()`. `WithDefaults` trims trailing slashes from the API URL, falls
back to `NAMESPACE` for the store name, and defaults the auth method to `none`.

| Variable | Default | Meaning |
|----------|---------|---------|
| `FGA_API_URL` | `http://openfga:8080` | OpenFGA HTTP API |
| `FGA_STORE_ID` | | store id; takes precedence over the name |
| `FGA_STORE_NAME` | `$NAMESPACE` | store name, used when no id is set |
| `FGA_AUTH_METHOD` | `none` | `none`, `api_token` or `client_credentials` |
| `FGA_API_TOKEN` | | token for `api_token` |
| `FGA_CLIENT_CREDENTIALS_API_TOKEN_ISSUER` | | for `client_credentials` |
| `FGA_CLIENT_CREDENTIALS_API_AUDIENCE` | | for `client_credentials` |
| `FGA_CLIENT_CREDENTIALS_CLIENT_ID` | | for `client_credentials` |
| `FGA_CLIENT_CREDENTIALS_CLIENT_SECRET` | | for `client_credentials` |
| `FGA_CLIENT_CREDENTIALS_SCOPES` | | for `client_credentials` |
| `FGA_WAIT_TIMEOUT` | `5m` | `command.Migrate` only: how long to wait for OpenFGA |

## Integration tests with `fgatest`

```go
//go:build integration

func TestMain(m *testing.M) { fgatest.Main(m) }

func TestSomething(t *testing.T) {
	fga, storeID := fgatest.NewStore(ctx, t) // empty store, unpinned client
	...
}
```

`fgatest.APIURL()`, `fgatest.Client(t)` and `fgatest.StoreName(t)` cover the
`Run`-style tests that need their own store name.

## Tests

```bash
go test -race -shuffle=on ./...
go test -race -shuffle=on -tags integration ./...   # needs Docker
golangci-lint run ./...
```
