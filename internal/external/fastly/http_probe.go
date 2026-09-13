package fastly

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPHealthProbeConfig describes the disposable route proof used by live
// acceptance. The origin URL is an operator-selected health endpoint; the
// route is built from the requested Fastly domain so a successful origin call
// cannot be mistaken for a successful edge route.
type HTTPHealthProbeConfig struct {
	OriginURL      string
	OriginHost     string
	ExpectedCNAME  string
	RoutePath      string
	ExpectedStatus int
	RouteTimeout   time.Duration
	RoutePoll      time.Duration
	Client         *http.Client
}

const (
	defaultFastlyRouteTimeout = 90 * time.Second
	defaultFastlyRoutePoll    = 2 * time.Second
)

// HTTPHealthProbe performs the minimum external proof needed by the Fastly
// lifecycle: the origin is healthy before mutation, and the requested domain
// resolves to the expected Fastly CNAME and returns the expected response
// through Fastly afterward. It deliberately does not claim WAF, failover, or
// rollback coverage; those require provider-specific policy exercises.
type HTTPHealthProbe struct {
	originURL      *url.URL
	originHost     string
	expectedCNAME  string
	routePath      string
	expectedStatus int
	routeTimeout   time.Duration
	routePoll      time.Duration
	client         *http.Client
}

func NewHTTPHealthProbe(config HTTPHealthProbeConfig) (*HTTPHealthProbe, error) {
	if strings.TrimSpace(config.OriginURL) == "" {
		return nil, errors.New("Fastly HTTP health probe origin URL is required")
	}
	originURL, err := url.Parse(config.OriginURL)
	if err != nil || originURL.Host == "" || (originURL.Scheme != "http" && originURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid Fastly HTTP health probe origin URL %q", config.OriginURL)
	}
	originHost := strings.TrimSpace(config.OriginHost)
	if originHost != "" && !validFastlyHost(originHost) {
		return nil, fmt.Errorf("invalid Fastly HTTP health probe origin host %q", config.OriginHost)
	}
	expectedCNAME := strings.TrimSuffix(strings.TrimSpace(config.ExpectedCNAME), ".")
	if !validFastlyHost(expectedCNAME) || net.ParseIP(expectedCNAME) != nil {
		return nil, fmt.Errorf("invalid Fastly expected CNAME %q", config.ExpectedCNAME)
	}
	routePath := config.RoutePath
	if routePath == "" {
		routePath = "/"
	}
	if !strings.HasPrefix(routePath, "/") || strings.ContainsAny(routePath, "\r\n") {
		return nil, fmt.Errorf("invalid Fastly HTTP health probe route path %q", routePath)
	}
	expectedStatus := config.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}
	if expectedStatus < 100 || expectedStatus > 599 {
		return nil, fmt.Errorf("invalid Fastly HTTP health probe status %d", expectedStatus)
	}
	routeTimeout := config.RouteTimeout
	if routeTimeout == 0 {
		routeTimeout = defaultFastlyRouteTimeout
	}
	routePoll := config.RoutePoll
	if routePoll == 0 {
		routePoll = defaultFastlyRoutePoll
	}
	if routeTimeout <= 0 || routePoll <= 0 || routePoll > routeTimeout {
		return nil, fmt.Errorf("invalid Fastly route convergence budget timeout=%s poll=%s", routeTimeout, routePoll)
	}
	client := config.Client
	if client == nil {
		transport, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, errors.New("Fastly HTTP health probe default transport is not an HTTP transport")
		}
		transport = transport.Clone()
		if originHost != "" {
			transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: originHost}
		}
		client = &http.Client{Transport: transport, Timeout: 20 * time.Second}
	}
	return &HTTPHealthProbe{originURL: originURL, originHost: originHost, expectedCNAME: expectedCNAME, routePath: routePath, expectedStatus: expectedStatus, routeTimeout: routeTimeout, routePoll: routePoll, client: client}, nil
}

func (probe *HTTPHealthProbe) Preflight(ctx context.Context, _ Request, _ string) (HealthObservation, error) {
	if ctx == nil {
		return HealthObservation{}, errors.New("Fastly HTTP health probe context is required")
	}
	if probe == nil || probe.client == nil || probe.originURL == nil {
		return HealthObservation{}, errors.New("Fastly HTTP health probe is not initialized")
	}
	healthy, err := probe.checkURL(ctx, probe.originURL.String(), false)
	if err != nil {
		return HealthObservation{}, fmt.Errorf("check Fastly origin: %w", err)
	}
	return HealthObservation{OriginHealthy: healthy}, nil
}

