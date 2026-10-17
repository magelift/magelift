package githubci

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	testCanaryToken = "ghp_CANARY_TEST_TOKEN_DO_NOT_LEAK_0123456789"
	testCanaryValue = "CANARY_VARIABLE_VALUE_SECRET_XYZ"
	testCanaryBody  = "CANARY_RAW_RESPONSE_BODY_LEAK"
)

type rewriteTransport struct {
	base *url.URL
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = t.base.Scheme
	clone.URL.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	doer := &http.Client{
		Transport: &rewriteTransport{base: base},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	client, err := New(testCanaryToken, doer)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func assertNoCanaryLeak(t *testing.T, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	for _, canary := range []string{testCanaryToken, testCanaryValue, testCanaryBody} {
		if strings.Contains(msg, canary) {
			t.Fatalf("error leaks canary %q: %s", canary, msg)
		}
	}
}

func TestNewRequiresToken(t *testing.T) {
	_, err := New("  ", nil)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestResolveRepositorySendsHeadersAndDecodes(t *testing.T) {
	var gotMethod, gotVersion, gotAuth, gotPath string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotVersion = r.Header.Get("X-GitHub-Api-Version")
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":42,"name":"shop","full_name":"acme/shop","default_branch":"main","owner":{"id":7,"login":"acme"}}`)
	})
	repo, err := client.ResolveRepository(context.Background(), "acme", "shop")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotVersion != githubAPIVersion || gotAuth != "Bearer "+testCanaryToken {
		t.Fatalf("headers: method=%s version=%s auth=%q", gotMethod, gotVersion, gotAuth)
	}
	if gotPath != "/repos/acme/shop" {
		t.Fatalf("path = %q", gotPath)
	}
	if repo.ID != 42 || repo.OwnerID != 7 || repo.FullName != "acme/shop" || repo.DefaultBranch != "main" {
		t.Fatalf("repo = %+v", repo)
	}
}

func TestResolveRepositoryRejectsMismatchedOwner(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":1,"name":"shop","full_name":"other/shop","default_branch":"main","owner":{"id":2,"login":"other"}}`)
	})
	_, err := client.ResolveRepository(context.Background(), "acme", "shop")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	assertNoCanaryLeak(t, err)
}

