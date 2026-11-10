# fga-migrator

Versioned migrations for an [OpenFGA](https://openfga.dev) store.

A module that owns an OpenFGA authorization model declares a list of numbered
migrations, each carrying a frozen snapshot of the model as of its version and
optional data steps. This library applies the pending ones, records the store's
schema version inside the store itself, and lets services refuse to start until
the store is at exactly the version they were built against.

```bash
go get github.com/hierarchy40/fga-migrator
```

## Packages

| Package | What it offers |
|---------|----------------|
| `fgamigrator` (module root) | `Migration`, `Step`, `Set` (`NewSet`, `MustSet`), `Set.Migrate`, `StoredSchemaVersion`, `VerifySchemaVersion`, `LatestModel`, `FindStore`, `Run`, and the `FGA_*` `Config` |
| `fgamigrator/command` | ready-made `main` bodies: `command.Migrate(set)` and `command.SchemaVersion(set)` |
| `fgamigrator/fgatest` | a testcontainers OpenFGA harness for integration tests |

## The version marker

Every model must contain one reserved type:

```
type schema_version
  relations
    define applied: [schema_version]
```

Each applied version is one tuple: user `schema_version:<N>`, relation
`applied`, object `schema_version:history`. The stored version is the highest
`N`. It is `0` when the store has no model, when the latest model lacks the
`schema_version` type (even if marker tuples remain), or when there are no
marker tuples. Marker writes ignore duplicates.

## Declaring migrations

Keep one model file per version and list the migrations once:

```go
package migrations

import (
	_ "embed"

	fgamigrator "github.com/hierarchy40/fga-migrator"
)

// RequiredSchemaVersion is bumped together with every new migration.
const RequiredSchemaVersion = 3

var (
	//go:embed model/0001.fga
	model0001 string
	//go:embed model/0002.fga
	model0002 string
)

var Set = fgamigrator.MustSet(RequiredSchemaVersion,
	fgamigrator.Migration{Version: 1, Name: "baseline", Model: model0001},
	fgamigrator.Migration{Version: 2, Name: "add_writer", Model: model0002, After: copyEditorsToWriters},
	fgamigrator.Migration{Version: 3, Name: "backfill_owners", Model: model0002, After: backfillOwners},
)

// Model is the current model: the highest version's snapshot.
var Model = Set.Model()
```

`NewSet` (and `MustSet`, which panics instead of returning the error) checks:

- there is at least one migration, and versions are contiguous from 1
  (the list may be in any order; it is sorted by version);
- the highest version equals the required version passed in;
- every migration has a name and a model;
- every model parses and contains the `schema_version` type with its `applied`
  relation.

**Every migration declares its full model**, including a data-only migration:
it repeats the previous version's model (version 3 above reuses `model0002`).
There is no "empty means unchanged" shortcut, so a forgotten model is an error
rather than a silent no-op, and every version's model is visible in the source.

## How a run applies migrations

`Set.Migrate(ctx, fga)` needs a client bound to the store and **not** pinned to
an authorization model id (writes must validate against the latest model). It:

1. reads the stored version and fails if it is above the required version
   (no downgrade);
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
only valid under the new model. A pure model change has neither.

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

1. Copy the current model to a new snapshot file and edit it (or reuse the
   previous snapshot for a data-only migration).
2. Append a `Migration` with the next version, a name, the snapshot and any steps.
3. Bump `RequiredSchemaVersion`.

## Commands

`command.Migrate(set)` is a whole migrate job: it reads `Config` from the
environment plus `FGA_WAIT_TIMEOUT` (Go duration, default `5m`) and calls
`fgamigrator.Run`, which waits for OpenFGA up to that timeout, resolves the
store by `FGA_STORE_ID` or else by `FGA_STORE_NAME` (falling back to
`NAMESPACE`) and creates it by name when missing, migrates, and verifies.
`Run` is the only function here that creates stores.

`command.SchemaVersion(set)` prints the required version, or with
`-model-digest` the `sha256:<hex>` digest of the highest version's model after
a DSL → JSON → DSL round trip (comments and whitespace do not change it).

```go
// cmd/migrate/main.go
func main() { command.Migrate(migrations.Set) }

// cmd/schemaversion/main.go
func main() { command.SchemaVersion(migrations.Set) }
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
