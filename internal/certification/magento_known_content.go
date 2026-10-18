package certification

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const (
	magentoKnownContentMaxExpect = 128
	magentoKnownContentMaxBody   = 1 << 20
)

// MagentoKnownContentRequest is the HA application-integrity probe: an HTTP
// GET must return 200 and include a known storefront marker. Schema-health
// commands such as setup:db:status are not a substitute.
type MagentoKnownContentRequest struct {
	URL    string
	Expect string
}

// VerifyMagentoKnownContent GETs the URL and requires the expected marker in
// the response body. It refuses credentialed URLs, control characters, and
// empty markers. A nil client uses a 15-second timeout.
func VerifyMagentoKnownContent(ctx context.Context, client *http.Client, request MagentoKnownContentRequest) error {
	if err := validateMagentoKnownContentRequest(request); err != nil {
		return err
	}
	if ctx == nil {
		return errors.New("magento known-content context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
	if err != nil {
		return fmt.Errorf("build magento known-content request: %w", err)
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("get magento known-content: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, magentoKnownContentMaxBody))
		return fmt.Errorf("magento known-content status %d, want 200", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, magentoKnownContentMaxBody+1))
	if err != nil {
		return fmt.Errorf("read magento known-content body: %w", err)
	}
	if len(body) > magentoKnownContentMaxBody {
		return errors.New("magento known-content body exceeds 1MiB")
	}
	if !strings.Contains(string(body), request.Expect) {
		return errors.New("magento known-content body did not contain the expected marker")
	}
	return nil
}

// VerifyMagentoCatalogSKU requires the observed catalog SKU to match the
// expected known product. setup:db:status is not a substitute. Observed may
// include harness noise; the first non-empty line that is not JSON is used.
func VerifyMagentoCatalogSKU(observed, expect string) error {
	if err := validateMagentoKnownContentMarker("catalog SKU", expect); err != nil {
		return err
	}
	sku := firstMagentoCatalogSKULine(observed)
	if sku == "" {
		return errors.New("magento catalog SKU was empty")
	}
	if strings.ContainsAny(sku, "\r\n\x00") {
		return errors.New("magento catalog SKU must be single-line")
	}
	if sku != expect {
		return errors.New("magento catalog SKU did not match the expected value")
	}
	return nil
}

// VerifyMagentoSeedProbe requires the observed magelift_seed_probe label to
// match the known migrate fixture. The default GCP dump is tiny.sql
// (label tiny-fixture); it has no catalog_product_entity rows, so a sample-data
// SKU is not a substitute.
func VerifyMagentoSeedProbe(observed, expect string) error {
	if err := validateMagentoKnownContentMarker("seed probe label", expect); err != nil {
		return err
	}
	label := firstMagentoCatalogSKULine(observed)
	if label == "" {
		return errors.New("magento seed probe label was empty")
	}
	if strings.ContainsAny(label, "\r\n\x00") {
		return errors.New("magento seed probe label must be single-line")
	}
	if label != expect {
		return errors.New("magento seed probe label did not match the expected value")
	}
	return nil
}

func firstMagentoCatalogSKULine(observed string) string {
	for _, line := range strings.Split(observed, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if magentoObservedHarnessNoise(line) {
			continue
		}
		return line
	}
	return ""
}

func magentoObservedHarnessNoise(line string) bool {
	if line == "" {
		return true
	}
	switch {
	case strings.HasPrefix(line, "+"), strings.HasPrefix(line, "{"), strings.HasPrefix(line, "warning:"), strings.HasPrefix(line, "Defaulted "):
		return true
	}
	return false
}

func validateMagentoKnownContentRequest(request MagentoKnownContentRequest) error {
	parsed, err := url.Parse(strings.TrimSpace(request.URL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("magento known-content URL must be an absolute http(s) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("magento known-content URL must use http or https")
	}
	if parsed.User != nil {
		return errors.New("magento known-content URL must not contain credentials")
	}
	return validateMagentoKnownContentMarker("expect marker", request.Expect)
}

func validateMagentoKnownContentMarker(label, expect string) error {
	if strings.TrimSpace(expect) == "" || strings.ContainsAny(expect, "\r\n\x00") {
		return fmt.Errorf("magento known-content %s is required and must be single-line", label)
	}
	if len(expect) > magentoKnownContentMaxExpect {
		return fmt.Errorf("magento known-content %s exceeds %d bytes", label, magentoKnownContentMaxExpect)
	}
	for _, r := range expect {
		if unicode.IsControl(r) {
			return fmt.Errorf("magento known-content %s must not contain control characters", label)
		}
	}
	return nil
}
