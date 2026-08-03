// Package endpoint validates the optional AWS-compatible emulator endpoint.
package endpoint

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

const EnvironmentVariable = "MAGELIFT_AWS_ENDPOINT_URL"

// FromEnv reads the endpoint override used by account-free emulator tests.
// Empty means that the AWS SDK should use its normal service endpoints.
func FromEnv() (string, error) {
	return Parse(os.Getenv(EnvironmentVariable))
}

// Parse accepts only loopback HTTP(S) endpoints with no credentials or path.
// Restricting this override prevents an accidental environment value from
// sending AWS credentials to an arbitrary host.
func Parse(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("AWS emulator endpoint must be an absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("AWS emulator endpoint must use HTTP or HTTPS")
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("AWS emulator endpoint must not contain credentials, a path, a query, or a fragment")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host != "localhost" {
		address := net.ParseIP(host)
		if address == nil || !address.IsLoopback() {
			return "", fmt.Errorf("AWS emulator endpoint host %q is not loopback", parsed.Hostname())
		}
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
