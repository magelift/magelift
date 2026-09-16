package recovery

import (
	"testing"
)

func TestObjectManifestSealAndOpenRejectsTampering(t *testing.T) {
	manifest := ObjectArchiveManifest{
		Version: 1, DataClass: "media", FixtureID: "fixture", OwnershipMarker: "owner",
		ArchiveBucket: "archive", ArchivePrefix: "recovery/operation",
		Entries: []ObjectArchiveEntry{{SourceKey: "media/a", ArchiveKey: "recovery/operation/objects/a", Size: 3, ETag: "etag"}},
	}
	body, digest, err := SealObjectManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	opened, openedDigest, err := OpenObjectManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if openedDigest != digest || opened.Digest != digest || opened.Entries[0].ArchiveKey != manifest.Entries[0].ArchiveKey {
		t.Fatalf("opened manifest = %#v, digest=%q want %q", opened, openedDigest, digest)
	}
	body[len(body)-2] ^= 1
	if _, _, err := OpenObjectManifest(body); err == nil {
		t.Fatal("tampered object manifest was accepted")
	}
}

func TestSafeArchiveKeyRejectsTraversalAndPreservesNestedKeys(t *testing.T) {
	if got := SafeArchiveKey("archive/objects", "nested/file.json"); got != "archive/objects/nested/file.json" {
		t.Fatalf("safe key = %q", got)
	}
	if got := SafeArchiveKey("archive/objects", "../escape"); got != "" {
		t.Fatalf("traversal key = %q", got)
	}
}
