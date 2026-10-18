package plugin

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/magelift/magelift/internal/automation"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	"github.com/magelift/magelift/sdk"
)

const expiredPreviewTimestamp = "2020-01-01T00:00:00Z"

type recordingAutomation struct {
	preview  map[string]int
	destroy  map[string]int
	calls    []string
	requests []automation.Request
}

func (s *recordingAutomation) Preview(_ context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	s.calls = append(s.calls, "preview")
	s.requests = append(s.requests, request)
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, "teardown-preview\n")
	}
	return s.preview, nil
}

func (s *recordingAutomation) Update(_ context.Context, request automation.Request, _ io.Writer) (map[string]int, error) {
	s.calls = append(s.calls, "update")
	s.requests = append(s.requests, request)
	return map[string]int{"create": 1}, nil
}

func (s *recordingAutomation) Destroy(_ context.Context, request automation.Request, _ io.Writer) (map[string]int, error) {
	s.calls = append(s.calls, "destroy")
	s.requests = append(s.requests, request)
	return s.destroy, nil
}

func (s *recordingAutomation) Outputs(context.Context) (auto.OutputMap, error) {
	s.calls = append(s.calls, "outputs")
	return auto.OutputMap{}, nil
}

func planTeardown(t *testing.T, server *Server, digest string) sdk.StoredPlan {
	t.Helper()
	request := testPlanRequest()
	request.AllowExpiredPreview = true
	request.Envelope.ExpiresAt = expiredPreviewTimestamp
	block := "project: example-gcp-project\nregion: europe-west1\nnetworkCidr: 10.20.0.0/16\nencryptionKeySecret: magento-crypt-key\n"
	if digest != "" {
		block = "project: example-gcp-project\nregion: europe-west1\nnetworkCidr: 10.20.0.0/16\nimageDigest: " + digest + "\nencryptionKeySecret: magento-crypt-key\n"
	}
	request.TargetBlock = []byte(block)
	planned, operr := server.Plan(context.Background(), request)
	if operr != nil {
		t.Fatal(operr)
	}
	var spec gcpstack.Spec
	if err := json.Unmarshal(planned.Plan.Opaque, &spec); err != nil {
		t.Fatal(err)
	}
	if !spec.AllowExpiredPreview || spec.Lifecycle.ExpiresAt.IsZero() || !spec.Lifecycle.ExpiresAt.Before(time.Now()) {
		t.Fatalf("stored teardown spec = allow %v expires %s", spec.AllowExpiredPreview, spec.Lifecycle.ExpiresAt)
	}
	if spec.Artifact.ImageDigest != digest {
		t.Fatalf("stored digest = %q, want %q", spec.Artifact.ImageDigest, digest)
	}
	return planned.Plan
}

func teardownCall(t *testing.T, plan sdk.StoredPlan) *sdk.StackCall {
	t.Helper()
	envelope := testEnvelope()
	envelope.StackName = plan.StackName
	envelope.StateBackendURL = plan.StateBackendURL
	return &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan}
}

func TestExpiredAbsentDigestTeardownPreviewThenDestroy(t *testing.T) {
	t.Parallel()
	backend := &recordingAutomation{preview: map[string]int{"delete": 4}, destroy: map[string]int{"delete": 4}}
	constructed := 0
	server := &Server{Admission: stubAdmission{}, NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		constructed++
		return backend, nil
	}}
	call := teardownCall(t, planTeardown(t, server, ""))

	previewed, operr := server.Preview(context.Background(), call)
	if operr != nil {
		t.Fatal(operr)
	}
	if previewed.Summary.Delete != 4 || len(previewed.Diagnostics) != 1 || previewed.Diagnostics[0] != "teardown-preview" {
		t.Fatalf("preview = %#v", previewed)
	}
	destroyed, operr := server.Destroy(context.Background(), call)
	if operr != nil {
		t.Fatal(operr)
	}
	if destroyed.Summary.Delete != 4 {
		t.Fatalf("destroy summary = %#v", destroyed.Summary)
	}
	if constructed != 2 || len(backend.calls) != 2 || backend.calls[0] != "preview" || backend.calls[1] != "destroy" {
		t.Fatalf("constructed=%d calls=%v", constructed, backend.calls)
	}
	if len(backend.requests) != 2 || !backend.requests[0].Destroy || !backend.requests[1].Destroy {
		t.Fatalf("requests = %#v, want Destroy on preview and destroy", backend.requests)
	}
}

