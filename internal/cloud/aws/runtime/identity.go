package runtime

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func NewIdentity(ctx *pulumi.Context, name string, args IdentityArgs, opts ...pulumi.ResourceOption) (*Identity, error) {
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
	identity := &Identity{}
	if err := ctx.RegisterComponentResourceV2(IdentityTypeToken, name, pulumi.Map{"secretCount": pulumi.Int(len(secrets))}, identity, opts...); err != nil {
		return nil, err
	}
	if err := provisionIdentity(ctx, name, IdentityArgs{Secrets: secrets, Tags: args.Tags}, identity, identity); err != nil {
		return nil, err
	}
	if err := ctx.RegisterResourceOutputs(identity, pulumi.Map{
		"executionRoleArn": identity.ExecutionRoleARN, "executionRoleName": identity.ExecutionRoleName,
		"taskRoleArn": identity.TaskRoleARN, "taskRoleName": identity.TaskRoleName,
		"deploymentRoleArn": identity.DeploymentRoleARN, "deploymentRoleName": identity.DeploymentRoleName,
	}); err != nil {
		return nil, err
	}
	return identity, nil
}

func createIdentity(ctx *pulumi.Context, name string, args IdentityArgs, parent pulumi.Resource) (*Identity, error) {
	identity := &Identity{}
	if err := provisionIdentity(ctx, name, args, identity, parent); err != nil {
		return nil, err
	}
	return identity, nil
}

func provisionIdentity(ctx *pulumi.Context, name string, args IdentityArgs, identity *Identity, parent pulumi.Resource) error {
	identity.secretReferences = append([]SecretReference(nil), args.Secrets...)
	assumeRole := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	executionRole, err := role(ctx, name+"-execution-role", "ECS task execution", assumeRole, args.Tags, name, parent)
	if err != nil {
		return err
	}
	taskRole, err := role(ctx, name+"-task-role", "Magento application task", assumeRole, args.Tags, name, parent)
	if err != nil {
		return err
	}
	deploymentRole, err := role(ctx, name+"-deployment-role", "Magento deployment task", assumeRole, args.Tags, name, parent)
	if err != nil {
		return err
	}
	identity.ExecutionRoleARN, identity.ExecutionRoleName = executionRole.Arn, executionRole.Name
	identity.TaskRoleARN, identity.TaskRoleName = taskRole.Arn, taskRole.Name
	identity.DeploymentRoleARN, identity.DeploymentRoleName = deploymentRole.Arn, deploymentRole.Name
	policy, err := executionPolicy(args.Secrets)
	if err != nil {
		return err
	}
	_, err = iam.NewRolePolicy(ctx, name+"-execution-policy", &iam.RolePolicyArgs{Role: executionRole.Name, Policy: pulumi.String(policy)}, pulumi.Parent(parent))
	return err
}

func role(ctx *pulumi.Context, name, description, assume string, inputTags map[string]string, component string, parent pulumi.Resource) (*iam.Role, error) {
	return iam.NewRole(ctx, name, &iam.RoleArgs{AssumeRolePolicy: pulumi.String(assume), Description: pulumi.String(description), Tags: tags(inputTags, component, name)}, pulumi.Parent(parent))
}

func executionPolicy(secrets []SecretReference) (string, error) {
	type statement struct {
		Effect   string   `json:"Effect"`
		Action   []string `json:"Action"`
		Resource any      `json:"Resource"`
	}
	document := struct {
		Version   string      `json:"Version"`
		Statement []statement `json:"Statement"`
	}{Version: "2012-10-17", Statement: []statement{
		{Effect: "Allow", Action: []string{"ecr:GetAuthorizationToken"}, Resource: "*"},
		{Effect: "Allow", Action: []string{"ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"}, Resource: "*"},
	}}
	byService := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, secret := range secrets {
		match := secretARN.FindStringSubmatch(secret.ARN)
		if seen[match[1]] == nil {
			seen[match[1]] = map[string]bool{}
		}
		if !seen[match[1]][secret.ARN] {
			byService[match[1]] = append(byService[match[1]], secret.ARN)
			seen[match[1]][secret.ARN] = true
		}
	}
	if values := byService["secretsmanager"]; len(values) > 0 {
		document.Statement = append(document.Statement, statement{Effect: "Allow", Action: []string{"secretsmanager:GetSecretValue"}, Resource: values})
	}
	if values := byService["ssm"]; len(values) > 0 {
		document.Statement = append(document.Statement, statement{Effect: "Allow", Action: []string{"ssm:GetParameters"}, Resource: values})
	}
	encoded, err := json.Marshal(document)
	return string(encoded), err
}

// attachExecutionLogPolicy grants awslogs drivers CreateLogStream/PutLogEvents on
// the Magento workload groups created before the runtime.
func attachExecutionLogPolicy(ctx *pulumi.Context, name string, args Args, identity *Identity, parent pulumi.Resource) error {
	prefix := strings.TrimSpace(args.LogGroupPrefix)
	region := strings.TrimSpace(args.Region)
	if prefix == "" || region == "" {
		return nil
	}
	policy, err := executionLogPolicy(region, prefix)
	if err != nil {
		return err
	}
	_, err = iam.NewRolePolicy(ctx, name+"-execution-logs-policy", &iam.RolePolicyArgs{
		Role: identity.ExecutionRoleName, Policy: pulumi.String(policy),
	}, pulumi.Parent(parent))
	return err
}

func executionLogPolicy(region, logGroupPrefix string) (string, error) {
	groupARN := "arn:aws:logs:" + region + ":*:log-group:" + logGroupPrefix + "/*"
	document := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect": "Allow",
			"Action": []string{"logs:CreateLogStream", "logs:PutLogEvents"},
			"Resource": []string{
				groupARN,
				groupARN + ":log-stream:*",
			},
		}},
	}
	encoded, err := json.Marshal(document)
	return string(encoded), err
}