func TestResolveRepositoryRejectsMalformedJSON(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"id":1,"name":"shop","full_name":"acme/shop","default_branch":"main","owner":{"id":2,"login":"acme"}}{`)
	})
	_, err := client.ResolveRepository(context.Background(), "acme", "shop")
	if err == nil {
		t.Fatal("expected malformed error")
	}
}

func TestResolveRepositoryRejectsInvalidSegment(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	_, err := client.ResolveRepository(context.Background(), "acme/foo/bar", "shop")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestEnsureVariableCreatesWhenAbsent(t *testing.T) {
	var mu sync.Mutex
	state := map[string]string{}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/actions/variables/MAGELIFT_GCP_REGION"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/actions/variables"):
			var body createVariableRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Name != "MAGELIFT_GCP_REGION" || body.Value != "europe-west1" {
				t.Fatalf("post body = %+v", body)
			}
			state[body.Name] = body.Value
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	result, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", "europe-west1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Name != "MAGELIFT_GCP_REGION" {
		t.Fatalf("result = %+v", result)
	}
}

func TestEnsureVariableNoopWhenSameValue(t *testing.T) {
	posts := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			_, _ = io.WriteString(w, `{"name":"MAGELIFT_GCP_REGION","value":"europe-west1"}`)
		case r.Method == http.MethodPost:
			posts++
			t.Fatal("unexpected POST")
		}
	})
	result, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", "europe-west1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Created || result.Name != "MAGELIFT_GCP_REGION" {
		t.Fatalf("result = %+v", result)
	}
	if posts != 0 {
		t.Fatalf("posts = %d", posts)
	}
}

func TestEnsureVariableConflictWhenDifferent(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s", r.Method)
		}
		_, _ = io.WriteString(w, `{"name":"MAGELIFT_GCP_REGION","value":"`+testCanaryValue+`"}`)
	})
	_, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", "europe-west1")
	var conflict *ErrVariableConflict
	if !errors.As(err, &conflict) || conflict.Name != "MAGELIFT_GCP_REGION" {
		t.Fatalf("err = %v", err)
	}
	assertNoCanaryLeak(t, err)
}

func TestEnsureVariablePost409RereadSameValue(t *testing.T) {
	var mu sync.Mutex
	gets := 0
	value := "europe-west1"
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			gets++
			if gets == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, `{"name":"MAGELIFT_GCP_REGION","value":"`+value+`"}`)
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
		default:
			t.Fatalf("method %s", r.Method)
		}
	})
	result, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", value)
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatalf("result = %+v", result)
	}
}

func TestEnsureVariablePost409RereadDifferentConflict(t *testing.T) {
	gets := 0
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets++
			if gets == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = io.WriteString(w, `{"name":"MAGELIFT_GCP_REGION","value":"`+testCanaryValue+`"}`)
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
		}
	})
	_, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", "europe-west1")
	var conflict *ErrVariableConflict
	if !errors.As(err, &conflict) || conflict.Name != "MAGELIFT_GCP_REGION" {
		t.Fatalf("err = %v", err)
	}
	assertNoCanaryLeak(t, err)
}

func TestEnsureVariableForbidden(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", "europe-west1")
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err = %v", err)
	}
	assertNoCanaryLeak(t, err)
}

func TestEnsureVariableRejectsDisallowedName(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	_, err := client.EnsureVariable(context.Background(), "acme", "shop", "GITHUB_TOKEN", "value")
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestEnsureVariableRejectsCredentialLikeValue(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	_, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", "ghp_not_a_real_token_but_pattern")
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestEnsureVariableRejectsOversizeValue(t *testing.T) {
	client := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	big := strings.Repeat("a", maxVariableValueLen+1)
	_, err := client.EnsureVariable(context.Background(), "acme", "shop", "MAGELIFT_GCP_REGION", big)
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestClientRejectsOversizeResponse(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, maxResponseBytes+10))
	})
	_, err := client.ResolveRepository(context.Background(), "acme", "shop")
	if err == nil {
		t.Fatal("expected size error")
	}
}

func TestClientHonorsContextCancel(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := client.ResolveRepository(ctx, "acme", "shop")
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("err = %v", err)
	}
}

func TestRedirectDoesNotForwardAuthorization(t *testing.T) {
	var sawRedirectAuth bool
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawRedirectAuth = true
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":1,"name":"shop","full_name":"acme/shop","default_branch":"main","owner":{"id":2,"login":"acme"}}`)
	}))
	t.Cleanup(redirect.Close)
	redirectURL, _ := url.Parse(redirect.URL)

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
	}))
	t.Cleanup(source.Close)
	base, err := url.Parse(source.URL)
	if err != nil {
		t.Fatal(err)
	}
	client, err := New(testCanaryToken, nil)
	if err != nil {
		t.Fatal(err)
	}
	client.doer.(*http.Client).Transport = &rewriteTransport{base: base}
	_, err = client.ResolveRepository(context.Background(), "acme", "shop")
	if err == nil {
		t.Fatal("expected non-OK handling for redirect response")
	}
	if sawRedirectAuth {
		t.Fatal("authorization reached redirect target")
	}
}

func TestTransportErrorDoesNotLeakUnderlyingMessage(t *testing.T) {
	doer := &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial tcp: secret " + testCanaryToken)
		}),
	}
	client, err := New(testCanaryToken, doer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ResolveRepository(context.Background(), "acme", "shop")
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("err = %v", err)
	}
	assertNoCanaryLeak(t, err)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestJSONDecoderRejectsAllTrailingData(t *testing.T) {
	for _, suffix := range []string{"}", " null", " []", " garbage"} {
		var value map[string]any
		if err := decodeJSONObject([]byte(`{"ok":true}`+suffix), &value); err == nil {
			t.Fatalf("accepted suffix %q", suffix)
		}
	}
}

func TestRepositorySegmentsRefusePathAndWhitespaceBeforeHTTP(t *testing.T) {
	client, err := New(testCanaryToken, roundTripperDoer{fn: func(*http.Request) (*http.Response, error) { t.Fatal("unexpected HTTP request"); return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	for _, segment := range []string{"..", ".", " owner", "owner ", "owner?x", "owner#x", "owner/x", "owner\\x"} {
		if _, err := client.ResolveRepository(context.Background(), segment, "shop"); err == nil {
			t.Fatalf("accepted segment %q", segment)
		}
	}
}

type roundTripperDoer struct {
	fn func(*http.Request) (*http.Response, error)
}

func (d roundTripperDoer) Do(r *http.Request) (*http.Response, error) { return d.fn(r) }
