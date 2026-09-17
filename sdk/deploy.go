// Package sdk deploy contracts govern Magento deploy execution across the
// core/provider boundary. The provider builds DeployInputs at plan time;
// the core passes the encoded inputs through to per-phase execution ops
// without decoding provider behavior. JSON keys intentionally match the
// pre-contract encoding (Go field names).
package sdk

import (
	"errors"
	"regexp"
	"strings"
)

// MagentoOverlays carries the Magento runtime contract bindings. Moved
// here from the core so deploy inputs are SDK-governed; the core keeps a
// type alias plus the binding constructors.
type MagentoOverlays struct {
	FrontName        string            `json:"FrontName"`
	CookieDomain     string            `json:"CookieDomain"`
	UnsecureBaseURL  string            `json:"UnsecureBaseURL"`
	SecureBaseURL    string            `json:"SecureBaseURL"`
	CORSOrigins      []string          `json:"CORSOrigins"`
	StorefrontOrigin string            `json:"StorefrontOrigin"`
	ConsumersMode    string            `json:"ConsumersMode"`
	ConsumerNames    []string          `json:"ConsumerNames"`
	Variables        map[string]string `json:"Variables"`
}

// DeployInputs holds the versioned Magento deploy knobs. Providers build
// it; the plugin decodes and executes it; the core treats it as opaque.
type DeployInputs struct {
	ImageDigest        string          `json:"ImageDigest"`
	DatabaseName       string          `json:"DatabaseName"`
	ApplicationMode    string          `json:"ApplicationMode"`
	ApplicationVersion string          `json:"ApplicationVersion"`
	WebRuntime         string          `json:"WebRuntime"`
	CPURequest         string          `json:"CPURequest"`
	MemoryRequest      string          `json:"MemoryRequest"`
	CloudProject       string          `json:"CloudProject"`
	Region             string          `json:"Region"`
	Magento            MagentoOverlays `json:"Magento"`
}

var deployInputsDigestPattern = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

// Validate checks the deploy contract before execution.
func (s DeployInputs) Validate() error {
	var problems []error
	if !deployInputsDigestPattern.MatchString(s.ImageDigest) {
		problems = append(problems, errors.New("image digest must be repository@sha256:..."))
	}
	if strings.TrimSpace(s.DatabaseName) == "" {
		problems = append(problems, errors.New("database name is required"))
	}
	return errors.Join(problems...)
}

// DeployAppPhase names one provider-executed deploy phase. The core drives
// phases in flow order; the plugin owns what each phase does.
type DeployAppPhase string

const (
	DeployPhaseRegister  DeployAppPhase = "register"
	DeployPhaseMigrate   DeployAppPhase = "migrate"
	DeployPhaseCleanup   DeployAppPhase = "cleanup"
	DeployPhaseStabilize DeployAppPhase = "stabilize"
	DeployPhaseHealth    DeployAppPhase = "health"
)

// Valid reports whether the phase names a known deploy step.
func (p DeployAppPhase) Valid() bool {
	switch p {
	case DeployPhaseRegister, DeployPhaseMigrate, DeployPhaseCleanup, DeployPhaseStabilize, DeployPhaseHealth:
		return true
	default:
		return false
	}
}

// DeployAppPhaseCall executes one deploy phase inside the provider.
// OutputsJSON carries the stack outputs the phase needs (fetched once by
// the core); StateJSON carries the opaque candidate handle between
// register/migrate/cleanup. The server decodes the stored plan's deploy
// inputs itself; the core never interprets them.
type DeployAppPhaseCall struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Envelope        Envelope       `json:"envelope"`
	Plan            StoredPlan     `json:"plan"`
	Phase           DeployAppPhase `json:"phase"`
	ImageDigest     string         `json:"imageDigest"`
	OutputsJSON     []byte         `json:"outputsJson,omitempty"`
	StateJSON       []byte         `json:"stateJson,omitempty"`
}

// DeployAppPhaseResult carries the phase outcome. Message is a human
// diagnostic line the core prints; StateJSON feeds the next phase.
type DeployAppPhaseResult struct {
	StateJSON []byte          `json:"stateJson,omitempty"`
	Message   string          `json:"message,omitempty"`
	Error     *OperationError `json:"error,omitempty"`
}

// MediaTransferCall moves the media tree between the provider bucket and
// operator disk. OutputsJSON carries the stack outputs (the media bucket
// name); LocalDir is an operator-local directory the plugin reads or
// writes. Direction is implied by the operation.
type MediaTransferCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	OutputsJSON     []byte     `json:"outputsJson,omitempty"`
	LocalDir        string     `json:"localDir"`
}

// MediaTransferResult reports a completed transfer.
type MediaTransferResult struct {
	FileCount int64           `json:"fileCount"`
	ByteCount int64           `json:"byteCount"`
	LocalDir  string          `json:"localDir"`
	Error     *OperationError `json:"error,omitempty"`
}
