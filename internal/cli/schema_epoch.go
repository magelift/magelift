package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/magelift/magelift/internal/dumpimport"
)

func (o *options) observeLiveSchemaEpoch(ctx context.Context) (int, error) {
	opts := dumpimport.Options{WorkDir: filepath.Dir(filepath.Clean(o.configPath))}
	o.applyDumpimportTransportEnv(&opts)
	if !schemaEpochTransportConfigured(opts) {
		return 0, errors.New("Magento schema epoch transport is not configured")
	}
	return dumpimport.SchemaEpoch(ctx, opts)
}

func schemaEpochTransportConfigured(opts dumpimport.Options) bool {
	if strings.EqualFold(strings.TrimSpace(opts.Runner), dumpimport.RunnerKube) {
		return strings.TrimSpace(opts.Namespace) != "" && (strings.TrimSpace(opts.Pod) != "" || strings.TrimSpace(opts.PodSelector) != "")
	}
	return strings.TrimSpace(opts.Host) != ""
}
