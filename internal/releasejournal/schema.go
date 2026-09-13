package releasejournal

import (
	"errors"
	"fmt"
)

var (
	// ErrSchemaUnavailable means rollback cannot prove the digest runs on the live schema.
	ErrSchemaUnavailable = errors.New("live schema compatibility is unavailable")
	// ErrSchemaMismatch means the target digest required an older schema than the live database.
	ErrSchemaMismatch = errors.New("rollback digest cannot run on the live schema")
)

const schemaRemediation = "restore the previous database or forward-fix the current schema"

// RefuseIncompatibleRollback fails closed before cutover when the target digest
// cannot run on the live schema, or when either epoch is missing.
func RefuseIncompatibleRollback(liveEpoch, targetEpoch int) error {
	if liveEpoch < 1 || targetEpoch < 1 {
		return fmt.Errorf("%w: Magento schema epoch was not recorded; %s", ErrSchemaUnavailable, schemaRemediation)
	}
	if targetEpoch < liveEpoch {
		return fmt.Errorf("%w: live schema epoch %d is newer than digest schema epoch %d; rollback would cut over before Magento can run; %s", ErrSchemaMismatch, liveEpoch, targetEpoch, schemaRemediation)
	}
	return nil
}

// SignatureMetadataForDigest returns the newest Cosign identity and issuer
// recorded for digest. Deploy rows after a promote often omit those fields;
// rollback still needs them to verify the same digest.
func SignatureMetadataForDigest(entries []Entry, digest string) (identity, issuer string) {
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.DigestReference == digest && entry.SignatureIdentity != "" && entry.SignatureIssuer != "" {
			return entry.SignatureIdentity, entry.SignatureIssuer
		}
	}
	return "", ""
}

// SchemaEpochForDigest returns the newest recorded Magento schema epoch for
// digest. Unsigned deploy rows after a promote often omit it; rollback still
// needs the promote-time epoch for RefuseIncompatibleRollback.
func SchemaEpochForDigest(entries []Entry, digest string) int {
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.DigestReference == digest && entry.SchemaEpoch >= 1 {
			return entry.SchemaEpoch
		}
	}
	return 0
}
