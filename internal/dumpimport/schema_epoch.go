package dumpimport

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	patchListCountSQL   = "SELECT COUNT(*) FROM patch_list"
	setupModuleCountSQL = "SELECT COUNT(*) FROM setup_module"
)

// SchemaEpoch reads Magento's applied-patch and setup_module counts and
// returns a monotonic epoch for rollback compatibility checks. Quality
// patches change patch_list; Magento upgrades change both tables. Either
// count below 1 after a successful query is treated as unobserved.
func SchemaEpoch(ctx context.Context, opts Options) (int, error) {
	if ctx == nil {
		return 0, errors.New("dumpimport: schema epoch context is required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	patches, patchErr := queryCount(ctx, opts, patchListCountSQL)
	modules, moduleErr := queryCount(ctx, opts, setupModuleCountSQL)
	epoch := CombineSchemaEpoch(patches, patchErr == nil, modules, moduleErr == nil)
	if epoch < 1 {
		if moduleErr != nil {
			return 0, fmt.Errorf("observe Magento schema epoch: %w", moduleErr)
		}
		if patchErr != nil {
			return 0, fmt.Errorf("observe Magento schema epoch: %w", patchErr)
		}
		return 0, errors.New("Magento schema epoch was not recorded")
	}
	return epoch, nil
}

// CombineSchemaEpoch turns Magento table counts into a recorded epoch.
// Missing patch_list (older Magento) still works from setup_module alone.
func CombineSchemaEpoch(patches int, patchesOK bool, modules int, modulesOK bool) int {
	if !patchesOK {
		patches = 0
	}
	if !modulesOK {
		modules = 0
	}
	if patches < 0 {
		patches = 0
	}
	if modules < 0 {
		modules = 0
	}
	return patches + modules
}

func queryCount(ctx context.Context, opts Options, sql string) (int, error) {
	raw, err := Query(ctx, opts, sql)
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("dumpimport: schema epoch count %q is not an integer", raw)
	}
	return count, nil
}
