package recovery

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/magelift/magelift/sdk"
)

const OperationVersion = 1

// OperationState is the provider-neutral durable identity carried by a
// resumable recovery operation. Provider prefixes identify the implementation;
// the payload contains no SDK model or secret value.
type OperationState struct {
	Version           int                     `json:"version"`
	Action            sdk.ResilienceAction    `json:"action"`
	DataClass         string                  `json:"dataClass"`
	Resource          string                  `json:"resource,omitempty"`
	Backup            string                  `json:"backup,omitempty"`
	Destination       sdk.RecoveryDestination `json:"destination,omitempty"`
	FixtureID         string                  `json:"fixtureId"`
	OwnershipMarker   string                  `json:"ownershipMarker"`
	IdempotencyKey    string                  `json:"idempotencyKey"`
	ApprovalReference string                  `json:"approvalReference,omitempty"`
	Target            string                  `json:"target,omitempty"`
	OperationRef      string                  `json:"operationRef,omitempty"`
	ResourceRef       string                  `json:"resourceRef,omitempty"`
}

func EncodeOperationID(prefix string, state OperationState) (string, error) {
	if strings.TrimSpace(prefix) == "" || strings.ContainsAny(prefix, "\r\n\x00") {
		return "", errors.New("recovery operation prefix is required")
	}
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(data), nil
}

func DecodeOperationID(prefix, operationID string) (OperationState, error) {
	if !strings.HasPrefix(operationID, prefix) {
		return OperationState{}, errors.New("recovery operation identity has an unsupported format")
	}
	data, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(operationID, prefix))
	if err != nil {
		return OperationState{}, errors.New("recovery operation identity is not valid base64")
	}
	var state OperationState
	if err := json.Unmarshal(data, &state); err != nil {
		return OperationState{}, errors.New("recovery operation identity is not valid JSON")
	}
	if state.Version != OperationVersion || state.Action == "" || strings.TrimSpace(state.DataClass) == "" || strings.TrimSpace(state.OwnershipMarker) == "" || strings.TrimSpace(state.IdempotencyKey) == "" {
		return OperationState{}, errors.New("recovery operation identity is incomplete")
	}
	return state, nil
}
