package providerhost

import (
	"context"
	"errors"
	"os"
	"testing"
)

// fakePluginVersionEnv selects the helper-plugin path in TestMain. The value
// is the SDK API version the helper reports from Ping.
const fakePluginVersionEnv = "MAGELIFT_TEST_FAKE_PLUGIN_VERSION"

func TestMain(m *testing.M) {
	if version := os.Getenv(fakePluginVersionEnv); version != "" {
		// Helper-plugin mode: serve one wrong-version provider until the
		// parent kills us. This path never runs the suite.
		Serve(wrongVersionAPI{API: &fakeExecuteAPI{}, version: version})
		return
	}
	os.Exit(m.Run())
}

// wrongVersionAPI reuses the fake executor with a Ping override so Dial
// dispenses a live plugin that reports a drifted SDK API version.
type wrongVersionAPI struct {
	API
	version string
}

func (f wrongVersionAPI) Ping(context.Context) (string, error) {
	return f.version, nil
}

func TestDialRefusesVersionMismatch(t *testing.T) {
	t.Setenv(fakePluginVersionEnv, "v0-test")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	session, err := Dial(context.Background(), executable)
	if !errors.Is(err, ErrUnsupportedAPI) {
		t.Fatalf("err = %v, want ErrUnsupportedAPI", err)
	}
	if session != nil {
		session.Close()
		t.Fatal("Dial returned a session for a mismatched plugin")
	}
}
