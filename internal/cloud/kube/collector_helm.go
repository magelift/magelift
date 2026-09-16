package kube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// HelmCollectorRelease is the provider-neutral inventory view returned by an
// injected Helm implementation. Chart values, rendered manifests, and Helm
// SDK objects stay behind HelmCollectorAPI.
type HelmCollectorRelease struct {
	Namespace       string
	Name            string
	ChartReference  string
	ChartVersion    string
	Distribution    string
	ConfigurationID string
	OwnershipMarker string
	Ready           bool
	Healthy         bool
}

// HelmCollectorReleaseRequest contains only semantic deployment intent. A
// provider-owned Helm implementation maps it to chart-specific values and RBAC
// while resolving CredentialRef inside its own trust boundary.
type HelmCollectorReleaseRequest struct {
	TargetProvider  sdk.ProviderID
	TargetRuntime   sdk.RuntimeID
	NativeReference string
	Namespace       string
	Name            string
	ChartReference  string
	ChartVersion    string
	Distribution    string
	Endpoint        string
	CredentialRef   string
	Signals         []string
	OwnershipMarker string
	ConfigurationID string
}

// HelmCollectorAPI is the narrow release lifecycle port. Implementations may
// use the official Helm Go SDK, a provider-managed Helm service, or a community
// adapter, but must return normalized release state and never secret values.
type HelmCollectorAPI interface {
	GetRelease(context.Context, string, string) (HelmCollectorRelease, bool, error)
	InstallRelease(context.Context, HelmCollectorReleaseRequest) (HelmCollectorRelease, error)
	UninstallRelease(context.Context, string, string) error
}

// HelmCollectorConfig selects one owned release and one immutable chart. The
// chart-specific values/RBAC implementation belongs to HelmCollectorAPI.
type HelmCollectorConfig struct {
	Namespace      string
	ReleaseName    string
	ChartReference string
	ChartVersion   string
	API            HelmCollectorAPI
	Probe          CollectorSignalProbe
}

// HelmCollectorBackend implements the shared CollectorBackend port for
// provider-owned Helm releases. It deliberately installs only missing releases
// and refuses configuration drift on an existing release: changing the marker
// creates an explicit, auditable lifecycle scope and makes rollback safe.
type HelmCollectorBackend struct {
	api            HelmCollectorAPI
	namespace      string
	releaseName    string
	chartReference string
	chartVersion   string
	probe          CollectorSignalProbe
}

var _ providerobservability.CollectorBackend = (*HelmCollectorBackend)(nil)

func NewHelmCollectorBackend(config HelmCollectorConfig) (*HelmCollectorBackend, error) {
	if config.API == nil {
		return nil, errors.New("Helm collector API is required")
	}
	namespace := strings.TrimSpace(config.Namespace)
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	if errs := validation.IsDNS1123Subdomain(namespace); len(errs) > 0 {
		return nil, fmt.Errorf("Helm collector namespace is invalid: %s", strings.Join(errs, "; "))
	}
	releaseName := strings.TrimSpace(config.ReleaseName)
	if releaseName == "" || len(validation.IsDNS1123Subdomain(releaseName)) > 0 || strings.ContainsAny(releaseName, "/\r\n\x00") {
		return nil, errors.New("Helm collector release name must be a valid DNS name")
	}
	chartReference := strings.TrimSpace(config.ChartReference)
	if chartReference == "" || strings.ContainsAny(chartReference, "\r\n\x00") {
		return nil, errors.New("Helm collector chart reference is required and must be single-line")
	}
	chartVersion := strings.TrimSpace(config.ChartVersion)
	if chartVersion == "" || strings.EqualFold(chartVersion, "latest") || strings.ContainsAny(chartVersion, " \t\r\n\x00") {
		return nil, errors.New("Helm collector chart version must be immutable and single-line")
	}
	if config.Probe == nil {
		return nil, errors.New("Helm collector signal probe is required")
	}
	return &HelmCollectorBackend{api: config.API, namespace: namespace, releaseName: releaseName, chartReference: chartReference, chartVersion: chartVersion, probe: config.Probe}, nil
}

func (backend *HelmCollectorBackend) Find(ctx context.Context, plan sdk.CollectorDeploymentPlan) (providerobservability.CollectorResource, bool, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorResource{}, false, err
	}
	release, found, err := backend.api.GetRelease(ctx, backend.namespace, backend.releaseName)
	if err != nil {
		return providerobservability.CollectorResource{}, false, fmt.Errorf("inspect Helm collector release: %w", err)
	}
	if !found {
		return providerobservability.CollectorResource{}, false, nil
	}
	return backend.resource(release, plan), true, nil
}