func TestExpiredValidDigestTeardownPreview(t *testing.T) {
	t.Parallel()
	backend := &recordingAutomation{preview: map[string]int{"delete": 1}}
	server := &Server{Admission: stubAdmission{}, NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		return backend, nil
	}}
	digest := "ghcr.io/magelift/magento@sha256:" + strings.Repeat("b", 64)
	previewed, operr := server.Preview(context.Background(), teardownCall(t, planTeardown(t, server, digest)))
	if operr != nil {
		t.Fatal(operr)
	}
	if previewed.Summary.Delete != 1 {
		t.Fatalf("preview summary = %#v", previewed.Summary)
	}
	if len(backend.calls) != 1 || backend.calls[0] != "preview" || len(backend.requests) != 1 || !backend.requests[0].Destroy {
		t.Fatalf("calls=%v requests=%#v", backend.calls, backend.requests)
	}
}

func TestApplyRejectsExpiredTeardownBeforeBackend(t *testing.T) {
	t.Parallel()
	constructed := false
	server := &Server{Admission: stubAdmission{}, NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		constructed = true
		return &recordingAutomation{}, nil
	}}
	digest := "ghcr.io/magelift/magento@sha256:" + strings.Repeat("c", 64)
	_, operr := server.Apply(context.Background(), teardownCall(t, planTeardown(t, server, digest)))
	if operr == nil || operr.Code != sdk.ErrCodeInvalid || !strings.Contains(operr.Message, "preview environment has expired") {
		t.Fatalf("apply error = %v", operr)
	}
	if constructed {
		t.Fatal("strict apply constructed a backend")
	}
}

func TestObservePreviewRejectsAbsentDigestBeforeBackend(t *testing.T) {
	t.Parallel()
	constructed := false
	server := &Server{Admission: stubAdmission{}, NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		constructed = true
		return &recordingAutomation{}, nil
	}}
	request := testPlanRequest()
	request.RuntimeObserve = true
	request.TargetBlock = []byte("project: example-gcp-project\nregion: europe-west1\nnetworkCidr: 10.20.0.0/16\nencryptionKeySecret: magento-crypt-key\n")
	planned, operr := server.Plan(context.Background(), request)
	if operr != nil {
		t.Fatal(operr)
	}
	_, operr = server.Preview(context.Background(), teardownCall(t, planned.Plan))
	if operr == nil || operr.Code != sdk.ErrCodeInvalid || !strings.Contains(operr.Message, "artifact image digest") {
		t.Fatalf("observe preview error = %v", operr)
	}
	if constructed {
		t.Fatal("observe preview constructed a backend")
	}
}

func TestTeardownPreviewRejectsMalformedDigestBeforeBackend(t *testing.T) {
	t.Parallel()
	constructed := false
	server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		constructed = true
		return &recordingAutomation{}, nil
	}}
	spec := testSpec()
	spec.AllowExpiredPreview = true
	spec.Artifact.ImageDigest = "ghcr.io/magelift/magento:latest"
	expires, err := time.Parse(time.RFC3339, expiredPreviewTimestamp)
	if err != nil {
		t.Fatal(err)
	}
	spec.Lifecycle.ExpiresAt = expires
	_, operr := server.Preview(context.Background(), teardownCall(t, storedTestPlan(t, spec)))
	if operr == nil || operr.Code != sdk.ErrCodeInvalid || !strings.Contains(operr.Message, "artifact image digest") {
		t.Fatalf("malformed digest error = %v", operr)
	}
	if constructed {
		t.Fatal("malformed teardown preview constructed a backend")
	}
}

func TestPreviewRejectsExpiredSpecWithoutTeardownFlag(t *testing.T) {
	t.Parallel()
	constructed := false
	server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		constructed = true
		return &recordingAutomation{}, nil
	}}
	spec := testSpec()
	expires, err := time.Parse(time.RFC3339, expiredPreviewTimestamp)
	if err != nil {
		t.Fatal(err)
	}
	spec.Lifecycle.ExpiresAt = expires
	_, operr := server.Preview(context.Background(), teardownCall(t, storedTestPlan(t, spec)))
	if operr == nil || operr.Code != sdk.ErrCodeInvalid || !strings.Contains(operr.Message, "preview environment has expired") {
		t.Fatalf("expired preview error = %v", operr)
	}
	if constructed {
		t.Fatal("expired deploy preview constructed a backend")
	}
}

func TestDeployPreviewLeavesDestroyUnset(t *testing.T) {
	t.Parallel()
	backend := &recordingAutomation{preview: map[string]int{"create": 1}}
	server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		return backend, nil
	}}
	previewed, operr := server.Preview(context.Background(), teardownCall(t, storedTestPlan(t, testSpec())))
	if operr != nil {
		t.Fatal(operr)
	}
	if previewed.Summary.Create != 1 {
		t.Fatalf("preview summary = %#v", previewed.Summary)
	}
	if len(backend.calls) != 1 || backend.calls[0] != "preview" || len(backend.requests) != 1 || backend.requests[0].Destroy {
		t.Fatalf("calls=%v requests=%#v", backend.calls, backend.requests)
	}
}
