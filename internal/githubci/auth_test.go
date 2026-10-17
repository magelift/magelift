package githubci

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTokenResolutionUsesExplicitGitHubLoginFirst(t *testing.T) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		t.Run(name, func(t *testing.T) {
			token, err := resolveToken(context.Background(), func(key string) string {
				if key == name {
					return testCanaryToken
				}
				return ""
			}, func(context.Context) (string, error) { t.Fatal("unexpected CLI login"); return "", nil })
			if err != nil || token != testCanaryToken {
				t.Fatal("explicit GitHub login was not selected")
			}
		})
	}
	token, err := resolveToken(context.Background(), func(name string) string { return name }, func(context.Context) (string, error) { t.Fatal("unexpected CLI login"); return "", nil })
	if err != nil || token != "GH_TOKEN" {
		t.Fatal("GH_TOKEN must win")
	}
}

func TestTokenResolutionReusesCLIAndSanitizesFailures(t *testing.T) {
	calls := 0
	token, err := resolveToken(context.Background(), func(string) string { return "" }, func(context.Context) (string, error) { calls++; return testCanaryToken + "\n", nil })
	if err != nil || token != testCanaryToken || calls != 1 {
		t.Fatal("CLI login not reused")
	}
	_, err = resolveToken(context.Background(), func(string) string { return "" }, func(context.Context) (string, error) { return "", errors.New(testCanaryToken) })
	if err == nil || strings.Contains(err.Error(), testCanaryToken) {
		t.Fatal("missing or unsafe login diagnostic")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = resolveToken(ctx, func(string) string { t.Fatal("read canceled environment"); return "" }, func(context.Context) (string, error) { t.Fatal("ran canceled CLI"); return "", nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
