package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/platform"
)

type fakePlatformState struct {
	info     platform.LockInfo
	err      error
	called   bool
	unlock   bool
	locked   bool
	backup   platform.BackupResult
	restore  platform.RestoreResult
	backups  int
	restores []string
	backend  string
}

func (m *fakePlatformState) Status(context.Context, platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	m.called = true
	if m.err != nil {
		return false, nil, m.backend, m.err
	}
	if m.info.Owner == "" {
		return false, nil, m.backend, nil
	}
	info := m.info
	return true, &info, m.backend, nil
}

func (m *fakePlatformState) Lock(context.Context, platform.PlannedStack, string) (func(context.Context) error, error) {
	m.locked = true
	return func(context.Context) error { return nil }, nil
}

func (m *fakePlatformState) Unlock(context.Context, platform.PlannedStack) (*platform.LockInfo, error) {
	m.unlock = true
	if m.err != nil {
		return nil, m.err
	}
	info := m.info
	return &info, nil
}

func (m *fakePlatformState) Backup(context.Context, platform.PlannedStack) (platform.BackupResult, error) {
	m.backups++
	return m.backup, nil
}

func (m *fakePlatformState) Restore(_ context.Context, _ platform.PlannedStack, id string) (platform.RestoreResult, error) {
	m.restores = append(m.restores, id)
	return m.restore, nil
}

func TestStateStatusReportsUnlockedEnvironment(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	manager := &fakePlatformState{backend: "s3://state"}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.testState = manager
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "state", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !manager.called || !strings.Contains(out.String(), `"locked": false`) {
		t.Fatalf("state status = %s", out.String())
	}
}

func TestStateStatusPropagatesInspectionFailure(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	manager := &fakePlatformState{err: errors.New("backend unavailable")}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.testState = manager
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "state", "status"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "inspect deployment lock") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStateUnlockRequiresConfirmation(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	manager := &fakePlatformState{info: platform.LockInfo{Project: "shop", Environment: "staging", Owner: "ci"}}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.testState = manager
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "state", "unlock"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("unexpected error/code: %v/%d", err, ExitCode(err))
	}
	if manager.unlock {
		t.Fatal("unlock was called without confirmation")
	}

	out.Reset()
	cmd = newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--yes", "--output", "json", "state", "unlock"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !manager.unlock || !strings.Contains(out.String(), `"environment": "staging"`) {
		t.Fatalf("unexpected unlock output: called=%v output=%s", manager.unlock, out.String())
	}
}

func TestStateBackupAndRestoreUseADeploymentLock(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	manager := &fakePlatformState{
		backup:  platform.BackupResult{ID: "backup-1", Location: "backups/backup-1"},
		restore: platform.RestoreResult{ID: "backup-1", Location: "backups/backup-1"},
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.testState = manager

	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "state", "backup"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !manager.locked || manager.backups != 1 || !strings.Contains(out.String(), `"id": "backup-1"`) {
		t.Fatalf("backup calls: locked=%v backups=%d output=%s", manager.locked, manager.backups, out.String())
	}

	out.Reset()
	cmd = newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--yes", "--output", "json", "state", "restore", "backup-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(manager.restores) != 1 || manager.restores[0] != "backup-1" || !strings.Contains(out.String(), `"id": "backup-1"`) {
		t.Fatalf("restore calls: %+v output=%s", manager.restores, out.String())
	}
}
