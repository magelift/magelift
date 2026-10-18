package stack

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	scalewayresilience "github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/internal/platform"
)

var (
	ErrSecretNameRequired      = errors.New("Scaleway application secret name is required")
	ErrSecretValueMissing      = errors.New("Scaleway application secret value is required")
	ErrSecretNotFound          = errors.New("Scaleway application secret was not found")
	ErrSecretNameAmbiguous     = errors.New("Scaleway application secret name is ambiguous")
	ErrSecretOwnershipConflict = errors.New("Scaleway application secret is not owned by this stack")
)

const (
	secretManagedTagPrefix = "magelift-managed:"
	secretOwnerTagPrefix   = "magelift-owner:"
)

// Secrets implements the actual platform.Secrets port for Scaleway Secret
// Manager. The client is intentionally provider-local and optional: the
// production module constructs it from the planned project/region, while
// tests inject the narrow existing SecretAPI without any cloud calls.
type Secrets struct {
	client scalewayresilience.SecretAPI
}

var _ platform.Secrets = Secrets{}

func (s Secrets) List(ctx context.Context, planned platform.PlannedStack) ([]platform.SecretMeta, error) {
	ownerTag, err := applicationSecretOwnerTag(planned)
	if err != nil {
		return nil, err
	}
	client, err := s.clientFor(ctx, planned)
	if err != nil {
		return nil, err
	}
	items, err := client.List(ctx)
	if err != nil {
		return nil, secretRequestError(ctx, "list application secrets", err)
	}
	result := make([]platform.SecretMeta, 0, len(items))
	for _, item := range items {
		if !isOwnedApplicationSecret(item, ownerTag) {
			continue
		}
		if strings.TrimSpace(item.Name) == "" {
			return nil, errors.New("list application secrets: Scaleway returned a secret without a name")
		}
		result = append(result, platform.SecretMeta{Name: item.Name})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s Secrets) Set(ctx context.Context, planned platform.PlannedStack, name string, value []byte) error {
	if strings.TrimSpace(name) == "" {
		return ErrSecretNameRequired
	}
	if len(value) == 0 {
		return ErrSecretValueMissing
	}
	ownerTag, err := applicationSecretOwnerTag(planned)
	if err != nil {
		return err
	}
	client, err := s.clientFor(ctx, planned)
	if err != nil {
		return err
	}
	item, found, err := s.find(ctx, client, name, ownerTag)
	if err != nil {
		return err
	}
	data := append([]byte(nil), value...)
	if found {
		if strings.TrimSpace(item.ID) == "" {
			return errors.New("set application secret: Scaleway returned a secret without an ID")
		}
		if err := client.CreateVersion(ctx, item.ID, data); err != nil {
			return secretRequestError(ctx, "set application secret version", err)
		}
		return nil
	}

	created, err := client.Create(ctx, name, applicationSecretTags(ownerTag), false)
	if err != nil {
		return secretRequestError(ctx, "create application secret", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return errors.New("create application secret: Scaleway returned a secret without an ID")
	}
	if !isOwnedApplicationSecret(created, ownerTag) {
		if cleanupErr := client.Delete(ctx, created.ID); cleanupErr != nil {
			return errors.Join(errors.New("create application secret did not prove ownership"), secretRequestError(ctx, "clean up unowned application secret", cleanupErr))
		}
		return errors.New("create application secret did not prove ownership")
	}
	if err := client.CreateVersion(ctx, created.ID, data); err != nil {
		createVersionErr := secretRequestError(ctx, "set application secret version", err)
		if cleanupErr := client.Delete(ctx, created.ID); cleanupErr != nil {
			return errors.Join(createVersionErr, secretRequestError(ctx, "clean up incomplete application secret", cleanupErr))
		}
		return createVersionErr
	}
	return nil
}

func (s Secrets) Remove(ctx context.Context, planned platform.PlannedStack, name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrSecretNameRequired
	}
	ownerTag, err := applicationSecretOwnerTag(planned)
	if err != nil {
		return err
	}
	client, err := s.clientFor(ctx, planned)
	if err != nil {
		return err
	}
	item, found, err := s.find(ctx, client, name, ownerTag)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrSecretNotFound, name)
	}
	if item.Protected {
		return fmt.Errorf("remove application secret %q: secret is protected in Scaleway Secret Manager; unprotect it explicitly before removal", name)
	}
	if err := client.Delete(ctx, item.ID); err != nil {
		return secretRequestError(ctx, "remove application secret", err)
	}
	return nil
}

func (s Secrets) clientFor(ctx context.Context, planned platform.PlannedStack) (scalewayresilience.SecretAPI, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway application secret context is required")
	}
	scalewayPlanned, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("Scaleway secrets received unexpected planned type %T", planned)
	}
	if s.client != nil {
		return s.client, nil
	}
	return scalewayresilience.NewScalewaySecretAPI(ctx, scalewayPlanned.Spec.Identity.ScalewayProject, scalewayPlanned.Spec.Identity.Region)
}

func (s Secrets) find(ctx context.Context, client scalewayresilience.SecretAPI, name, ownerTag string) (scalewayresilience.SecretMetadata, bool, error) {
	items, err := client.List(ctx)
	if err != nil {
		return scalewayresilience.SecretMetadata{}, false, secretRequestError(ctx, "find application secret", err)
	}
	var found scalewayresilience.SecretMetadata
	foundName := false
	for _, item := range items {
		if item.Name != name {
			continue
		}
		if !isOwnedApplicationSecret(item, ownerTag) {
			return scalewayresilience.SecretMetadata{}, false, fmt.Errorf("%w: %q", ErrSecretOwnershipConflict, name)
		}
		if foundName {
			return scalewayresilience.SecretMetadata{}, false, fmt.Errorf("%w: %q", ErrSecretNameAmbiguous, name)
		}
		found = item
		foundName = true
	}
	return found, foundName, nil
}

func applicationSecretOwnerTag(planned platform.PlannedStack) (string, error) {
	value, ok := planned.(Planned)
	if !ok {
		return "", fmt.Errorf("Scaleway secrets received unexpected planned type %T", planned)
	}
	stackName := strings.TrimSpace(value.StackName())
	if stackName == "" || strings.ContainsAny(stackName, "\r\n\x00") {
		return "", errors.New("Scaleway application secrets require a safe stack identity")
	}
	return secretOwnerTagPrefix + stackName, nil
}

func applicationSecretTags(ownerTag string) []string {
	return []string{secretManagedTagPrefix + "application-secrets", ownerTag}
}

func isOwnedApplicationSecret(secret scalewayresilience.SecretMetadata, ownerTag string) bool {
	managedTag := secretManagedTagPrefix + "application-secrets"
	return hasSecretTag(secret.Tags, managedTag) && hasSecretTag(secret.Tags, ownerTag)
}

func hasSecretTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}

// Provider errors are deliberately normalized at the platform boundary. A
// generated SDK error must not be able to echo a request payload or secret
// value into CLI output, plans, logs, or evidence.
func secretRequestError(ctx context.Context, operation string, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	return fmt.Errorf("%s: Scaleway Secret Manager request failed", operation)
}
