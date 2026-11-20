package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/providers/aws/eksops"
	"github.com/magelift/magelift/providers/aws/stack"
	"github.com/magelift/magelift/sdk"
	"gopkg.in/yaml.v3"
)

type stored struct {
	Envelope    sdk.Envelope
	ECS         *stack.Spec  `json:",omitempty"`
	EKS         *eksops.Spec `json:",omitempty"`
	AccountOnly bool         `json:",omitempty"`
	DestroyOnly bool         `json:",omitempty"`
	ObserveOnly bool         `json:",omitempty"`
	Admitted    bool         `json:",omitempty"`
}

func parseTarget(data []byte) (*config.AWSTarget, error) {
	if len(data) == 0 {
		return nil, errors.New("target.aws is required")
	}
	var target *config.AWSTarget
	if err := yaml.Unmarshal(data, &target); err != nil || target == nil {
		return nil, errors.New("invalid AWS target block")
	}
	return target, nil
}

func (s *Server) ValidateConfig(req *sdk.ValidateConfigRequest, resp *sdk.ValidateConfigResult) error {
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	_, err := module(req.Runtime)
	if err == nil {
		var target *config.AWSTarget
		target, err = parseTarget(req.TargetBlock)
		if err == nil {
			resp.Problems = config.ValidateAWSTarget(config.Config{Target: config.Target{Provider: "aws", Runtime: req.Runtime, AWS: target}})
		}
	}
	if err != nil {
		resp.Problems = []string{err.Error()}
	}
	resp.Valid = len(resp.Problems) == 0
	return nil
}

func projectConfig(req *sdk.PlanRequest, target *config.AWSTarget) config.Config {
	e, app, m, email := req.Envelope, req.Application, req.Application.Magento, req.Application.Email
	cfg := config.Config{
		Project: config.Project{Name: e.Project}, StateEnvironment: req.StateEnvironment, Account: e.Account,
		Class: e.EnvironmentClass, Protection: e.Protected, Domain: e.Domain, ExpiresAt: e.ExpiresAt, MonthlyBudgetCents: e.MonthlyBudgetCents,
		Target: config.Target{Provider: "aws", Runtime: req.Runtime, AWS: target}, Defaults: config.Defaults{Region: e.Region, Preset: e.Preset},
		Application: config.Application{Edition: app.Edition, Version: app.Version, Mode: app.Mode, WebRuntime: app.WebRuntime,
			Magento: config.MagentoRuntime{FrontName: m.FrontName, CookieDomain: m.CookieDomain, SecureBaseURL: m.SecureBaseURL, UnsecureBaseURL: m.UnsecureBaseURL,
				CORSOrigins: m.CORSOrigins, StorefrontOrigin: m.StorefrontOrigin, Consumers: config.MagentoConsumers{Mode: m.ConsumersMode, Names: m.ConsumerNames},
				QueueTransport: m.QueueTransport, QueueModule: m.QueueModule, Variables: m.Variables}},
		Email:    config.EmailConfig{Mode: email.Mode, Host: email.Host, Port: email.Port, Username: email.Username, From: email.From, Credential: email.Credential},
		Services: req.Services,
	}
	cfg.Edge.NativeProvider = req.Edge.NativeProvider
	cfg.Edge.ExternalProvider = req.Edge.ExternalProvider
	cfg.Edge.Mode = req.Edge.Mode
	cfg.Observability.NativeProvider = req.Observability.NativeProvider
	cfg.Observability.ExternalProvider = req.Observability.ExternalProvider
	if email.ManagedDomain != "" || email.ManagedHostedZoneID != "" {
		cfg.Email.Managed = &config.EmailManaged{Domain: email.ManagedDomain, HostedZoneID: email.ManagedHostedZoneID}
	}
	return cfg
}

