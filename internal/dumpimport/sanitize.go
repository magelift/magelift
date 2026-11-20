package dumpimport

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// SanitizedLabel describes the only transformation applied to exported data.
const SanitizedLabel = "Magento dump with hashed email addresses and other personal data"

// mailboxPattern matches mailbox addresses inside SQL string values. It
// requires a dotted domain so DEFINER=`user`@`localhost` is left alone.
var mailboxPattern = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// SanitizeSQL hashes email addresses and preserves other personal data.
func SanitizeSQL(sql []byte) []byte {
	return mailboxPattern.ReplaceAllFunc(sql, func(match []byte) []byte {
		sum := sha256.Sum256(match)
		return []byte(hex.EncodeToString(sum[:8]) + "@sanitized.invalid")
	})
}

func sanitizeOutputFile(path string) error {
	gzipped := strings.HasSuffix(strings.ToLower(path), ".gz")
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("dumpimport: open dump for sanitize: %w", err)
	}
	var reader io.Reader = src
	if gzipped {
		gz, err := gzip.NewReader(src)
		if err != nil {
			_ = src.Close()
			return fmt.Errorf("dumpimport: gunzip dump for sanitize: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	body, err := io.ReadAll(reader)
	closeErr := src.Close()
	if err != nil {
		return fmt.Errorf("dumpimport: read dump for sanitize: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("dumpimport: close dump for sanitize: %w", closeErr)
	}
	sanitized := SanitizeSQL(body)
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("dumpimport: rewrite sanitized dump: %w", err)
	}
	defer out.Close()
	var writer io.Writer = out
	if gzipped {
		gz := gzip.NewWriter(out)
		defer gz.Close()
		writer = gz
	}
	if _, err := writer.Write(sanitized); err != nil {
		return fmt.Errorf("dumpimport: write sanitized dump: %w", err)
	}
	return nil
}
