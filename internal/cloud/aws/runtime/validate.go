package runtime

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func validate(name string, args Args) ([]SecretReference, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" || args.VpcID == nil || len(args.PrivateSubnetIDs) == 0 {
		return nil, errors.New("runtime name, region, VPC, and private subnets are required")
	}
	if args.ApplicationMode != "integrated" && args.ApplicationMode != "headless" {
		return nil, errors.New("runtime application mode must be integrated or headless")
	}
	if args.WebRuntime != "nginx-fpm" && args.WebRuntime != "frankenphp-classic" {
		return nil, errors.New("runtime web runtime must be nginx-fpm or frankenphp-classic")
	}
	if args.ApplicationMode == "integrated" {
		if !imageDigest.MatchString(args.VarnishImage) {
			return nil, errors.New("integrated runtime requires a Varnish image pinned by a lowercase SHA-256 digest")
		}
	} else if args.VarnishImage != "" {
		return nil, errors.New("headless runtime cannot include a Varnish image")
	}
	if (args.WebSecurityGroupID == nil) != (args.TargetGroupARN == nil) {
		return nil, errors.New("runtime external web security group and target group must be provided together")
	}
	if !imageDigest.MatchString(args.Image) {
		return nil, errors.New("runtime image must be pinned by a lowercase SHA-256 digest")
	}
	if args.SearchProxyImage != "" && !imageDigest.MatchString(args.SearchProxyImage) {
		return nil, errors.New("search proxy image must be pinned by a lowercase SHA-256 digest")
	}
	if args.SearchProxyImage != "" && (args.Capabilities == nil || args.Capabilities.SearchEndpoint == nil) {
		return nil, errors.New("search proxy requires a search endpoint")
	}
	if args.ContainerPort < 1 || args.ContainerPort > 65535 {
		return nil, errors.New("container port must be between 1 and 65535")
	}
	cpu, cpuErr := strconv.Atoi(args.TaskCPU)
	memory, memoryErr := strconv.Atoi(args.TaskMemory)
	if cpuErr != nil || memoryErr != nil || cpu < 1 || memory < 1 || args.DesiredCount < 1 {
		return nil, errors.New("benchmark-selected task CPU, memory, and desired count are required")
	}
	if args.Capabilities != nil {
		values := []pulumi.StringInput{
			args.Capabilities.DatabaseWriterEndpoint, args.Capabilities.DatabaseSecretARN, args.Capabilities.CacheEndpoint,
			args.Capabilities.SessionEndpoint, args.Capabilities.SearchEndpoint, args.Capabilities.QueueMode,
			args.Capabilities.QueueEndpoint, args.Capabilities.QueueUsername, args.Capabilities.MediaBucket,
		}
		for _, value := range values {
			if value == nil {
				return nil, errors.New("runtime capability references must be complete")
			}
		}
	}
	if args.EncryptionKeyARN == nil {
		return nil, errors.New("runtime Magento encryption key secret is required")
	}
	if args.DatabaseSecretARN == nil {
		return nil, errors.New("runtime managed database secret is required")
	}
	secrets, err := validateSecretReferences(args.Secrets)
	if err != nil {
		return nil, err
	}
	for _, secret := range secrets {
		if secret.Name == "MAGELIFT_DATABASE_CREDENTIALS" {
			return nil, errors.New("MAGELIFT_DATABASE_CREDENTIALS is reserved for the managed database secret")
		}
	}
	if !hasSecretReference(secrets, platform.EnvMagentoCryptKey) {
		return nil, errors.New("runtime secrets must include " + platform.EnvMagentoCryptKey)
	}
	for _, secret := range secrets {
		if secret.Name == platform.EnvMagentoCryptKey && secret.JSONKey != "" {
			return nil, errors.New(platform.EnvMagentoCryptKey + " must reference the full secret value")
		}
	}
	if encryptionInput, ok := args.EncryptionKeyARN.(pulumi.String); ok {
		for _, secret := range secrets {
			if secret.Name == platform.EnvMagentoCryptKey && secret.ARN != string(encryptionInput) {
				return nil, errors.New("runtime encryption key secret reference does not match the task secret")
			}
		}
	}
	if args.Identity != nil && !sameSecretReferences(secrets, args.Identity.secretReferences) {
		return nil, errors.New("runtime identity and task definitions must use the same secret references")
	}
	return secrets, nil
}

func validateSecretReferences(input []SecretReference) ([]SecretReference, error) {
	result := append([]SecretReference(nil), input...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	for index, secret := range result {
		if !secretName.MatchString(secret.Name) || !secretARN.MatchString(secret.ARN) {
			return nil, errors.New("task secrets require a stable environment name and Secrets Manager or SSM ARN")
		}
		if secret.JSONKey != "" && (!strings.Contains(secret.ARN, ":secret:") || !secretJSONKey.MatchString(secret.JSONKey)) {
			return nil, errors.New("JSON key selectors require a Secrets Manager ARN and a safe key name")
		}
		if index > 0 && result[index-1].Name == secret.Name {
			return nil, fmt.Errorf("duplicate task secret name %q", secret.Name)
		}
	}
	return result, nil
}

func sameSecretReferences(left, right []SecretReference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func hasSecretReference(secrets []SecretReference, name string) bool {
	for _, secret := range secrets {
		if secret.Name == name {
			return true
		}
	}
	return false
}