func (backend *HelmCollectorBackend) Ensure(ctx context.Context, plan sdk.CollectorDeploymentPlan) (providerobservability.CollectorResource, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorResource{}, err
	}
	release, found, err := backend.api.GetRelease(ctx, backend.namespace, backend.releaseName)
	if err != nil {
		return providerobservability.CollectorResource{}, fmt.Errorf("inspect Helm collector release before install: %w", err)
	}
	configurationID := helmCollectorConfigurationID(plan, backend.chartReference, backend.chartVersion)
	if found {
		if release.OwnershipMarker != plan.OwnershipMarker {
			return providerobservability.CollectorResource{}, errors.New("refusing to mutate an unowned Helm collector release")
		}
		if !backend.releaseMatches(release, plan, configurationID) {
			return providerobservability.CollectorResource{}, errors.New("refusing to mutate an existing Helm collector release with configuration drift; use a new ownership marker")
		}
		return backend.resource(release, plan), nil
	}
	installed, err := backend.api.InstallRelease(ctx, HelmCollectorReleaseRequest{
		TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, NativeReference: plan.NativeReference,
		Namespace: backend.namespace, Name: backend.releaseName, ChartReference: backend.chartReference, ChartVersion: backend.chartVersion,
		Distribution: plan.Distribution, Endpoint: plan.Endpoint, CredentialRef: plan.CredentialRef, Signals: append([]string(nil), plan.Signals...),
		OwnershipMarker: plan.OwnershipMarker, ConfigurationID: configurationID,
	})
	if err != nil {
		return providerobservability.CollectorResource{}, fmt.Errorf("install Helm collector release: %w", err)
	}
	if installed.Namespace == "" {
		installed.Namespace = backend.namespace
	}
	if installed.Name == "" {
		installed.Name = backend.releaseName
	}
	if !backend.releaseMatches(installed, plan, configurationID) {
		return providerobservability.CollectorResource{}, errors.New("Helm collector install did not return exact ownership and configuration proof")
	}
	return backend.resource(installed, plan), nil
}

func (backend *HelmCollectorBackend) Verify(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) (providerobservability.CollectorVerification, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorVerification{}, err
	}
	release, found, err := backend.api.GetRelease(ctx, backend.namespace, backend.releaseName)
	if err != nil {
		return providerobservability.CollectorVerification{}, fmt.Errorf("inspect Helm collector release for verification: %w", err)
	}
	if !found || !resource.Owned || release.OwnershipMarker != plan.OwnershipMarker || resource.Identity != helmReleaseIdentity(backend.namespace, backend.releaseName) {
		return providerobservability.CollectorVerification{Reason: "Helm collector release is missing or ownership changed"}, nil
	}
	if !backend.releaseMatches(release, plan, helmCollectorConfigurationID(plan, backend.chartReference, backend.chartVersion)) {
		return providerobservability.CollectorVerification{Reason: "Helm collector release configuration changed"}, nil
	}
	verification := providerobservability.CollectorVerification{Ready: release.Ready, Healthy: release.Healthy}
	if !verification.Ready {
		verification.Reason = "Helm collector release is not ready"
		return verification, nil
	}
	if !verification.Healthy {
		verification.Reason = "Helm collector release is not healthy"
		return verification, nil
	}
	delivered, err := backend.probe.VerifyCollectorSignals(ctx, plan, resource)
	if err != nil {
		return providerobservability.CollectorVerification{}, fmt.Errorf("verify Helm collector signal delivery: %w", err)
	}
	verification.SignalsDelivered = delivered
	if !delivered {
		verification.Reason = "Helm collector signal probe did not verify delivery"
	}
	return verification, nil
}

func (backend *HelmCollectorBackend) Rollback(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	return backend.deleteOwned(ctx, plan, resource)
}

func (backend *HelmCollectorBackend) Destroy(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	return backend.deleteOwned(ctx, plan, resource)
}

func (backend *HelmCollectorBackend) Inventory(ctx context.Context, provider sdk.ProviderID, runtime sdk.RuntimeID, marker string) ([]providerobservability.CollectorResource, error) {
	if backend == nil || backend.api == nil {
		return nil, errors.New("Helm collector backend is required")
	}
	if ctx == nil {
		return nil, errors.New("Helm collector inventory context is required")
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("Helm collector ownership marker is required")
	}
	release, found, err := backend.api.GetRelease(ctx, backend.namespace, backend.releaseName)
	if err != nil {
		return nil, fmt.Errorf("inventory Helm collector release: %w", err)
	}
	if !found {
		return nil, nil
	}
	plan := sdk.CollectorDeploymentPlan{TargetProvider: provider, TargetRuntime: runtime, Workload: "kubernetes", Distribution: release.Distribution, OwnershipMarker: marker}
	return []providerobservability.CollectorResource{backend.resource(release, plan)}, nil
}

