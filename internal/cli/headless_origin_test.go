package cli

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestInitRejectsWildcardStorefrontOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.php")
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "init", "--provider", "aws", "--storefront-origin", "https://*.example.test"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("wildcard origin was accepted")
	}
}
