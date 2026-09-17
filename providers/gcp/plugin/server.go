// Package plugin serves the versioned typed operations over go-plugin
// net/rpc. Handlers are pure orchestration over the provider packages;
// every cloud touch goes through an injectable seam for fake-client tests.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/secretsafe"
	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	gcpops "github.com/magelift/magelift/providers/gcp/ops"
	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
	"github.com/magelift/magelift/sdk"
)

// OperationVersion is the version every operation reports until one of them
// needs an independent minor bump.
const OperationVersion = "1.0"

// Runtimes served by this provider binary.
var Runtimes = []string{"gke-autopilot", "gke-standard"}

// Timeout bounds one operation. RetryableOnTimeout reports whether a client
// may safely retry after a timeout: true only when the operation provably
// performed no mutation.
type Timeout struct {
	Duration           time.Duration
	RetryableOnTimeout bool
}

// DefaultTimeouts bounds every operation. Mutations are never retryable on
// timeout: an ambiguous timeout must reconcile (re-read state), not replay.
func DefaultTimeouts() map[sdk.Operation]Timeout {
	fastRead := Timeout{Duration: 60 * time.Second, RetryableOnTimeout: true}
	slowRead := Timeout{Duration: 5 * time.Minute, RetryableOnTimeout: true}
	mutation := Timeout{Duration: 10 * time.Minute}
	return map[sdk.Operation]Timeout{
		sdk.OpDescribe:               {Duration: 15 * time.Second, RetryableOnTimeout: true},
		sdk.OpValidateConfig:         {Duration: 15 * time.Second, RetryableOnTimeout: true},
		sdk.OpPlan:                   slowRead,
		sdk.OpPreview:                {Duration: 10 * time.Minute, RetryableOnTimeout: true},
		sdk.OpApply:                  {Duration: 60 * time.Minute},
		sdk.OpOutputs:                {Duration: 2 * time.Minute, RetryableOnTimeout: true},
		sdk.OpDestroy:                {Duration: 60 * time.Minute},
		sdk.OpDestroyLeftoverBackups: mutation,
		sdk.OpBootstrapVerify:        fastRead,
		sdk.OpBootstrapEnsure:        mutation,
		sdk.OpStateStatus:            fastRead,
		sdk.OpStateLock:              fastRead,
		sdk.OpStateUnlock:            fastRead,
		sdk.OpStateBackup:            {Duration: 15 * time.Minute},
		sdk.OpStateRestore:           {Duration: 15 * time.Minute},
		sdk.OpSecretList:             fastRead,
		sdk.OpSecretSet:              fastRead,
		sdk.OpSecretRemove:           fastRead,
		sdk.OpSecretRead:             fastRead,
		sdk.OpTailLogs:               {Duration: 2 * time.Minute, RetryableOnTimeout: true},
		sdk.OpCheckRuntime:           {Duration: 2 * time.Minute, RetryableOnTimeout: true},
		sdk.OpPrepareExec:            fastRead,
		sdk.OpPrepareTunnel:          fastRead,
		sdk.OpCostInputs:             {Duration: 2 * time.Minute, RetryableOnTimeout: true},
		sdk.OpInventory:              {Duration: 2 * time.Minute, RetryableOnTimeout: true},
		sdk.OpDelete:                 {Duration: 5 * time.Minute},
		sdk.OpEdgePlan:               {Duration: 2 * time.Minute, RetryableOnTimeout: true},
		sdk.OpEdgeExecute:            mutation,
		sdk.OpResiliencePlan:         {Duration: 2 * time.Minute, RetryableOnTimeout: true},
		sdk.OpResilienceExecute:      mutation,
		sdk.OpDeployAppPhase:         {Duration: 45 * time.Minute},
	}
}

// Server implements the 30 plugin operations. Nil seam fields select
// production wiring; tests inject fakes.
type Server struct {
	Version string

	// NewDeployStores builds the candidate and runtime stores for deploy
	// phases. Nil builds production stores from the client factory.
	NewDeployStores func(factory kube.ClientFactory, outputs map[string]any) (*kube.CandidateStore, *kube.DeploymentRuntime, error)
	// ExecTokens mints fresh bearers for exec/tunnel launch kubeconfigs.
	// Nil uses ambient ADC. Tests stub it; production leaves it nil.
	ExecTokens kube.TokenSource

	Admission stackRegionAdmission
	Bootstrap gcpops.Bootstrap
	State     gcpops.State
	Secrets   gcpops.Secrets
	Estimator gcpcost.Estimator

	NewStack             func(ctx context.Context, stackName string, spec gcpstack.Spec, backendURL string) (Automation, error)
	KubeClients          kube.ClientFactory
	NewEdgeAdapter       func(ctx context.Context, project string) (sdk.EdgeAdapter, error)
	NewResilienceAdapter func(ctx context.Context, project string) (sdk.ResilienceAdapter, error)
	NewCleanupSQL        func(ctx context.Context) (gcpresilience.CloudSQLAPI, error)

	Timeouts map[sdk.Operation]Timeout
}

// stackRegionAdmission abstracts admission for tests.
type stackRegionAdmission interface {
	AdmitSpec(context.Context, gcpstack.Spec) (gcpstack.Spec, error)
}

