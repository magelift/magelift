package paasimport

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Result is the output of a PaaS → MageLift map (D-05).
type Result struct {
	YAML     []byte
	Unmapped []UnmappedKey
}

// UnmappedKey is a residual PaaS key that could not be mapped.
type UnmappedKey struct {
	Path   string // dotted path, e.g. hooks.build or stage.deploy.FOO
	Source string // relative config file
}

// HasUnmapped reports whether residual keys remain.
func (r Result) HasUnmapped() bool {
	return len(r.Unmapped) > 0
}

// SidecarPath returns the durable unmapped report path adjacent to yamlPath.
// Default basename magelift.yaml → magelift.unmapped.md; other stems keep their basename.
func SidecarPath(yamlPath string) string {
	dir := filepath.Dir(yamlPath)
	base := filepath.Base(yamlPath)
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
	b.WriteString("# MageLift import; unmapped keys\n\n")
	b.WriteString("Import wrote a best-effort `magelift.yaml` but refused success because these PaaS keys could not be mapped (D-05).\n\n")
	if len(keys) == 0 {
		b.WriteString("_No unmapped keys._\n")
		return b.String()
	}
	b.WriteString("| Source | Key |\n| --- | --- |\n")
	for _, k := range keys {
		src := k.Source
		if src == "" {
			src = "?"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` |\n", src, k.Path)
	}
	b.WriteString("\n## What to do\n\n")
	b.WriteString("Work through each family, then delete this file and re-run `magelift config validate`. Details live in `docs/migrating-from-paas.md`.\n\n")
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
		return "hooks", "Free-form shell hooks have no portable equivalent. " +
			"Translate each hook body into a typed build hook: argument vectors only, " +
			"`composer` or `magento` executables only, no shell strings.\n\n" +
			"Before (ACC shape):\n\n```yaml\nhooks:\n  deploy: |\n    php bin/magento cache:flush\n```\n\n" +
			"After (MageLift shape; pick the step your command attaches to):\n\n" +
			"```yaml\nbuild:\n  hooks:\n    flush-cache:\n      phase: package\n      relationship: after\n      target: <step-id>\n      command:\n        executable: magento\n        arguments: [cache:flush]\n```\n\n" +
			"Drop stock `setup:upgrade` and `setup:static-content:deploy` lines: the pipeline already runs them."
	case "crons":
		return "crons", "MageLift runs Magento cron for you (ECS cron service plus `magelift cron-run`). " +
			"Keep custom schedules in Magento configuration and verify consumers under the target catalog; delete stock `cron:run` entries."
	case "relationships":
		return "relationships", "Replace each PaaS alias with a managed-service intent: database, cache, queue, and search live under the target catalog " +
			"(for example `target.aws.catalog.queueMode`), not as relationship names. Check `magelift config effective` shows the service you expect."
	case "runtime":
		return "runtime", "Carry the PHP version into `build.php` and needed extensions into `build.extensions` by hand. " +
			"The isolated builder validates both before it runs Magento lifecycle steps."
	case "dependencies":
		return "dependencies", "Nothing to do in `magelift.yaml`: `composer.json` stays the source of truth for PHP packages. " +
			"Only the `composer/composer` version maps (into `build.composer.version`); the rest travels with the repo."
	case "stage":
		return "stage", "The D-07 env allowlist is frozen: only crypt, static-content, URL, patch, admin, cookie, CORS, and consumer-runner keys map. " +
			"Move anything else into Magento configuration or a secret reference; see `docs/ece-parity.md` for the intentional gaps."
	default:
		return "other", "Review the key by hand against `docs/migrating-from-paas.md`. If it names a secret, store it as a secret reference, never plaintext in YAML."
	}
}
