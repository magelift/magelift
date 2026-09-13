package kube

import "testing"

func TestRedactMigrationLogs(t *testing.T) {
	got := redactMigrationLogs(`password="dont-log" token=also-private user=magento`)
	if got != `password=[redacted] token=[redacted] user=magento` {
		t.Fatalf("redacted logs = %q", got)
	}
}
