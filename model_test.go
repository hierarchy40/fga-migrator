package fgamigrator

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestModelDigestIgnoresCommentsAndWhitespace(t *testing.T) {
	digest, err := mustMigrations(t, baseline()).ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("digest = %q, want sha256:<64 hex>", digest)
	}
	edited := "# a comment\n" + strings.ReplaceAll(v1Model, "\n\n", "\n\n\n") + "\n# trailing\n"
	if again, err := mustMigrations(t, Migration{Version: 1, Name: "baseline", Model: edited}).ModelDigest(); err != nil || again != digest {
		t.Fatalf("comment and whitespace edits changed the digest: %q then %q (%v)", digest, again, err)
	}
}

func TestModelDigestIsTheHighestVersions(t *testing.T) {
	one, err := mustMigrations(t, baseline()).ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	two, err := mustMigrations(t, baseline(), addWriter()).ModelDigest()
	if err != nil {
		t.Fatal(err)
	}
	want, err := modelDigest(mustEffective(t, v2Model))
	if err != nil {
		t.Fatal(err)
	}
	if two != want || two == one {
		t.Fatalf("digest of a v2 collection = %q, want %q (v1 digest %q)", two, want, one)
	}
}

func TestModelDigestMatchesAHandWrittenMarkerType(t *testing.T) {
	digest, err := mustMigrations(t, baseline()).ModelDigest()
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

func TestEffectiveModelRejectsADeclaredMarkerType(t *testing.T) {
	if _, err := effectiveModel(v1Model + handWrittenMarker); err == nil || !strings.Contains(err.Error(), "reserved schema_version type") {
		t.Fatalf("err = %v, want the reserved type rejection", err)
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
