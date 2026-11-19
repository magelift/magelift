package sdk

import (
	"bytes"
	"encoding/json"
	"errors"
)

// EdgeRPCOpaqueVersion is the minor wire version that adds bound opaque plan
// state while preserving the original top-level plan and request fields.
const EdgeRPCOpaqueVersion = 1

// EdgeRPCBinding ties transient adapter state to the already validated
// provider stack that owns the RPC session.
type EdgeRPCBinding struct {
	Provider    ProviderID `json:"provider"`
	Runtime     RuntimeID  `json:"runtime"`
	StackName   string     `json:"stackName"`
	Account     string     `json:"account"`
	Environment string     `json:"environment"`
}

// BoundEdgeOpaque retains the binding returned with a provider plan while it
// remains in the host process. Hosts unwrap State only when sending it back
// to the same verified stack for execution.
type BoundEdgeOpaque struct {
	Binding EdgeRPCBinding
	State   json.RawMessage
}

// EdgeRPCBindingFor derives the RPC binding from the verified provider plan.
func EdgeRPCBindingFor(envelope Envelope, plan StoredPlan) EdgeRPCBinding {
	return EdgeRPCBinding{Provider: ProviderID(plan.Provider), Runtime: RuntimeID(plan.Runtime), StackName: plan.StackName, Account: envelope.Account, Environment: envelope.Environment}
}

// ValidateFor rejects adapter state replayed against another verified
// provider, runtime, account, stack, or environment.
func (b EdgeRPCBinding) ValidateFor(envelope Envelope, plan StoredPlan) error {
	want := EdgeRPCBindingFor(envelope, plan)
	if b != want || b.Provider == "" || b.Runtime == "" || b.StackName == "" || b.Environment == "" {
		return errors.New("edge RPC binding does not match the verified provider stack")
	}
	return nil
}

type edgePlanRPC struct {
	EdgePlan
	RPCVersion int             `json:"rpcVersion,omitempty"`
	Binding    EdgeRPCBinding  `json:"binding,omitempty"`
	Opaque     json.RawMessage `json:"opaque,omitempty"`
}

// EncodeEdgePlanRPC encodes an edge plan and its transient adapter state. The
// plan fields remain at the top level for compatibility with older plugins.
func EncodeEdgePlanRPC(plan EdgePlan, binding EdgeRPCBinding) ([]byte, error) {
	var opaque json.RawMessage
	if plan.Opaque != nil {
		encoded, err := json.Marshal(plan.Opaque)
		if err != nil {
			return nil, err
		}
		opaque = encoded
	}
	return json.Marshal(edgePlanRPC{EdgePlan: plan, RPCVersion: EdgeRPCOpaqueVersion, Binding: binding, Opaque: opaque})
}

// DecodeEdgePlanRPC decodes current RPC state and accepts the original
// portable-only plan shape. A zero returned version identifies that legacy
// shape so callers can fail clearly when the plan needs opaque state.
func DecodeEdgePlanRPC(data []byte) (plan EdgePlan, binding EdgeRPCBinding, version int, err error) {
	var wire edgePlanRPC
	if err = json.Unmarshal(data, &wire); err != nil {
		return EdgePlan{}, EdgeRPCBinding{}, 0, err
	}
	if wire.RPCVersion == EdgeRPCOpaqueVersion {
		plan = wire.EdgePlan
		if plan.AdapterID == "" || wire.Binding.Provider == "" || wire.Binding.Runtime == "" || wire.Binding.StackName == "" || wire.Binding.Environment == "" {
			return EdgePlan{}, EdgeRPCBinding{}, 0, errors.New("edge plan RPC state has incomplete binding")
		}
		plan.Opaque = json.RawMessage(bytes.Clone(wire.Opaque))
		return plan, wire.Binding, wire.RPCVersion, nil
	}
	if wire.RPCVersion != 0 {
		return EdgePlan{}, EdgeRPCBinding{}, 0, errors.New("unsupported edge RPC version")
	}
	var legacy EdgePlan
	if err = json.Unmarshal(data, &legacy); err != nil {
		return EdgePlan{}, EdgeRPCBinding{}, 0, err
	}
	return legacy, EdgeRPCBinding{}, 0, nil
}

type edgeExecutionRPC struct {
	EdgeExecutionRequest
	RPCVersion int             `json:"rpcVersion,omitempty"`
	Binding    EdgeRPCBinding  `json:"binding,omitempty"`
	PlanOpaque json.RawMessage `json:"planOpaque,omitempty"`
}

// EncodeEdgeExecutionRPC carries the provider state attached to the
// in-memory plan being executed. The public request stays flattened and no
// provider state is added to journals or checkpoints.
func EncodeEdgeExecutionRPC(request EdgeExecutionRequest, binding EdgeRPCBinding) ([]byte, error) {
	var opaque json.RawMessage
	if request.Plan.Opaque != nil {
		encoded, err := json.Marshal(request.Plan.Opaque)
		if err != nil {
			return nil, err
		}
		opaque = encoded
	}
	return json.Marshal(edgeExecutionRPC{EdgeExecutionRequest: request, RPCVersion: EdgeRPCOpaqueVersion, Binding: binding, PlanOpaque: opaque})
}

// DecodeEdgeExecutionRPC restores provider-owned state for the immediate
// operation and accepts the original portable-only request shape.
func DecodeEdgeExecutionRPC(data []byte) (request EdgeExecutionRequest, binding EdgeRPCBinding, version int, err error) {
	var wire edgeExecutionRPC
	if err = json.Unmarshal(data, &wire); err != nil {
		return EdgeExecutionRequest{}, EdgeRPCBinding{}, 0, err
	}
	if wire.RPCVersion == EdgeRPCOpaqueVersion {
		request = wire.EdgeExecutionRequest
		if request.Plan.AdapterID == "" || wire.Binding.Provider == "" || wire.Binding.Runtime == "" || wire.Binding.StackName == "" || wire.Binding.Environment == "" {
			return EdgeExecutionRequest{}, EdgeRPCBinding{}, 0, errors.New("edge execution RPC state has incomplete binding")
		}
		request.Plan.Opaque = json.RawMessage(bytes.Clone(wire.PlanOpaque))
		return request, wire.Binding, wire.RPCVersion, nil
	}
	if wire.RPCVersion != 0 {
		return EdgeExecutionRequest{}, EdgeRPCBinding{}, 0, errors.New("unsupported edge RPC version")
	}
	var legacy EdgeExecutionRequest
	if err = json.Unmarshal(data, &legacy); err != nil {
		return EdgeExecutionRequest{}, EdgeRPCBinding{}, 0, err
	}
	return legacy, EdgeRPCBinding{}, 0, nil
}
