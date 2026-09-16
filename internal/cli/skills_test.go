package cli

import (
	"bytes"
	"testing"
)

func TestSkillsInstallAndVerifyRegisterAgentScopeSkillFlags(t *testing.T) {
	out := &bytes.Buffer{}
	root := newCommandWithOptions(testOptions(out, &fakeTerminal{interactive: false}))
	for _, path := range [][]string{{"skills", "install"}, {"skills", "verify"}} {
		found, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("find %v: %v", path, err)
		}
		for _, name := range []string{"agent", "scope", "skill"} {
			if found.Flags().Lookup(name) == nil {
				t.Errorf("magelift %s %s is missing --%s", path[0], path[1], name)
			}
		}
	}
}
