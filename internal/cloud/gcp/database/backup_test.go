package database

import "testing"

func TestFinalBackupRetentionDays(t *testing.T) {
	tests := []struct {
		name                    string
		backupEnabled           bool
		transactionLogRetention int
		backupRetentionCount    int
		want                    int
	}{
		{name: "backups disabled", backupEnabled: false, transactionLogRetention: 7, backupRetentionCount: 8, want: 0},
		{name: "uses larger of log days and backup count", backupEnabled: true, transactionLogRetention: 7, backupRetentionCount: 14, want: 14},
		{name: "uses log days when count is smaller", backupEnabled: true, transactionLogRetention: 7, backupRetentionCount: 3, want: 7},
		{name: "defaults to seven when policy is unset", backupEnabled: true, want: 7},
		{name: "caps at 365", backupEnabled: true, backupRetentionCount: 400, want: 365},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := finalBackupRetentionDays(test.backupEnabled, test.transactionLogRetention, test.backupRetentionCount); got != test.want {
				t.Fatalf("finalBackupRetentionDays() = %d, want %d", got, test.want)
			}
		})
	}
}
