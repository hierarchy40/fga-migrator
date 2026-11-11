package fgamigrator

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestNewSetOrdersByVersion(t *testing.T) {
	s, err := NewSet(3,
		Migration{Version: 3, Name: "drop_editor", Model: v3Model},
		baseline(),
		Migration{Version: 2, Name: "add_writer", Model: v2Model},
	)
	if err != nil {
		t.Fatal(err)
	}
	ms := s.Migrations()
	for i, m := range ms {
		if m.Version != i+1 {
			t.Fatalf("migration %d has version %d", i, m.Version)
		}
	}
	if s.Required() != 3 || s.Model() != v3Model {
		t.Fatalf("required = %d, model is v3 = %v", s.Required(), s.Model() == v3Model)
	}
	ms[0].Name = "mutated"
	if s.Migrations()[0].Name != "baseline" {
		t.Fatal("Migrations exposes the set's backing slice")
	}
}

func TestNewSetRejectsInvalidSets(t *testing.T) {
	cases := map[string]struct {
		ms       []Migration
		required int
		want     string
	}{
		"empty":           {nil, 0, "no migrations"},
		"gap":             {[]Migration{baseline(), {Version: 3, Name: "c", Model: v3Model}}, 3, "contiguous"},
		"not from one":    {[]Migration{{Version: 2, Name: "b", Model: v2Model}}, 2, "contiguous"},
		"duplicate":       {[]Migration{baseline(), {Version: 1, Name: "again", Model: v1Model}}, 2, "contiguous"},
		"behind":          {[]Migration{baseline()}, 2, "want required schema version 2"},
		"ahead":           {[]Migration{baseline(), {Version: 2, Name: "b", Model: v2Model}}, 1, "want required schema version 1"},
		"no name":         {[]Migration{{Version: 1, Model: v1Model}}, 1, "no name"},
		"no model":        {[]Migration{{Version: 1, Name: "baseline"}}, 1, "no model"},
		"later no model":  {[]Migration{baseline(), {Version: 2, Name: "data_only"}}, 2, "no model"},
		"bad dsl":         {[]Migration{{Version: 1, Name: "baseline", Model: "model\n  schema 1.1\ntype"}}, 1, "model"},
		"declares marker": {[]Migration{{Version: 1, Name: "baseline", Model: v1Model + handWrittenMarker}}, 1, "reserved schema_version type"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewSet(tc.required, tc.ms...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestMustSetPanicsOnInvalidSet(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustSet accepted an empty set")
		}
	}()
	MustSet(1)
}

func TestPendingSelectsVersionsAfterStored(t *testing.T) {
	s := mustSet(t, baseline(), Migration{Version: 2, Name: "add_writer", Model: v2Model})
	if got := s.pending(0); len(got) != 2 {
		t.Fatalf("pending(0) = %d migrations, want 2", len(got))
	}
	if got := s.pending(1); len(got) != 1 || got[0].Version != 2 {
		t.Fatalf("pending(1) = %v, want version 2", got)
	}
	if got := s.pending(2); len(got) != 0 {
		t.Fatalf("pending(2) = %v, want none", got)
	}
}

func TestModelDigestIgnoresCommentsAndWhitespace(t *testing.T) {
	s := mustSet(t, baseline())
	digest, err := s.ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("digest = %q, want sha256:<64 hex>", digest)
	}
	edited := "# a comment\n" + strings.ReplaceAll(v1Model, "\n\n", "\n\n\n") + "\n# trailing\n"
	if again, err := mustSet(t, Migration{Version: 1, Name: "baseline", Model: edited}).ModelDigest(); err != nil || again != digest {
		t.Fatalf("comment and whitespace edits changed the digest: %q then %q (%v)", digest, again, err)
	}
}

func TestModelDigestIsTheHighestVersions(t *testing.T) {
	one, err := mustSet(t, baseline()).ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	two, err := mustSet(t, baseline(), Migration{Version: 2, Name: "add_writer", Model: v2Model}).ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	want, err := modelDigest(mustEffective(t, v2Model))
	if err != nil {
		t.Fatal(err)
	}
	if two != want || two == one {
		t.Fatalf("digest of a v2 set = %q, want %q (v1 digest %q)", two, want, one)
	}
}

func TestMarkerRoundTrip(t *testing.T) {
	n, err := parseMarker(markerUser(7))
	if err != nil || n != 7 {
		t.Fatalf("parseMarker(markerUser(7)) = %d, %v", n, err)
	}
	for _, bad := range []string{"user:1", "schema_version:x", ""} {
		if _, err := parseMarker(bad); err == nil {
			t.Fatalf("parseMarker(%q) accepted", bad)
		}
	}
}

func TestModelDigestMatchesAHandWrittenMarkerType(t *testing.T) {
	digest, err := mustSet(t, baseline()).ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	handWritten, err := normaliseDSL(v1Model + handWrittenMarker)
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(handWritten))); digest != want {
		t.Fatalf("digest = %q, want %q", digest, want)
	}
}

func TestEffectiveModelAddsTheMarkerType(t *testing.T) {
	model := mustEffective(t, v1Model)
	last := model.TypeDefinitions[len(model.TypeDefinitions)-1]
	if _, ok := last.GetRelations()[markerRelation]; last.Type != markerType || !ok {
		t.Fatalf("last type = %q with relations %v, want the marker type", last.Type, last.GetRelations())
	}
	unmarked, err := dslToModel(v1Model)
	if err != nil {
		t.Fatal(err)
	}
	if same, err := sameModel(model, mustEffective(t, v1Model)); err != nil || !same {
		t.Fatalf("sameModel(v1, v1) = %v, %v", same, err)
	}
	if same, err := sameModel(unmarked, model); err != nil || same {
		t.Fatalf("sameModel(unmarked v1, v1) = %v, %v", same, err)
	}
}