func (backend *HelmCollectorBackend) deleteOwned(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return err
	}
	if !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker || resource.Identity != helmReleaseIdentity(backend.namespace, backend.releaseName) {
		return errors.New("refusing Helm collector cleanup without exact ownership")
	}
	release, found, err := backend.api.GetRelease(ctx, backend.namespace, backend.releaseName)
	if err != nil {
		return fmt.Errorf("inspect Helm collector release before cleanup: %w", err)
	}
	if !found {
		return nil
	}
	if release.OwnershipMarker != plan.OwnershipMarker {
		return errors.New("refusing Helm collector cleanup after ownership changed")
	}
	if !backend.releaseMatches(release, plan, helmCollectorConfigurationID(plan, backend.chartReference, backend.chartVersion)) {
		return errors.New("refusing Helm collector cleanup after configuration changed")
	}
	if err := backend.api.UninstallRelease(ctx, backend.namespace, backend.releaseName); err != nil {
		return fmt.Errorf("uninstall Helm collector release: %w", err)
	}
	if err := waitForCollectorResourcesGone(ctx, func(checkCtx context.Context) (bool, error) {
		release, found, err := backend.api.GetRelease(checkCtx, backend.namespace, backend.releaseName)
		if err != nil {
			return false, err
		}
		if !found {
			return true, nil
		}
		if release.OwnershipMarker != plan.OwnershipMarker {
			return false, errors.New("refusing Helm collector cleanup after ownership changed")
		}
		if !backend.releaseMatches(release, plan, helmCollectorConfigurationID(plan, backend.chartReference, backend.chartVersion)) {
			return false, errors.New("refusing Helm collector cleanup after configuration changed")
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("verify Helm collector cleanup: %w", err)
	}
	return nil
}

func (backend *HelmCollectorBackend) validatePlan(ctx context.Context, plan sdk.CollectorDeploymentPlan) error {
	if backend == nil || backend.api == nil {
		return errors.New("Helm collector backend is required")
	}
	if ctx == nil {
		return errors.New("Helm collector context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("Helm collector ownership marker is required")
	}
	if strings.TrimSpace(plan.CredentialRef) == "" {
		return errors.New("Helm collector credential reference is required")
	}
	if err := sdk.ValidateCredentialReference(plan.CredentialRef); err != nil {
		return fmt.Errorf("Helm collector credential reference: %w", err)
	}
	if strings.TrimSpace(plan.Endpoint) == "" {
		return errors.New("Helm collector endpoint is required")
	}
	if strings.TrimSpace(plan.Distribution) == "" {
		return errors.New("Helm collector distribution is required")
	}
	return nil
}

func (backend *HelmCollectorBackend) resource(release HelmCollectorRelease, plan sdk.CollectorDeploymentPlan) providerobservability.CollectorResource {
	owned := release.OwnershipMarker == plan.OwnershipMarker
	return providerobservability.CollectorResource{
		Identity: helmReleaseIdentity(backend.namespace, backend.releaseName), TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime,
		Workload: plan.Workload, Distribution: plan.Distribution, OwnershipMarker: release.OwnershipMarker, Status: helmReleaseStatus(release), Owned: owned,
	}
}

func (backend *HelmCollectorBackend) releaseMatches(release HelmCollectorRelease, plan sdk.CollectorDeploymentPlan, configurationID string) bool {
	return release.Namespace == backend.namespace && release.Name == backend.releaseName &&
		release.ChartReference == backend.chartReference && release.ChartVersion == backend.chartVersion &&
		release.Distribution == plan.Distribution && release.OwnershipMarker == plan.OwnershipMarker &&
		release.ConfigurationID == configurationID
}

func helmReleaseIdentity(namespace, name string) string { return "helm:" + namespace + "/" + name }

func helmReleaseStatus(release HelmCollectorRelease) string {
	if release.Ready && release.Healthy {
		return "ready"
	}
	return "pending"
}

func helmCollectorConfigurationID(plan sdk.CollectorDeploymentPlan, chartReference, chartVersion string) string {
	hash := sha256.New()
	for _, value := range []string{
		chartReference, chartVersion, string(plan.TargetProvider), string(plan.TargetRuntime), plan.NativeReference,
		plan.Distribution, plan.Endpoint, plan.CredentialRef, strings.Join(plan.Signals, "\x00"),
	} {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{'\x00'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
