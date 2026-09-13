package recovery

import "testing"

func TestSecretArchiveSealAndOpenRejectsTampering(t *testing.T) {
	archive := SecretArchive{Version: 1, DataClass: "configuration-secrets", FixtureID: "fixture", OwnershipMarker: "owner", SourceSecret: "projects/demo/secrets/app", Encoding: "binary", Value: []byte("secret")}
	body, digest, err := SealSecretArchive(archive)
	if err != nil {
		t.Fatal(err)
	}
	opened, openedDigest, err := OpenSecretArchive(body)
	if err != nil {
		t.Fatal(err)
	}
	if openedDigest != digest || opened.Digest != digest || string(opened.Value) != "secret" {
		t.Fatalf("opened archive = %#v, digest=%q want %q", opened, openedDigest, digest)
	}
	body[len(body)-2] ^= 1
	if _, _, err := OpenSecretArchive(body); err == nil {
		t.Fatal("tampered secret archive was accepted")
	}
}
