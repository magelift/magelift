package dumpimport

import "testing"

func TestCombineSchemaEpoch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		patches   int
		patchesOK bool
		modules   int
		modulesOK bool
		want      int
	}{
		{name: "both", patches: 12, patchesOK: true, modules: 80, modulesOK: true, want: 92},
		{name: "no-patch-table", patches: 0, patchesOK: false, modules: 80, modulesOK: true, want: 80},
		{name: "empty", patches: 0, patchesOK: true, modules: 0, modulesOK: true, want: 0},
		{name: "negative-ignored", patches: -1, patchesOK: true, modules: 4, modulesOK: true, want: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := CombineSchemaEpoch(test.patches, test.patchesOK, test.modules, test.modulesOK)
			if got != test.want {
				t.Fatalf("got %d want %d", got, test.want)
			}
		})
	}
}
