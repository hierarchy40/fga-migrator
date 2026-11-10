# CLAUDE.md

OpenFGA migration engine library (`github.com/hierarchy40/fga-migrator`).
See README.md for the design.

## Rules

- Keep the marker contract stable: type `schema_version`, relation `applied`,
  object `schema_version:history`, user `schema_version:<N>`.
- Migrations apply one at a time in ascending version order; never batch steps
  or model writes across versions.
- The caller owns its migrations; no package-level registry in this module.
- Test models live in `testdata/` and stay neutral (documents, folders).
- Commit subjects use `feat:`, `fix:`, `docs:`, `chore:`, `ref:` or `test:`.
- Comments: default to none; at most one short phrase.

## Checks

```bash
go vet ./... && go vet -tags integration ./...
go test -race -shuffle=on ./...
go test -race -shuffle=on -tags integration ./...   # needs Docker
golangci-lint run ./...
```
