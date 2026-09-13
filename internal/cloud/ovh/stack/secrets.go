package stack

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/cloud/ovh/resilience"
	"github.com/magelift/magelift/internal/platform"
)

const (
	applicationSecretValueLimit = 64 * 1024
	applicationSecretRoot       = "magelift/application-secrets"
	secretManagedByKey          = "magelift-managed-by"
	secretManagedByValue        = "magelift"
	secretDataClassKey          = "magelift-data-class"
	secretDataClassValue        = "application-secrets"
	secretOwnerKey              = "magelift-owner"
)

var applicationSecretPathSegment = regexp.MustCompile(`^[A-Za-z0-9_+=.@-]+$`)

// ApplicationSecretClientFactory constructs the provider-local OKMS port for
// one planned OVH target. The factory owns authentication and must resolve
// credentials only inside the provider boundary; secret values never belong in
// the planned stack or the factory's returned metadata.
type ApplicationSecretClientFactory func(context.Context, platform.PlannedStack) (resilience.SecretAPI, error)

// Secrets adapts the provider-local OVHcloud Secret Manager port to the exact
// platform.Secrets contract. Secret paths are scoped to one validated stack
// and ownership metadata prevents this adapter from touching foreign secrets.
type Secrets struct {
	newClient ApplicationSecretClientFactory
}

var _ platform.Secrets = (*Secrets)(nil)

// NewSecrets constructs an application-secret adapter around an injected
// provider-local OKMS boundary. It performs no authentication or cloud call.
func NewSecrets(newClient ApplicationSecretClientFactory) *Secrets {
	return &Secrets{newClient: newClient}
}

func (s *Secrets) List(ctx context.Context, planned platform.PlannedStack) ([]platform.SecretMeta, error) {
	client, prefix, err := s.resolve(ctx, planned)
	if err != nil {
		return nil, err
	}
	owned, err := listOwnedApplicationSecrets(ctx, client, prefix)
	if err != nil {
		return nil, fmt.Errorf("list OVHcloud application secrets: %w", err)
	}
	result := make([]platform.SecretMeta, 0, len(owned))
	for _, secret := range owned {
		result = append(result, platform.SecretMeta{Name: secret.name})
	}
	return result, nil
}

func (s *Secrets) Set(ctx context.Context, planned platform.PlannedStack, name string, value []byte) error {
	if err := validateApplicationSecretName(name); err != nil {
		return err
	}
	if len(value) == 0 {
		return errors.New("application secret value is required")
	}
	if len(value) > applicationSecretValueLimit {
		return fmt.Errorf("application secret value exceeds %d-byte limit", applicationSecretValueLimit)
	}
	client, prefix, err := s.resolve(ctx, planned)
	if err != nil {
		return err
	}
	owned, err := listOwnedApplicationSecrets(ctx, client, prefix)
	if err != nil {
		return fmt.Errorf("inspect OVHcloud application secrets before update: %w", err)
	}
	path := prefix + "/" + name
	for _, secret := range owned {
		if secret.path != path {
			continue
		}
		updated, updateErr := client.CreateVersion(ctx, path, value)
		if updateErr != nil {
			return fmt.Errorf("create OVHcloud application secret version: %w", updateErr)
		}
		if !isOwnedApplicationSecret(updated, prefix) {
			return errors.New("OVHcloud application secret update did not prove ownership")
		}
		return nil
	}

	created, err := client.Create(ctx, path, applicationSecretMetadata(prefix), value)
	if err != nil {
		return fmt.Errorf("create OVHcloud application secret: %w", err)
	}
	if !isOwnedApplicationSecret(created, prefix) {
		return errors.New("OVHcloud application secret creation did not prove ownership")
	}
	return nil
}

func (s *Secrets) Remove(ctx context.Context, planned platform.PlannedStack, name string) error {
	if err := validateApplicationSecretName(name); err != nil {
		return err
	}
	client, prefix, err := s.resolve(ctx, planned)
	if err != nil {
		return err
	}
	owned, err := listOwnedApplicationSecrets(ctx, client, prefix)
	if err != nil {
		return fmt.Errorf("inspect OVHcloud application secrets before removal: %w", err)
	}
	path := prefix + "/" + name
	for _, secret := range owned {
		if secret.path != path {
			continue
		}
		if err := client.Delete(ctx, path); err != nil {
			return fmt.Errorf("delete OVHcloud application secret: %w", err)
		}
		return nil
	}
	return errors.New("OVHcloud application secret was not found or is not owned by this stack")
}

