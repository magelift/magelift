// Package naming keeps AWS resource Name attributes within service limits.
package naming

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// AWSName truncates value to limit characters while keeping a short content hash
// so distinct long inputs do not collide after truncation.
func AWSName(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit < 8 {
		limit = 8
	}
	if len(value) <= limit {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	suffix := hex.EncodeToString(sum[:3])
	keep := limit - 1 - len(suffix)
	if keep < 1 {
		return suffix[:limit]
	}
	prefix := strings.TrimRight(value[:keep], "-")
	if prefix == "" {
		return suffix[:limit]
	}
	return prefix + "-" + suffix
}
