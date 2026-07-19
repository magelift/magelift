// Package benchmark provides the repeatable workload measurement used to set
// MageLift preset capacity. It deliberately records cost inputs separately
// from prices: prices are provider and region data, not benchmark output.
package benchmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const SchemaVersion = 1

type Request struct {
	Path   string `json:"path" yaml:"path"`
	Weight int    `json:"weight" yaml:"weight"`
}

type Catalog struct {
	Name           string `json:"name" yaml:"name"`
	Revision       string `json:"revision" yaml:"revision"`
	Edition        string `json:"edition" yaml:"edition"`
	MagentoVersion string `json:"magentoVersion" yaml:"magentoVersion"`
	PHPVersion     string `json:"phpVersion" yaml:"phpVersion"`
	Database       string `json:"database" yaml:"database"`
	Search         string `json:"search" yaml:"search"`
	Queue          string `json:"queue" yaml:"queue"`
	Cache          string `json:"cache" yaml:"cache"`
}

// Validate ensures a benchmark can be reproduced from its recorded catalog.
// A report without an identity cannot safely be used to change preset capacity.
func (c Catalog) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("benchmark catalog name is required")
	}
	if strings.TrimSpace(c.Revision) == "" {
		return errors.New("benchmark catalog revision is required")
	}
	if c.Edition != "open-source" && c.Edition != "commerce" {
		return fmt.Errorf("unsupported Magento edition %q", c.Edition)
	}
	return nil
}

type Config struct {
	URL         string
	Duration    time.Duration
	Concurrency int
	Requests    []Request
	Catalog     Catalog
	Client      *http.Client
}

type CostInput struct {
	Resource string  `json:"resource" yaml:"resource"`
	Quantity float64 `json:"quantity" yaml:"quantity"`
	Unit     string  `json:"unit" yaml:"unit"`
	Hours    float64 `json:"hours" yaml:"hours"`
}

type CostEvidence struct {
	Status        string      `json:"status" yaml:"status"`
	Provider      string      `json:"provider,omitempty" yaml:"provider,omitempty"`
	Region        string      `json:"region,omitempty" yaml:"region,omitempty"`
	HoursPerMonth float64     `json:"hoursPerMonth" yaml:"hoursPerMonth"`
	Inputs        []CostInput `json:"inputs" yaml:"inputs"`
	EstimateCents *int64      `json:"estimateCents,omitempty" yaml:"estimateCents,omitempty"`
}

type Report struct {
	SchemaVersion  int            `json:"schemaVersion" yaml:"schemaVersion"`
	TargetURL      string         `json:"targetURL" yaml:"targetURL"`
	Catalog        Catalog        `json:"catalog" yaml:"catalog"`
	Requests       []Request      `json:"requests" yaml:"requests"`
	Concurrency    int            `json:"concurrency" yaml:"concurrency"`
	DurationMillis int64          `json:"durationMillis" yaml:"durationMillis"`
	Samples        int            `json:"samples" yaml:"samples"`
	Successful     int            `json:"successful" yaml:"successful"`
	Failed         int            `json:"failed" yaml:"failed"`
	Errors         map[string]int `json:"errors,omitempty" yaml:"errors,omitempty"`
	LatencyMillis  Percentiles    `json:"latencyMillis" yaml:"latencyMillis"`
	ThroughputRPS  float64        `json:"throughputRPS" yaml:"throughputRPS"`
	Cost           CostEvidence   `json:"cost" yaml:"cost"`
}

type Percentiles struct {
	P50 float64 `json:"p50" yaml:"p50"`
	P95 float64 `json:"p95" yaml:"p95"`
	P99 float64 `json:"p99" yaml:"p99"`
}

