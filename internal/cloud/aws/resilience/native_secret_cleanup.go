package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// cleanupSecretClass removes only the archive objects and isolated restore
// secrets selected by the operation marker. The source secret is deliberately
// outside this provider cleanup boundary; the provider-neutral certification
// cell deletes its exact synthetic source reference after this succeeds.
func (api *NativeAPI) cleanupSecretClass(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.s3 == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager recovery requires Secrets Manager and archive S3 APIs for cleanup")
	}
	marker := strings.TrimSpace(state.OwnershipMarker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager cleanup ownership marker is required and must be single-line")
	}

	refs := make([]string, 0)
	archivePrefix := api.archivePrefixForMarker(marker) + "/" + state.DataClass + "/"
	if err := api.cleanupS3ObjectPrefix(ctx, s3ObjectStore{api: api.s3, config: api.config}, api.archiveBucket(), archivePrefix, marker, true, &refs); err != nil {
		return provider.NativeOperationObservation{}, err
	}

	owned, err := api.listOwnedSecrets(ctx, marker)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("list AWS Secrets Manager restore secrets for cleanup: %w", err)
	}
	for _, resource := range owned {
		name, err := parseSecretReference(resource.Identity)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		description, err := api.secrets.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
		if err != nil {
			if isSecretNotFound(err) {
				continue
			}
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect AWS Secrets Manager restore secret %q for cleanup: %w", resource.Identity, err)
		}
		if description == nil || !secretTagsMatch(description.Tags, marker, state.DataClass) || !strings.HasPrefix(name, api.restoreSecretPrefix(state.DataClass)) {
			return provider.NativeOperationObservation{}, fmt.Errorf("refusing to clean AWS Secrets Manager secret %q because ownership metadata drifted", resource.Identity)
		}
		if _, err := api.secrets.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{SecretId: aws.String(name), ForceDeleteWithoutRecovery: aws.Bool(true)}); err != nil && !isSecretNotFound(err) {
			return provider.NativeOperationObservation{}, fmt.Errorf("delete owned AWS Secrets Manager restore secret %q: %w", resource.Identity, err)
		}
		if err := waitForSecretGone(ctx, api.secrets, name); err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("verify AWS Secrets Manager restore secret %q cleanup: %w", resource.Identity, err)
		}
		refs = append(refs, resource.Identity)
	}
	remaining, err := api.listOwnedSecrets(ctx, marker)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify AWS Secrets Manager restore cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return provider.NativeOperationObservation{}, fmt.Errorf("owned AWS Secrets Manager restore secrets remain after cleanup: %#v", remaining)
	}
	sort.Strings(refs)
	return observationWithOperation(state, operationID, refs, []string{"aws.secrets-manager.ownership-cleanup", "aws.s3.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func waitForSecretGone(ctx context.Context, api SecretsAPI, name string) error {
	for {
		_, err := api.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
		if isSecretNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
