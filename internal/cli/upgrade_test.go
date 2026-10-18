package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	mageliftupgrade "github.com/magelift/magelift/internal/upgrade"
)

type fakeUpgradeClient struct {
	release   upgradeRelease
	installed bool
}

func (f *fakeUpgradeClient) Latest(context.Context) (upgradeRelease, error) {
	return f.release, nil
}

func (f *fakeUpgradeClient) Tagged(context.Context, string) (upgradeRelease, error) {
	return f.release, nil
}

func (f *fakeUpgradeClient) Install(context.Context, upgradeRelease, string) error {
	f.installed = true
	return nil
}

func TestUpgradeCheckIsReadOnly(t *testing.T) {
	oldVersion := Version
	Version = "1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	var output bytes.Buffer
	fake := &fakeUpgradeClient{release: mageliftupgrade.Release{TagName: "v1.1.0"}}
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.newUpgrade = func() upgradeClient { return fake }
	o.configPath = "unused"
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--output", "json", "upgrade", "--check"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.installed {
		t.Fatal("check installed an update")
	}
	if !bytes.Contains(output.Bytes(), []byte(`"updateAvailable": true`)) {
		t.Fatalf("output = %s", output.String())
	}
}

func TestUpgradePropagatesInstallFailure(t *testing.T) {
	oldVersion := Version
	Version = "1.0.0"
	t.Cleanup(func() { Version = oldVersion })
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.newUpgrade = func() upgradeClient { return failingUpgradeClient{} }
	o.executable = func() (string, error) { return "/tmp/magelift", nil }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"upgrade"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 3 || !errors.Is(err, errUpgradeInstall) {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
}

var errUpgradeInstall = errors.New("install failed")

type failingUpgradeClient struct{}

func (failingUpgradeClient) Latest(context.Context) (upgradeRelease, error) {
	return upgradeRelease{TagName: "v1.1.0"}, nil
}

func (failingUpgradeClient) Tagged(context.Context, string) (upgradeRelease, error) {
	return upgradeRelease{TagName: "v1.1.0"}, nil
}

func (failingUpgradeClient) Install(context.Context, upgradeRelease, string) error {
	return errUpgradeInstall
}
