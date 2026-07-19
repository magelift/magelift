package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	awsstate "github.com/acourtiol/magelift/internal/cloud/aws/state"
)

type fakeStateManager struct {
	info   awsstate.Info
	err    error
	called bool
	unlock bool
	locked bool
}

type fakeStateArchive struct {
	backup   awsstate.BackupResult
	restore  awsstate.RestoreResult
	backups  int
	restores []string
}

func (a *fakeStateArchive) Backup(context.Context) (awsstate.BackupResult, error) {
	a.backups++
	return a.backup, nil
}

func (a *fakeStateArchive) Restore(_ context.Context, id string) (awsstate.RestoreResult, error) {
	a.restores = append(a.restores, id)
	return a.restore, nil
}

func (m *fakeStateManager) Status(context.Context) (awsstate.Info, error) {
	m.called = true
	return m.info, m.err
}

func (m *fakeStateManager) Unlock(context.Context) (awsstate.Info, error) {
	m.unlock = true
	return m.info, m.err
}

func (m *fakeStateManager) Lock(context.Context, string, string, string) (func() error, error) {
	m.locked = true
	return func() error { return nil }, nil
}

func TestStateStatusReportsUnlockedEnvironment(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	manager := &fakeStateManager{err: awsstate.ErrNotLocked}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newState = func(context.Context, string, string, string, string, string) (stateManager, error) {
		return manager, nil
	}
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
	manager := &fakeStateManager{err: errors.New("backend unavailable")}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newState = func(context.Context, string, string, string, string, string) (stateManager, error) {
		return manager, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "state", "status"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "inspect deployment lock") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStateUnlockRequiresConfirmation(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	manager := &fakeStateManager{info: awsstate.Info{Project: "shop", Environment: "staging", Owner: "ci"}}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newState = func(context.Context, string, string, string, string, string) (stateManager, error) {
		return manager, nil
	}
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
	manager := &fakeStateManager{}
	archive := &fakeStateArchive{backup: awsstate.BackupResult{ID: "backup-1", Objects: 2}, restore: awsstate.RestoreResult{ID: "backup-1", Objects: 2}}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newState = func(context.Context, string, string, string, string, string) (stateManager, error) {
		return manager, nil
	}
	o.newArchive = func(context.Context, string, string, string) (stateArchive, error) { return archive, nil }

	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "state", "backup"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !manager.locked || archive.backups != 1 || !strings.Contains(out.String(), `"id": "backup-1"`) {
		t.Fatalf("backup calls: locked=%v backups=%d output=%s", manager.locked, archive.backups, out.String())
	}

	out.Reset()
	cmd = newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--yes", "--output", "json", "state", "restore", "backup-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(archive.restores) != 1 || archive.restores[0] != "backup-1" || !strings.Contains(out.String(), `"id": "backup-1"`) {
		t.Fatalf("restore calls: %+v output=%s", archive.restores, out.String())
	}
}