func (s *Server) timeoutFor(operation sdk.Operation) Timeout {
	if s != nil && s.Timeouts != nil {
		if timeout, ok := s.Timeouts[operation]; ok {
			return timeout
		}
	}
	return DefaultTimeouts()[operation]
}

func (s *Server) admission() stackRegionAdmission {
	if s != nil && s.Admission != nil {
		return s.Admission
	}
	return gcpstack.RegionAdmission{}
}

// loadSpec unmarshals the opaque plan the server stored. Corrupt plans fail
// as integrity errors: the bytes crossed core storage uninspected, so any
// corruption means storage or transport tampering, never operator input.
func loadSpec(opaque []byte) (gcpstack.Spec, *sdk.OperationError) {
	if len(opaque) == 0 {
		return gcpstack.Spec{}, InvalidError("stored plan is empty; re-plan the environment")
	}
	var spec gcpstack.Spec
	if err := json.Unmarshal(opaque, &spec); err != nil {
		return gcpstack.Spec{}, &sdk.OperationError{Code: sdk.ErrCodeIntegrity, Message: "stored plan is corrupt; re-plan the environment"}
	}
	return spec, nil
}

// checkEnvelope fails closed when the request envelope does not match the
// stored plan: applying a plan against a changed configuration must re-plan,
// never silently drift.
func checkEnvelope(envelope sdk.Envelope, stored sdk.StoredPlan, spec gcpstack.Spec) *sdk.OperationError {
	if mismatch := envelopeMismatch(envelope, spec); mismatch != "" {
		return &sdk.OperationError{Code: sdk.ErrCodeIntegrity, Message: mismatch}
	}
	if stored.StackName != "" && envelope.StackName != "" && stored.StackName != envelope.StackName {
		return &sdk.OperationError{Code: sdk.ErrCodeIntegrity, Message: "request stack does not match the stored plan; re-plan the environment"}
	}
	if stored.StateBackendURL != "" && envelope.StateBackendURL != "" && stored.StateBackendURL != envelope.StateBackendURL {
		return &sdk.OperationError{Code: sdk.ErrCodeIntegrity, Message: "request state backend does not match the stored plan; re-plan the environment"}
	}
	return nil
}

func envelopeMismatch(envelope sdk.Envelope, spec gcpstack.Spec) string {
	if envelope.Project != "" && envelope.Project != spec.Identity.Project {
		return "request project does not match the stored plan; re-plan the environment"
	}
	if envelope.Environment != "" && envelope.Environment != spec.Identity.Environment {
		return "request environment does not match the stored plan; re-plan the environment"
	}
	if envelope.Region != "" && envelope.Region != spec.Identity.Region {
		return "request region does not match the stored plan; re-plan the environment"
	}
	return ""
}

// InvalidError classifies operator-fixable input failures.
func InvalidError(message string) *sdk.OperationError {
	return &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: message}
}

// mapError classifies cloud and transport failures into the typed taxonomy.
// Every message passes through the credential redactor: provider output can
// carry credential-shaped material that must never cross the wire in text.
func mapError(err error) *sdk.OperationError {
	if err == nil {
		return nil
	}
	message, _ := secretsafe.RedactSensitiveText(err.Error())
	if automation.IsConcurrentUpdate(err) {
		return &sdk.OperationError{Code: sdk.ErrCodeConflict, Message: message + " (another update holds the stack; no mutation was applied)", Retryable: true}
	}
	var ownershipErr *automation.PreviewOwnershipError
	if errors.As(err, &ownershipErr) {
		return &sdk.OperationError{Code: sdk.ErrCodeConflict, Message: message}
	}
	if errors.Is(err, gcpstate.ErrLocked) {
		return &sdk.OperationError{Code: sdk.ErrCodeConflict, Message: message}
	}
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		return &sdk.OperationError{Code: sdk.ErrCodeCredential, Message: "GCP credentials expired or were revoked; re-authenticate: " + message}
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
		case 401, 403:
			return &sdk.OperationError{Code: sdk.ErrCodeCredential, Message: "GCP credentials lack permission; re-authenticate: " + message}
		case 404:
			return &sdk.OperationError{Code: sdk.ErrCodeNotFound, Message: message}
		case 409:
			return &sdk.OperationError{Code: sdk.ErrCodeConflict, Message: message}
		case 429, 500, 502, 503, 504:
			return &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message, Retryable: true}
		default:
			return &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message}
		}
	}
	return &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message}
}

// marshalJSON encodes a result payload, failing as internal: marshal of a
// server-built value cannot fail on operator input.
func marshalJSON(value any) ([]byte, *sdk.OperationError) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: "encode operation result: " + err.Error()}
	}
	return data, nil
}

// unmarshalJSON decodes a request payload, failing as invalid: malformed
// bytes are always a core-side bug or a version skew, reported plainly.
func unmarshalJSON(data []byte, value any, what string) *sdk.OperationError {
	if err := json.Unmarshal(data, value); err != nil {
		return InvalidError("malformed " + what + ": " + err.Error())
	}
	return nil
}
