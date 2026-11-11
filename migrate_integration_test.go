//go:build integration

package fgamigrator

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hierarchy40/fga-migrator/fgatest"
	openfga "github.com/openfga/go-sdk"
	"github.com/openfga/go-sdk/client"
)

func TestMain(m *testing.M) { fgatest.Main(m) }

func testContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func mustMigrate(ctx context.Context, t *testing.T, s *Set, fga *client.OpenFgaClient) {
	t.Helper()
	if err := s.Migrate(ctx, fga); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func storedVersion(ctx context.Context, t *testing.T, fga *client.OpenFgaClient) int {
	t.Helper()
	v, err := StoredSchemaVersion(ctx, fga)
	if err != nil {
		t.Fatalf("stored schema version: %v", err)
	}
	return v
}

func modelCount(ctx context.Context, t *testing.T, fga *client.OpenFgaClient) int {
	t.Helper()
	res, err := fga.ReadAuthorizationModels(ctx).Execute()
	if err != nil {
		t.Fatalf("read models: %v", err)
	}
	return len(res.AuthorizationModels)
}

func readUsers(ctx context.Context, t *testing.T, fga *client.OpenFgaClient, relation, object string) []string {
	t.Helper()
	users, err := usersOf(ctx, fga, relation, object)
	if err != nil {
		t.Fatalf("read %s %s: %v", relation, object, err)
	}
	return users
}

func usersOf(ctx context.Context, fga *client.OpenFgaClient, relation, object string) ([]string, error) {
	res, err := fga.Read(ctx).Body(client.ClientReadRequest{Relation: &relation, Object: &object}).Execute()
	if err != nil {
		return nil, err
	}
	var users []string
	for _, tuple := range res.GetTuples() {
		users = append(users, tuple.Key.User)
	}
	slices.Sort(users)
	return users, nil
}

func markers(ctx context.Context, t *testing.T, fga *client.OpenFgaClient) []string {
	t.Helper()
	return readUsers(ctx, t, fga, markerRelation, markerObject)
}

func markerUsers(versions ...int) []string {
	out := make([]string, 0, len(versions))
	for _, v := range versions {
		out = append(out, markerUser(v))
	}
	slices.Sort(out)
	return out
}

func writeUnmarked(ctx context.Context, t *testing.T, fga *client.OpenFgaClient, dsl string) {
	t.Helper()
	model, err := dslToModel(dsl)
	if err != nil {
		t.Fatal(err)
	}
	writeModel(ctx, t, fga, model)
}

func writeMarked(ctx context.Context, t *testing.T, fga *client.OpenFgaClient, dsl string) {
	t.Helper()
	writeModel(ctx, t, fga, mustEffective(t, dsl))
}

func writeModel(ctx context.Context, t *testing.T, fga *client.OpenFgaClient, model *openfga.AuthorizationModel) {
	t.Helper()
	_, err := fga.WriteAuthorizationModel(ctx).Body(client.ClientWriteAuthorizationModelRequest{
		TypeDefinitions: model.TypeDefinitions,
		SchemaVersion:   model.SchemaVersion,
		Conditions:      model.Conditions,
	}).Execute()
	if err != nil {
		t.Fatalf("write model: %v", err)
	}
}

func writeTuples(ctx context.Context, t *testing.T, fga *client.OpenFgaClient, tuples ...client.ClientTupleKey) {
	t.Helper()
	if _, err := fga.Write(ctx).Body(client.ClientWriteRequest{Writes: tuples}).Execute(); err != nil {
		t.Fatalf("write tuples: %v", err)
	}
}

func check(ctx context.Context, t *testing.T, fga *client.OpenFgaClient, user, relation, object string) bool {
	t.Helper()
	res, err := fga.Check(ctx).Body(client.ClientCheckRequest{User: user, Relation: relation, Object: object}).Execute()
	if err != nil {
		t.Fatalf("check %s %s %s: %v", user, relation, object, err)
	}
	return res.GetAllowed()
}

func latestIs(ctx context.Context, fga *client.OpenFgaClient, dsl string) (bool, error) {
	want, err := effectiveModel(dsl)
	if err != nil {
		return false, err
	}
	return latestIsModel(ctx, fga, want)
}

func latestIsUnmarked(ctx context.Context, fga *client.OpenFgaClient, dsl string) (bool, error) {
	want, err := dslToModel(dsl)
	if err != nil {
		return false, err
	}
	return latestIsModel(ctx, fga, want)
}

func latestIsModel(ctx context.Context, fga *client.OpenFgaClient, want *openfga.AuthorizationModel) (bool, error) {
	latest, err := LatestModel(ctx, fga)
	if err != nil || latest == nil {
		return false, err
	}
	return sameModel(latest, want)
}

func assertAtVersion(ctx context.Context, t *testing.T, s *Set, fga *client.OpenFgaClient) {
	t.Helper()
	if err := VerifySchemaVersion(ctx, fga, s.Required()); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if same, err := latestIs(ctx, fga, s.Model()); err != nil || !same {
		t.Fatalf("latest model is not the set's model (same=%v, err=%v)", same, err)
	}
}

// renameSet records the model each step sees.
func renameSet(seen map[string]string) *Set {
	observe := func(step string) Step {
		return func(ctx context.Context, fga *client.OpenFgaClient) error {
			seen[step] = "none"
			if legacy, err := latestIsUnmarked(ctx, fga, v1Model); err != nil || legacy {
				seen[step] = "legacy"
				return err
			}
			for name, dsl := range map[string]string{"v1": v1Model, "v2": v2Model, "v3": v3Model} {
				same, err := latestIs(ctx, fga, dsl)
				if err != nil {
					return err
				}
				if same {
					seen[step] = name
				}
			}
			return nil
		}
	}
	and := func(a, b Step) Step {
		return func(ctx context.Context, fga *client.OpenFgaClient) error {
			if err := a(ctx, fga); err != nil {
				return err
			}
			return b(ctx, fga)
		}
	}
	copyEditors := func(ctx context.Context, fga *client.OpenFgaClient) error {
		editors, err := usersOf(ctx, fga, "editor", "document:1")
		if err != nil || len(editors) == 0 {
			return err
		}
		writes := make([]client.ClientTupleKey, 0, len(editors))
		for _, user := range editors {
			writes = append(writes, client.ClientTupleKey{User: user, Relation: "writer", Object: "document:1"})
		}
		_, err = fga.Write(ctx).Body(client.ClientWriteRequest{Writes: writes}).
			Options(client.ClientWriteOptions{Conflict: client.ClientWriteConflictOptions{
				OnDuplicateWrites: client.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
			}}).Execute()
		return err
	}
	dropEditors := func(ctx context.Context, fga *client.OpenFgaClient) error {
		editors, err := usersOf(ctx, fga, "editor", "document:1")
		if err != nil || len(editors) == 0 {
			return err
		}
		deletes := make([]client.ClientTupleKeyWithoutCondition, 0, len(editors))
		for _, user := range editors {
			deletes = append(deletes, client.ClientTupleKeyWithoutCondition{User: user, Relation: "editor", Object: "document:1"})
		}
		_, err = fga.Write(ctx).Body(client.ClientWriteRequest{Deletes: deletes}).
			Options(client.ClientWriteOptions{Conflict: client.ClientWriteConflictOptions{
				OnMissingDeletes: client.CLIENT_WRITE_REQUEST_ON_MISSING_DELETES_IGNORE,
			}}).Execute()
		return err
	}
	return MustSet(3,
		Migration{Version: 1, Name: "baseline", Model: v1Model, Before: observe("1.before"), After: observe("1.after")},
		Migration{Version: 2, Name: "add_writer", Model: v2Model, Before: observe("2.before"), After: and(observe("2.after"), copyEditors)},
		Migration{Version: 3, Name: "drop_editor", Model: v3Model, Before: and(observe("3.before"), dropEditors), After: observe("3.after")},
	)
}

func TestMigrateFreshStore(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	s := mustSet(t, baseline())

	if v := storedVersion(ctx, t, fga); v != 0 {
		t.Fatalf("fresh store version = %d, want 0", v)
	}
	if err := VerifySchemaVersion(ctx, fga, 1); err == nil {
		t.Fatal("verify passed on a fresh store")
	}

	mustMigrate(ctx, t, s, fga)

	assertAtVersion(ctx, t, s, fga)
	if got, want := markers(ctx, t, fga), markerUsers(1); !slices.Equal(got, want) {
		t.Fatalf("markers = %v, want %v", got, want)
	}
}

func TestMigrateRerunIsNoop(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	s := renameSet(map[string]string{})
	mustMigrate(ctx, t, s, fga)
	models, stamped := modelCount(ctx, t, fga), markers(ctx, t, fga)

	seen := map[string]string{}
	again := renameSet(seen)
	mustMigrate(ctx, t, again, fga)

	if got := modelCount(ctx, t, fga); got != models {
		t.Fatalf("re-run wrote a model: %d models, want %d", got, models)
	}
	if got := markers(ctx, t, fga); !slices.Equal(got, stamped) {
		t.Fatalf("re-run markers = %v, want %v", got, stamped)
	}
	if len(seen) != 0 {
		t.Fatalf("re-run ran steps %v", seen)
	}
	assertAtVersion(ctx, t, again, fga)
}

func TestMigrateRefusesDowngrade(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	s := mustSet(t, baseline())
	mustMigrate(ctx, t, s, fga)
	writeTuples(ctx, t, fga, client.ClientTupleKey{User: markerUser(2), Relation: markerRelation, Object: markerObject})

	err := s.Migrate(ctx, fga)
	if err == nil || !strings.Contains(err.Error(), "refusing to downgrade") {
		t.Fatalf("migrate with a newer store: err = %v, want downgrade refusal", err)
	}
	if err := VerifySchemaVersion(ctx, fga, 1); err == nil {
		t.Fatal("verify passed on a store ahead of the required version")
	}
}

func TestMigrateLegacyModelWithoutMarkerType(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	writeUnmarked(ctx, t, fga, v1Model)
	writeTuples(ctx, t, fga,
		client.ClientTupleKey{User: "user:1", Relation: "viewer", Object: "folder:1"},
		client.ClientTupleKey{User: "folder:1", Relation: "parent", Object: "document:1"},
	)

	if v := storedVersion(ctx, t, fga); v != 0 {
		t.Fatalf("legacy store version = %d, want 0", v)
	}

	s := mustSet(t, baseline())
	mustMigrate(ctx, t, s, fga)

	assertAtVersion(ctx, t, s, fga)
	if !check(ctx, t, fga, "user:1", "viewer", "document:1") {
		t.Fatal("existing tuples no longer resolve after migration")
	}
}

func TestMigrateAppliesMigrationsOneAtATime(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	writeUnmarked(ctx, t, fga, v1Model)
	writeTuples(ctx, t, fga, client.ClientTupleKey{User: "user:7", Relation: "editor", Object: "document:1"})

	seen := map[string]string{}
	s := renameSet(seen)
	mustMigrate(ctx, t, s, fga)

	want := map[string]string{
		"1.before": "legacy", "1.after": "v1",
		"2.before": "v1", "2.after": "v2",
		"3.before": "v2", "3.after": "v3",
	}
	for step, model := range want {
		if seen[step] != model {
			t.Errorf("step %s saw model %q, want %q (all: %v)", step, seen[step], model, seen)
		}
	}
	assertAtVersion(ctx, t, s, fga)
	if got, want := markers(ctx, t, fga), markerUsers(1, 2, 3); !slices.Equal(got, want) {
		t.Fatalf("markers = %v, want %v", got, want)
	}
	if !check(ctx, t, fga, "user:7", "writer", "document:1") {
		t.Fatal("v2 after step did not copy the editor to writer before v3 dropped editors")
	}
	if got := readUsers(ctx, t, fga, "editor", "document:1"); len(got) != 0 {
		t.Fatalf("editor tuples left after v3: %v", got)
	}

	if err := mustSet(t, baseline()).Migrate(ctx, fga); err == nil {
		t.Fatal("version-1 set accepted a version-3 store")
	}
}

func TestMigrateResumesAfterAFailedMigration(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	runs := map[string]int{}
	count := func(step string, fail *bool) Step {
		return func(context.Context, *client.OpenFgaClient) error {
			runs[step]++
			if fail != nil && *fail {
				return errors.New("boom")
			}
			return nil
		}
	}
	failing := true
	s := MustSet(3,
		Migration{Version: 1, Name: "baseline", Model: v1Model, After: count("1.after", nil)},
		Migration{Version: 2, Name: "add_writer", Model: v2Model, Before: count("2.before", nil), After: count("2.after", &failing)},
		Migration{Version: 3, Name: "drop_editor", Model: v3Model, Before: count("3.before", nil)},
	)

	err := s.Migrate(ctx, fga)
	if err == nil || !strings.Contains(err.Error(), "migration 2 add_writer") {
		t.Fatalf("err = %v, want a migration 2 failure", err)
	}
	if v := storedVersion(ctx, t, fga); v != 1 {
		t.Fatalf("version after a failure in 2 = %d, want 1", v)
	}
	if got, want := markers(ctx, t, fga), markerUsers(1); !slices.Equal(got, want) {
		t.Fatalf("markers = %v, want %v", got, want)
	}
	if runs["3.before"] != 0 {
		t.Fatal("migration 3 ran after migration 2 failed")
	}

	failing = false
	mustMigrate(ctx, t, s, fga)

	if want := map[string]int{"1.after": 1, "2.before": 2, "2.after": 2, "3.before": 1}; !maps.Equal(runs, want) {
		t.Fatalf("step runs = %v, want %v", runs, want)
	}
	assertAtVersion(ctx, t, s, fga)
	if got, want := markers(ctx, t, fga), markerUsers(1, 2, 3); !slices.Equal(got, want) {
		t.Fatalf("markers = %v, want %v", got, want)
	}
}

func TestMigrateAfterOldWriterOverwroteModel(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	writeUnmarked(ctx, t, fga, v1Model)
	writeTuples(ctx, t, fga, client.ClientTupleKey{User: "user:7", Relation: "editor", Object: "document:1"})
	mustMigrate(ctx, t, renameSet(map[string]string{}), fga)
	stamped := markers(ctx, t, fga)

	writeUnmarked(ctx, t, fga, v1Model)

	if v := storedVersion(ctx, t, fga); v != 0 {
		t.Fatalf("version under a model without the marker type = %d, want 0", v)
	}
	if err := VerifySchemaVersion(ctx, fga, 3); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("verify under a model without the marker type: err = %v, want version mismatch", err)
	}

	seen := map[string]string{}
	s := renameSet(seen)
	mustMigrate(ctx, t, s, fga)

	assertAtVersion(ctx, t, s, fga)
	if got := markers(ctx, t, fga); !slices.Equal(got, stamped) {
		t.Fatalf("markers = %v, want %v (no duplicates)", got, stamped)
	}
	if seen["1.before"] != "legacy" || seen["3.after"] != "v3" {
		t.Fatalf("recovery did not replay every version from the legacy model: %v", seen)
	}
	if !check(ctx, t, fga, "user:7", "viewer", "document:1") {
		t.Fatal("data lost while replaying the migrations")
	}
}