func (r Report) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported benchmark report schema version %d", r.SchemaVersion)
	}
	if strings.TrimSpace(r.TargetURL) == "" {
		return errors.New("benchmark report target URL is required")
	}
	if err := r.Catalog.Validate(); err != nil {
		return fmt.Errorf("invalid benchmark catalog: %w", err)
	}
	if r.Samples < 0 || r.Successful < 0 || r.Failed < 0 || r.Successful+r.Failed != r.Samples {
		return errors.New("benchmark report request counts are inconsistent")
	}
	if r.Concurrency < 1 || r.DurationMillis < 0 || r.ThroughputRPS < 0 {
		return errors.New("benchmark report runtime values are invalid")
	}
	if r.LatencyMillis.P50 < 0 || r.LatencyMillis.P95 < r.LatencyMillis.P50 || r.LatencyMillis.P99 < r.LatencyMillis.P95 {
		return errors.New("benchmark report latency percentiles are invalid")
	}
	if r.Cost.HoursPerMonth <= 0 {
		return errors.New("benchmark report cost hours must be positive")
	}
	switch r.Cost.Status {
	case "unpriced", "estimated", "priced":
	default:
		return fmt.Errorf("unsupported benchmark cost status %q", r.Cost.Status)
	}
	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("benchmark URL is required")
	}
	parsed, err := url.ParseRequestURI(c.URL)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("benchmark URL must be an absolute HTTP(S) URL")
	}
	if c.Duration < time.Second || c.Duration > 24*time.Hour {
		return errors.New("benchmark duration must be between one second and 24 hours")
	}
	if c.Concurrency < 1 || c.Concurrency > 10000 {
		return errors.New("benchmark concurrency must be between one and 10000")
	}
	if len(c.Requests) == 0 {
		return errors.New("benchmark request mix must not be empty")
	}
	weight := 0
	for _, request := range c.Requests {
		if request.Path == "" || !strings.HasPrefix(request.Path, "/") {
			return fmt.Errorf("benchmark request path %q must start with /", request.Path)
		}
		if request.Weight < 1 {
			return errors.New("benchmark request weights must be positive")
		}
		weight += request.Weight
	}
	if weight > 10000 {
		return errors.New("benchmark request weights must total at most 10000")
	}
	if err := c.Catalog.Validate(); err != nil {
		return err
	}
	return nil
}

func Run(ctx context.Context, cfg Config) (Report, error) {
	if err := cfg.Validate(); err != nil {
		return Report{}, err
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	base, _ := url.Parse(cfg.URL)
	schedule := make([]string, 0)
	for _, request := range cfg.Requests {
		for i := 0; i < request.Weight; i++ {
			schedule = append(schedule, request.Path)
		}
	}
	started := time.Now()
	deadline := started.Add(cfg.Duration)
	runContext, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var next atomic.Uint64
	var mu sync.Mutex
	latencies := make([]time.Duration, 0)
	errorsByClass := make(map[string]int)
	successful, failed := 0, 0
	var wg sync.WaitGroup
	for worker := 0; worker < cfg.Concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-runContext.Done():
					return
				default:
				}
				if time.Now().After(deadline) {
					return
				}
				path := schedule[(next.Add(1)-1)%uint64(len(schedule))]
				requestURL := *base
				requestURL.Path = strings.TrimSuffix(base.Path, "/") + path
				req, err := http.NewRequestWithContext(runContext, http.MethodGet, requestURL.String(), nil)
				if err != nil {
					mu.Lock()
					failed++
					errorsByClass["request"]++
					mu.Unlock()
					continue
				}
				begin := time.Now()
				response, err := client.Do(req)
				elapsed := time.Since(begin)
				if response != nil {
					_, _ = io.Copy(io.Discard, response.Body)
					_ = response.Body.Close()
				}
				mu.Lock()
				latencies = append(latencies, elapsed)
				if err != nil {
					failed++
					errorsByClass["transport"]++
				} else if response.StatusCode >= http.StatusBadRequest {
					failed++
					errorsByClass[fmt.Sprintf("http_%d", response.StatusCode)]++
				} else {
					successful++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	ended := time.Now()
	mu.Lock()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	report := Report{
		SchemaVersion: SchemaVersion, TargetURL: cfg.URL, Catalog: cfg.Catalog, Requests: append([]Request(nil), cfg.Requests...),
		Concurrency: cfg.Concurrency, DurationMillis: ended.Sub(started).Milliseconds(), Samples: len(latencies), Successful: successful, Failed: failed,
		Errors: errorsByClass, LatencyMillis: percentileReport(latencies), Cost: CostEvidence{Status: "unpriced", HoursPerMonth: 730, Inputs: []CostInput{}},
	}
	mu.Unlock()
	if report.DurationMillis > 0 {
		report.ThroughputRPS = float64(report.Samples) / (float64(report.DurationMillis) / 1000)
	}
	if err := report.Validate(); err != nil {
		return Report{}, err
	}
	return report, nil
}

func percentileReport(values []time.Duration) Percentiles {
	if len(values) == 0 {
		return Percentiles{}
	}
	value := func(percentile float64) float64 {
		index := int(float64(len(values)-1) * percentile)
		return float64(values[index].Microseconds()) / 1000
	}
	return Percentiles{P50: value(.50), P95: value(.95), P99: value(.99)}
}