func (probe *HTTPHealthProbe) Verify(ctx context.Context, request Request, result Result) (HealthObservation, error) {
	if ctx == nil {
		return HealthObservation{}, errors.New("Fastly HTTP health probe context is required")
	}
	if probe == nil || probe.client == nil || probe.originURL == nil {
		return HealthObservation{}, errors.New("Fastly HTTP health probe is not initialized")
	}
	if len(request.Domains) != 1 {
		return HealthObservation{}, errors.New("Fastly HTTP health probe requires exactly one route domain")
	}
	originHealthy, err := probe.checkURL(ctx, probe.originURL.String(), false)
	if err != nil {
		return HealthObservation{}, fmt.Errorf("check Fastly origin after apply: %w", err)
	}
	routeURL := (&url.URL{Scheme: routeScheme(request.ProviderConfig), Host: request.Domains[0], Path: probe.routePath}).String()
	routeHealthy, headers, err := probe.checkRouteUntilReady(ctx, routeURL)
	if err != nil {
		return HealthObservation{}, fmt.Errorf("check Fastly route %q: %w", routeURL, err)
	}
	dnsOwnershipVerified := verifyFastlyCNAME(request.Domains[0], probe.expectedCNAME)
	fastlyHeaders := hasFastlyResponseHeaders(headers)
	return HealthObservation{
		OriginHealthy:        originHealthy,
		RouteHealthy:         routeHealthy && fastlyHeaders,
		DNSOwnershipVerified: dnsOwnershipVerified,
		TLSVerified:          request.ProviderConfig.TLS && routeScheme(request.ProviderConfig) == "https" && routeHealthy,
		PurgeVerified:        result.PurgeRequested && routeHealthy && fastlyHeaders,
	}, nil
}

func routeScheme(config Config) string {
	if config.TLS {
		return "https"
	}
	return "http"
}

func (probe *HTTPHealthProbe) checkURL(ctx context.Context, rawURL string, requireFastlyHeaders bool) (bool, error) {
	ok, _, err := probe.do(ctx, rawURL, requireFastlyHeaders)
	return ok, err
}

func (probe *HTTPHealthProbe) checkRoute(ctx context.Context, rawURL string) (bool, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, nil, err
	}
	response, err := probe.client.Do(request)
	if err != nil {
		return false, nil, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != probe.expectedStatus {
		return false, response.Header, fmt.Errorf("HTTP status %d, expected %d (via=%q x-served-by=%q x-cache=%q)", response.StatusCode, probe.expectedStatus, response.Header.Get("Via"), response.Header.Get("X-Served-By"), response.Header.Get("X-Cache"))
	}
	if !hasFastlyResponseHeaders(response.Header) {
		return false, response.Header, errors.New("Fastly response headers were not present")
	}
	return true, response.Header, nil
}

func (probe *HTTPHealthProbe) checkRouteUntilReady(ctx context.Context, rawURL string) (bool, http.Header, error) {
	deadline := time.Now().Add(probe.routeTimeout)
	var lastHeaders http.Header
	var lastErr error
	for {
		ready, headers, err := probe.checkRoute(ctx, rawURL)
		lastHeaders = headers
		if err == nil && ready {
			return true, headers, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = errors.New("route did not reach the expected response")
			}
			return false, lastHeaders, fmt.Errorf("route did not converge within %s: %w", probe.routeTimeout, lastErr)
		}
		if err := waitForFastlyProbe(ctx, probe.routePoll); err != nil {
			return false, lastHeaders, err
		}
	}
}

func waitForFastlyProbe(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for Fastly route convergence: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func (probe *HTTPHealthProbe) do(ctx context.Context, rawURL string, requireFastlyHeaders bool) (bool, http.Header, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, nil, err
	}
	if probe.originHost != "" {
		request.Host = probe.originHost
	}
	response, err := probe.client.Do(request)
	if err != nil {
		return false, nil, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != probe.expectedStatus {
		return false, response.Header, nil
	}
	if requireFastlyHeaders && !hasFastlyResponseHeaders(response.Header) {
		return false, response.Header, nil
	}
	return true, response.Header, nil
}

func verifyFastlyCNAME(domain, expected string) bool {
	actual, err := net.LookupCNAME(domain)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSuffix(actual, "."), strings.TrimSuffix(expected, "."))
}

func hasFastlyResponseHeaders(headers http.Header) bool {
	return headers.Get("Via") != "" || headers.Get("X-Served-By") != "" || headers.Get("X-Cache") != ""
}