func (s *Server) Plan(req *sdk.PlanRequest, resp *sdk.PlanResult) error {
	return respond(req.ProtocolVersion, &resp.Error, 5*time.Minute, func(ctx context.Context) error {
		m, err := module(req.Runtime)
		if err != nil {
			return invalid(err)
		}
		target, err := parseTarget(req.TargetBlock)
		if err != nil {
			return invalid(err)
		}
		cfg := projectConfig(req, target)
		if !req.AccountOnly && !req.AllowExpiredPreview {
			if err := errors.Join(platform.ValidateFirstPartyEdge(cfg), platform.ValidateFirstPartyObservability(cfg)); err != nil {
				return invalid(err)
			}
			if req.Runtime == "eks" && req.Edge.NativeProvider != "" {
				return invalid(errors.New("EKS does not manage a native edge; configure the Kubernetes ingress or an external edge"))
			}
		}
		if problems := config.ValidateAWSTarget(cfg); len(problems) > 0 {
			return invalid(errors.New(strings.Join(problems, ", ")))
		}
		body := stored{Envelope: req.Envelope, AccountOnly: req.AccountOnly, DestroyOnly: req.AllowExpiredPreview, ObserveOnly: req.RuntimeObserve}
		var planned platform.PlannedStack
		if req.Runtime == "ecs-fargate" && !req.AccountOnly && !req.AllowExpiredPreview {
			spec, err := stack.PlanFromConfigWithOptions(cfg, req.Envelope.Environment, stack.PlanOptions{RuntimeObserve: req.RuntimeObserve, Intents: &stack.ResolvedIntents{Edge: req.Edge, Observability: req.Observability, UseDefaultEdge: req.UseProviderDefaultEdge}})
			if err != nil {
				return invalid(err)
			}
			planned = stack.Planned{Spec: spec}
		} else {
			planned, err = m.Plan(cfg, req.Envelope.Environment, platform.PlanOptions{Context: ctx, AllowExpiredPreview: req.AllowExpiredPreview, AccountOnly: req.AccountOnly, RuntimeObserve: req.RuntimeObserve})
			if err != nil {
				return invalid(err)
			}
		}
		if value, ok := planned.(eksops.Planned); ok && req.LiveQueueReplicas != 0 {
			planned, err = value.WithLiveQueueReplicas(req.LiveQueueReplicas)
			if err != nil {
				return invalid(err)
			}
		}
		if req.Envelope.StackName != planned.StackName() {
			return invalid(errors.New("AWS stack name does not match its project and environment"))
		}
		if req.Admit {
			if body.AccountOnly || body.DestroyOnly || body.ObserveOnly {
				return invalid(errors.New("AWS admission requires a deployment plan"))
			}
			admit := s.admit
			if admit == nil {
				admit = platform.AdmitPlan
			}
			planned, err = admit(ctx, m, planned)
			if err != nil {
				return err
			}
			body.Admitted = true
		}
		switch value := planned.(type) {
		case stack.Planned:
			body.ECS = &value.Spec
		case eksops.Planned:
			body.EKS = &value.Spec
		default:
			return errors.New("unknown AWS stack plan")
		}
		opaque, err := json.Marshal(body)
		if err != nil {
			return err
		}
		backend := planned.(interface{ StateBackendURL() string }).StateBackendURL()
		resp.Plan = sdk.StoredPlan{Provider: "aws", Runtime: req.Runtime, StackName: planned.StackName(), Tier: sdk.ExtensionTier(planned.RuntimeTier()), ImageDigest: planned.ImageDigest(), StateBackendURL: backend, Opaque: opaque}
		if body.ECS != nil {
			resp.Plan.Adopted = stack.AdoptReport(*body.ECS)
		}
		return nil
	})
}

func checked(envelope sdk.Envelope, plan sdk.StoredPlan) (stored, platform.StackModule, platform.PlannedStack, error) {
	var b stored
	if err := json.Unmarshal(plan.Opaque, &b); err != nil || b.Envelope != envelope || plan.Provider != "aws" || strings.TrimSpace(envelope.Account) == "" {
		return b, nil, nil, integrity("AWS plan does not belong to the requested environment")
	}
	m, err := module(plan.Runtime)
	if err != nil {
		return b, nil, nil, integrity(err.Error())
	}
	var p platform.PlannedStack
	switch {
	case b.ECS != nil && b.EKS == nil && plan.Runtime == "ecs-fargate":
		p = stack.Planned{Spec: *b.ECS}
	case b.EKS != nil && b.ECS == nil && plan.Runtime == "eks":
		p = eksops.Planned{Spec: *b.EKS}
	default:
		return b, nil, nil, integrity("AWS stored plan has a different runtime")
	}
	var identity stack.Identity
	if b.ECS != nil {
		identity = b.ECS.Identity
	} else {
		identity = b.EKS.Identity
	}
	var adopted []sdk.AdoptedResource
	if b.ECS != nil {
		adopted = stack.AdoptReport(*b.ECS)
	}
	if !slices.Equal(adopted, plan.Adopted) {
		return b, nil, nil, integrity("attached resources differ from the planned stack")
	}
	if identity.Project != envelope.Project || identity.Environment != envelope.Environment ||
		identity.Region != envelope.Region || identity.AccountID != envelope.Account ||
		identity.EnvironmentClass != envelope.EnvironmentClass || string(identity.Preset) != envelope.Preset ||
		p.Protected() != envelope.Protected {
		return b, nil, nil, integrity("AWS stored plan identity differs from the requested environment")
	}
	if p.StackName() != plan.StackName || plan.StackName != envelope.StackName ||
		plan.Tier != sdk.ExtensionTier(p.RuntimeTier()) ||
		p.ImageDigest() != plan.ImageDigest || p.(interface{ StateBackendURL() string }).StateBackendURL() != plan.StateBackendURL {
		return b, nil, nil, integrity("AWS stored plan metadata differs from the planned stack")
	}
	return b, m, p, nil
}

func withPlan(version string, failure **sdk.OperationError, envelope sdk.Envelope, plan sdk.StoredPlan, run func(context.Context, platform.StackModule, platform.PlannedStack) error) error {
	return respond(version, failure, 45*time.Minute, func(ctx context.Context) error {
		_, m, p, err := checked(envelope, plan)
		if err != nil {
			return err
		}
		return run(ctx, m, p)
	})
}
