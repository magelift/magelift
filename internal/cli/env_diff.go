package cli

import (
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/spf13/cobra"
)

type environmentChange struct {
	Field      string `json:"field" yaml:"field"`
	From       any    `json:"from" yaml:"from"`
	To         any    `json:"to" yaml:"to"`
	FromSource string `json:"fromSource,omitempty" yaml:"fromSource,omitempty"`
	ToSource   string `json:"toSource,omitempty" yaml:"toSource,omitempty"`
}

func envDiffCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "diff FROM TO", Short: "Compare two resolved environments and show where each difference came from", Args: cobra.ExactArgs(2), RunE: func(_ *cobra.Command, args []string) error {
		file, err := o.load()
		if err != nil {
			return invalid(err)
		}
		left, _, err := o.resolveEnvironment(file, args[0])
		if err != nil {
			return invalid(err)
		}
		right, _, err := o.resolveEnvironment(file, args[1])
		if err != nil {
			return invalid(err)
		}
		changes := []environmentChange{}
		var visit func(string, any, any)
		visit = func(path string, a, b any) {
			if reflect.DeepEqual(a, b) {
				return
			}
			am, aMap := a.(map[string]any)
			bm, bMap := b.(map[string]any)
			if (aMap || a == nil) && (bMap || b == nil) && (aMap || bMap) {
				keys := map[string]bool{}
				for k := range am {
					keys[k] = true
				}
				for k := range bm {
					keys[k] = true
				}
				for _, k := range slices.Sorted(maps.Keys(keys)) {
					visit(strings.TrimPrefix(path+"."+k, "."), am[k], bm[k])
				}
				return
			}
			changes = append(changes, environmentChange{path, a, b, sourceOf(left.Provenance, path), sourceOf(right.Provenance, path)})
		}
		visit("", left.SafeEffectiveOutput()["config"], right.SafeEffectiveOutput()["config"])
		return o.write(map[string]any{"from": args[0], "to": args[1], "changes": changes})
	}}
}

func sourceOf(p map[string]config.Provenance, path string) string {
	for path != "" {
		if v, ok := p[path]; ok {
			return v.Source
		}
		idx := strings.LastIndexByte(path, '.')
		if idx < 0 {
			break
		}
		path = path[:idx]
	}
	return ""
}