func TestMigrateRepairsDriftedModelAtRequiredVersion(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	s := mustSet(t, baseline(), Migration{Version: 2, Name: "add_writer", Model: v2Model})
	mustMigrate(ctx, t, s, fga)
	writeMarked(ctx, t, fga, v1Model)

	mustMigrate(ctx, t, s, fga)

	assertAtVersion(ctx, t, s, fga)
}

func TestMigrateRejectsPinnedClient(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	s := mustSet(t, baseline())
	mustMigrate(ctx, t, s, fga)
	latest, err := LatestModel(ctx, fga)
	if err != nil {
		t.Fatal(err)
	}
	if err := fga.SetAuthorizationModelId(latest.Id); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx, fga); err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Fatalf("err = %v, want a pinned-client refusal", err)
	}
}

func TestStoredVersionWithoutModel(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	if latest, err := LatestModel(ctx, fga); err != nil || latest != nil {
		t.Fatalf("LatestModel on an empty store = %v, %v", latest, err)
	}
	if v := storedVersion(ctx, t, fga); v != 0 {
		t.Fatalf("version = %d, want 0", v)
	}
}

func TestStoredVersionWithoutMarkers(t *testing.T) {
	ctx := testContext(t)
	fga, _ := fgatest.NewStore(ctx, t)
	writeMarked(ctx, t, fga, v1Model)
	if v := storedVersion(ctx, t, fga); v != 0 {
		t.Fatalf("version = %d, want 0", v)
	}
}
