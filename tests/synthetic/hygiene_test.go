//go:build synthetic

package synthetic_test

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	fqdnPattern   = regexp.MustCompile(`\b(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,}\b`)
	ipv4Pattern   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	twelveDigits  = regexp.MustCompile(`\b\d{12}\b`)
	secretAssign  = regexp.MustCompile(`(?i)\b(password|passwd|secret|token|api[_-]?key|private[_-]?key)\b\s*[:=]\s*(\S+)`)
	exampleSuffix = []string{".example.test", ".example.invalid"}
	exampleBare   = []string{"example.test", "example.invalid"}
)

const exampleAccountID = "123456789012"

func isExampleIdentity(token string) bool {
	lower := strings.ToLower(token)
	for _, bare := range exampleBare {
		if lower == bare {
			return true
		}
	}
	for _, suffix := range exampleSuffix {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func isPlaceholderValue(value string) bool {
	trimmed := strings.Trim(value, "\"'")
	lower := strings.ToLower(trimmed)
	if trimmed == "" {
		return true
	}
	for _, marker := range []string{"example.test", "example.invalid", "example-test", "example-invalid", exampleAccountID, "changeme", "placeholder"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	digits := strings.ReplaceAll(strings.ReplaceAll(trimmed, "-", ""), ":", "")
	if digits != "" && strings.Trim(digits, "0") == "" {
		return true
	}
	return false
}

// scanHygiene returns one violation per offending line. It scans content only;
// callers decide which tree to walk.
func scanHygiene(path string, content []byte) []string {
	var violations []string
	for i, line := range strings.Split(string(content), "\n") {
		for _, token := range fqdnPattern.FindAllString(line, -1) {
			if !isExampleIdentity(token) {
				violations = append(violations, fmt.Sprintf("%s:%d: non-example identity %q", path, i+1, token))
			}
		}
		for _, token := range ipv4Pattern.FindAllString(line, -1) {
			addr, err := netip.ParseAddr(token)
			if err != nil || !addr.Is4() || !addr.IsPrivate() {
				violations = append(violations, fmt.Sprintf("%s:%d: non-private IPv4 %q", path, i+1, token))
			}
		}
		for _, token := range twelveDigits.FindAllString(line, -1) {
			if token != exampleAccountID && strings.Trim(token, "0") != "" {
				violations = append(violations, fmt.Sprintf("%s:%d: non-example account ID %q", path, i+1, token))
			}
		}
		for _, match := range secretAssign.FindAllStringSubmatch(line, -1) {
			if len(match) == 3 && !isPlaceholderValue(match[2]) {
				violations = append(violations, fmt.Sprintf("%s:%d: secret-like assignment %q", path, i+1, match[0]))
			}
		}
	}
	return violations
}

func TestHygieneSelfTest(t *testing.T) {
	t.Parallel()
	good := "domain: shop.example.test\naccount: \"123456789012\"\n# example-only\n"
	if violations := scanHygiene("good", []byte(good)); len(violations) != 0 {
		t.Fatalf("good sample violations = %v", violations)
	}
	bad := map[string]string{
		"fqdn":    "domain: shop.evilcorp.com\n",
		"email":   "admin: ops@evilcorp.com\n",
		"ipv4":    "host: 8.8.8.8\n",
		"account": "account: \"999999999999\"\n",
		"secret":  "password: hunter2-hunter2\n",
	}
	for name, sample := range bad {
		if violations := scanHygiene(name, []byte(sample)); len(violations) == 0 {
			t.Errorf("bad sample %q passed hygiene", name)
		}
	}
}

func TestHygieneFixtures(t *testing.T) {
	root := "../fixtures/synthetic"
	var violations []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		violations = append(violations, scanHygiene(path, content)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("fixture hygiene violations:\n%s", strings.Join(violations, "\n"))
	}
}
