package paasimport

import (
	"fmt"
	"path/filepath"
	"strings"
)

// UnmappedKey is a residual PaaS key that could not be mapped.
type UnmappedKey struct {
	Path   string // dotted path, e.g. hooks.build or stage.deploy.FOO
	Source string // relative config file
}

// SidecarPath returns the durable unmapped report path adjacent to projectPath.
// Default basename magelift.php → magelift.unmapped.md; other stems keep their basename.
func SidecarPath(projectPath string) string {
	dir := filepath.Dir(projectPath)
	base := filepath.Base(projectPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = "magelift"
	}
	return filepath.Join(dir, stem+".unmapped.md")
}

// RenderUnmappedReport formats residual keys as Markdown for operators/CI.
// The report carries paths and guidance only, never PaaS values: UnmappedKey
// has no value field, so secret bytes cannot leak through this path.
func RenderUnmappedReport(keys []UnmappedKey) string {
	var b strings.Builder
	b.WriteString("# Settings to review after import\n\n")
	if len(keys) == 0 {
		b.WriteString("_No unmapped keys._\n")
		return b.String()
	}
	b.WriteString("MageLift wrote the project file and exited with status 2. Review these settings before deploying.\n\n")
	b.WriteString("| Source | Key |\n| --- | --- |\n")
	for _, k := range keys {
		src := k.Source
		if src == "" {
			src = "?"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` |\n", src, k.Path)
	}
	b.WriteString("\n## What to do\n\n")
	b.WriteString("Review each setting, then run `magelift config validate` on the imported project. [Migration guide](https://github.com/magelift/magelift/blob/main/docs/migrating-from-paas.md)\n\n")
	seen := map[string]bool{}
	for _, k := range keys {
		family, action := guidanceForFamily(k.Path)
		if seen[family] {
			continue
		}
		seen[family] = true
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", family, action)
	}
	return b.String()
}

// guidanceForFamily maps an unmapped path to its checklist family and action.
// Actions name the MageLift equivalent; the hooks family shows the generic
// before/after shape rather than echoing the user's hook body.
func guidanceForFamily(path string) (family, action string) {
	first, _, _ := strings.Cut(path, ".")
	switch first {
	case "hooks":
		return "hooks", "Review each shell hook and decide when it belongs in the Magento lifecycle. MageLift runs schema upgrades and static content deployment. For an additional build command, add an entry to `build.hooks` in the PHP project. Replace `STEP_ID` with the step your hook follows.\n\n" +
			"```php\n'flush-cache' => [\n    'phase' => 'package',\n    'relationship' => 'after',\n    'target' => 'STEP_ID',\n    'command' => ['executable' => 'magento', 'arguments' => ['cache:flush']],\n],\n```"
	case "crons":
		return "crons", "MageLift schedules Magento cron on the selected runtime. Keep custom schedules in Magento configuration and check their logs after deploying."
	case "relationships":
		return "relationships", "Match each source alias to a database, cache, queue or search entry under `services`. Run `magelift config effective` to check the family and version for this environment."
	case "runtime":
		return "runtime", "Carry the PHP version into `build.php` and needed extensions into `build.extensions` by hand. " +
			"The isolated builder validates both before it runs Magento lifecycle steps."
	case "dependencies":
		return "dependencies", "Keep application package requirements in `composer.json` and commit the lock. The importer copies the Composer tool version into `build.composer.version` when it recognizes the setting."
	case "stage":
		return "stage", "Move unrecognized Magento environment settings into your application configuration or store them in the destination secret manager. Match encryption, URL and consumer settings to the source shop before importing data."
	default:
		return "other", "Review the key by hand against `docs/migrating-from-paas.md`. If it names a secret, store it as a secret reference, keep raw values out of the project file."
	}
}