type ownedApplicationSecretEntry struct {
	path string
	name string
}

func (s *Secrets) resolve(ctx context.Context, planned platform.PlannedStack) (resilience.SecretAPI, string, error) {
	if s == nil || s.newClient == nil {
		return nil, "", errors.New("OVHcloud application Secret Manager client factory is not configured")
	}
	if ctx == nil {
		return nil, "", errors.New("OVHcloud application Secret Manager context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	prefix, err := applicationSecretPrefix(planned)
	if err != nil {
		return nil, "", err
	}
	client, err := s.newClient(ctx, planned)
	if err != nil {
		return nil, "", fmt.Errorf("construct OVHcloud application Secret Manager client: %w", err)
	}
	if client == nil {
		return nil, "", errors.New("OVHcloud application Secret Manager client factory returned nil")
	}
	return client, prefix, nil
}

func applicationSecretPrefix(planned platform.PlannedStack) (string, error) {
	if planned == nil {
		return "", errors.New("OVHcloud application secrets require a planned stack")
	}
	if _, ok := planned.(Planned); !ok {
		return "", fmt.Errorf("OVHcloud application secrets received unexpected planned type %T", planned)
	}
	stackName := strings.TrimSpace(planned.StackName())
	if stackName == "" || !applicationSecretPathSegment.MatchString(stackName) {
		return "", errors.New("OVHcloud application secrets require a safe stack name")
	}
	return applicationSecretRoot + "/" + stackName, nil
}

func listOwnedApplicationSecrets(ctx context.Context, client resilience.SecretAPI, prefix string) ([]ownedApplicationSecretEntry, error) {
	listed, err := client.List(ctx)
	if err != nil {
		return nil, err
	}
	owned := make([]ownedApplicationSecretEntry, 0, len(listed))
	seen := make(map[string]struct{}, len(listed))
	for _, secret := range listed {
		if !isOwnedApplicationSecret(secret, prefix) {
			continue
		}
		name := strings.TrimPrefix(secret.Path, prefix+"/")
		if err := validateApplicationSecretName(name); err != nil {
			return nil, errors.New("OVHcloud application Secret Manager returned an invalid owned path")
		}
		if _, exists := seen[name]; exists {
			return nil, errors.New("OVHcloud application Secret Manager returned duplicate owned paths")
		}
		seen[name] = struct{}{}
		owned = append(owned, ownedApplicationSecretEntry{path: secret.Path, name: name})
	}
	sort.Slice(owned, func(i, j int) bool { return owned[i].name < owned[j].name })
	return owned, nil
}

func isOwnedApplicationSecret(secret resilience.SecretMetadata, prefix string) bool {
	return secret.Path != "" &&
		strings.HasPrefix(secret.Path, prefix+"/") &&
		strings.EqualFold(strings.TrimSpace(secret.State), "active") &&
		secret.CurrentVersion > 0 &&
		secret.CustomMetadata[secretManagedByKey] == secretManagedByValue &&
		secret.CustomMetadata[secretDataClassKey] == secretDataClassValue &&
		secret.CustomMetadata[secretOwnerKey] == prefix
}

func applicationSecretMetadata(prefix string) map[string]string {
	return map[string]string{
		secretManagedByKey: secretManagedByValue,
		secretDataClassKey: secretDataClassValue,
		secretOwnerKey:     prefix,
	}
}

func validateApplicationSecretName(name string) error {
	if name == "" {
		return errors.New("application secret name is required")
	}
	if len(name) > 512 || name != strings.TrimSpace(name) || strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") {
		return errors.New("application secret name is invalid")
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." || !applicationSecretPathSegment.MatchString(segment) {
			return errors.New("application secret name is invalid")
		}
	}
	return nil
}
