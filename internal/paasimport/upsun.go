package paasimport

import (
	"fmt"
	"os"
	"path/filepath"
)

// MapUpsun maps an Upsun / Platform.sh config root into MageLift YAML.
// Thin stub until 04-03: emits a Load-valid schemaVersion-1 document so the
// init --from-upsun CLI surface is callable offline without full mapping.
func MapUpsun(root string) ([]byte, error) {
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("Upsun config root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Upsun config root must be a directory: %s", root)
	}
	return emitMagelift(baseDocument("upsun-import", "8.3"))
}
