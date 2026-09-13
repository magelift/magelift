package config

import "testing"

func TestCanonicalAuroraMySQLVersion(t *testing.T) {
	if got := CanonicalAuroraMySQLVersion("8.0.mysql_aurora.3.12"); got != "8.0.mysql_aurora.3.12.0" {
		t.Fatalf("truncated 3.12 pin = %q", got)
	}
	if got := CanonicalAuroraMySQLVersion("8.0.mysql_aurora.3.12.0"); got != "8.0.mysql_aurora.3.12.0" {
		t.Fatalf("full 3.12.0 pin = %q", got)
	}
	if got := CanonicalAuroraMySQLVersion("8.4.mysql_aurora.8.4.7"); got != "8.4.mysql_aurora.8.4.7" {
		t.Fatalf("8.4 pin must stay verbatim = %q", got)
	}
}
