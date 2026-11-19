package sdk

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
)

var sqlSettingName = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)

func ValidateDatabaseParameters(parameters map[string]string) error {
	for _, key := range slices.Sorted(maps.Keys(parameters)) {
		value := parameters[key]
		switch key {
		case "character_set_server", "collation_server":
			if sqlSettingName.MatchString(value) {
				continue
			}
		case "max_allowed_packet":
			size, err := strconv.ParseUint(value, 10, 32)
			if err == nil && size >= 1024 && size <= 1<<30 {
				continue
			}
		}
		return fmt.Errorf("unsupported database parameter or value for %q", key)
	}
	return nil
}

// DatabaseArguments passes shop settings to the upstream container entrypoint.
func DatabaseArguments(parameters map[string]string) []string {
	args := []string{"--log-bin-trust-function-creators=1"}
	for _, name := range slices.Sorted(maps.Keys(parameters)) {
		args = append(args, "--"+name+"="+parameters[name])
	}
	return args
}
