package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	buildpipeline "github.com/magelift/magelift/internal/build/pipeline"
	cvss30 "github.com/pandatix/go-cvss/30"
	cvss31 "github.com/pandatix/go-cvss/31"
	cvss40 "github.com/pandatix/go-cvss/40"
)

var composerBinaryVersion = regexp.MustCompile(`(?i)Composer version\s+(2\.\d+\.\d+)(?:\s|$)`)

func detectBuilderComposer(ctx context.Context, image, platform string) (string, error) {
	return detectComposer(ctx, image, platform, "composer")
}

func detectComposer(ctx context.Context, image, platform, entrypoint string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--network=none", "--platform", platform, "--entrypoint", entrypoint, image, "--version", "--no-ansi")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("inspect builder Composer version: %w", err)
	}
	match := composerBinaryVersion.FindSubmatch(output)
	if len(match) != 2 {
		return "", fmt.Errorf("builder Composer version is unreadable")
	}
	return string(match[1]), nil
}

func composerVersionMatches(required, actual string) bool {
	return buildpipeline.RuntimeVersionMatches(required, actual)
}

type composerAdvisory struct {
	ID       string `json:"id"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
}

func advisorySeverity(advisory composerAdvisory) (int, bool) {
	level, known := 0, false
	switch strings.ToUpper(advisory.DatabaseSpecific.Severity) {
	case "CRITICAL":
		level, known = 4, true
	case "HIGH":
		level, known = 3, true
	case "MODERATE", "MEDIUM":
		level, known = 2, true
	case "LOW", "NONE":
		level, known = 1, true
	}
	for _, item := range advisory.Severity {
		var score float64
		var err error
		switch {
		case item.Type == "CVSS_V3" && strings.HasPrefix(item.Score, "CVSS:3.0/"):
			var v *cvss30.CVSS30
			v, err = cvss30.ParseVector(item.Score)
			if err == nil {
				score = v.BaseScore()
			}
		case item.Type == "CVSS_V3" && strings.HasPrefix(item.Score, "CVSS:3.1/"):
			var v *cvss31.CVSS31
			v, err = cvss31.ParseVector(item.Score)
			if err == nil {
				score = v.BaseScore()
			}
		case item.Type == "CVSS_V4" && strings.HasPrefix(item.Score, "CVSS:4.0/"):
			var v *cvss40.CVSS40
			v, err = cvss40.ParseVector(item.Score)
			if err == nil {
				score = v.Score()
			}
		default:
			continue
		}
		if err != nil {
			continue
		}
		known = true
		current := 1
		if score >= 9 {
			current = 4
		} else if score >= 7 {
			current = 3
		} else if score >= 4 {
			current = 2
		}
		if current > level {
			level = current
		}
	}
	return level, known
}

func checkComposerAdvisories(ctx context.Context, client *http.Client, endpoint, version string, diagnostics io.Writer) error {
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	warn := func(reason string) {
		fmt.Fprintf(diagnostics, "magelift: Composer advisory check incomplete: %s\n", reason)
	}
	seen := map[string]bool{}
	token := ""
	for page := 0; page < 100; page++ {
		query := map[string]any{"package": map[string]string{"ecosystem": "Packagist", "name": "composer/composer"}, "version": version}
		if token != "" {
			query["page_token"] = token
		}
		payload, _ := json.Marshal(query)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			warn("invalid request")
			return nil
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			warn("lookup unavailable")
			return nil
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			warn(fmt.Sprintf("lookup returned HTTP %d", resp.StatusCode))
			return nil
		}
		if readErr != nil || len(body) > 8<<20 || !strings.HasPrefix(strings.TrimSpace(string(body)), "{") {
			warn("invalid response")
			return nil
		}
		var results struct {
			Vulns         []composerAdvisory `json:"vulns"`
			NextPageToken string             `json:"next_page_token"`
		}
		if json.Unmarshal(body, &results) != nil {
			warn("invalid response")
			return nil
		}
		for _, advisory := range results.Vulns {
			link := "https://osv.dev/vulnerability/" + advisory.ID
			level, known := advisorySeverity(advisory)
			switch {
			case level == 4:
				return fmt.Errorf("Composer %s has critical advisory %s (%s); select a patched Composer image", version, advisory.ID, link)
			case level == 3:
				fmt.Fprintf(diagnostics, "magelift: Composer %s has high advisory %s (%s)\n", version, advisory.ID, link)
			case !known:
				warn("unknown severity for " + advisory.ID)
			}
		}
		token = results.NextPageToken
		if token == "" {
			return nil
		}
		if seen[token] {
			warn("repeated page token")
			return nil
		}
		seen[token] = true
	}
	warn("pagination limit reached")
	return nil
}
