package secrets

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
)

var (
	ErrSecretNameRequired    = errors.New("secret name is required")
	ErrParameterNameRequired = errors.New("parameter name is required")
	ErrSecretValueMissing    = errors.New("secret has no value")
	ErrParameterValueMissing = errors.New("parameter has no value")
)

type SecretsManagerAPI interface {
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

type SSMAPI interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

type SecretsStoreAPI interface {
	CreateSecret(context.Context, *secretsmanager.CreateSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	PutSecretValue(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
	DeleteSecret(context.Context, *secretsmanager.DeleteSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
}

type Secret struct {
	ARN  string `json:"arn" yaml:"arn"`
	Name string `json:"name" yaml:"name"`
}

type Store struct {
	client SecretsStoreAPI
}

type Resolver struct {
	secrets    SecretsManagerAPI
	parameters SSMAPI
}

func New(ctx context.Context, region string) (*Resolver, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	var options []func(*config.LoadOptions) error
	if region != "" {
		options = append(options, config.WithRegion(region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}

	return NewFromClients(newSecretsManagerClient(cfg, endpoint), newSSMClient(cfg, endpoint)), nil
}

func NewFromClients(secrets SecretsManagerAPI, parameters SSMAPI) *Resolver {
	return &Resolver{secrets: secrets, parameters: parameters}
}

func NewStore(ctx context.Context, region string) (*Store, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	options := []func(*config.LoadOptions) error{}
	if region != "" {
		options = append(options, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	return NewStoreFromClient(newSecretsManagerClient(cfg, endpoint)), nil
}

func newSecretsManagerClient(cfg aws.Config, endpoint string) *secretsmanager.Client {
	return secretsmanager.NewFromConfig(cfg, func(options *secretsmanager.Options) {
		if endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
}

func newSSMClient(cfg aws.Config, endpoint string) *ssm.Client {
	return ssm.NewFromConfig(cfg, func(options *ssm.Options) {
		if endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
	})
}

func NewStoreFromClient(client SecretsStoreAPI) *Store {
	return &Store{client: client}
}

func (s *Store) List(ctx context.Context) ([]Secret, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("Secrets Manager client is not configured")
	}
	var secrets []Secret
	var token *string
	for {
		output, err := s.client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{NextToken: token})
		if err != nil {
			return nil, fmt.Errorf("list AWS Secrets Manager secrets: %w", err)
		}
		if output == nil {
			return nil, errors.New("list AWS Secrets Manager secrets: empty response")
		}
		for _, item := range output.SecretList {
			secrets = append(secrets, Secret{ARN: aws.ToString(item.ARN), Name: aws.ToString(item.Name)})
		}
		if output.NextToken == nil || aws.ToString(output.NextToken) == "" {
			break
		}
		token = output.NextToken
	}
	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Name < secrets[j].Name })
	return secrets, nil
}

func (s *Store) Set(ctx context.Context, name string, value []byte) error {
	if s == nil || s.client == nil {
		return errors.New("Secrets Manager client is not configured")
	}
	if name == "" {
		return ErrSecretNameRequired
	}
	if len(value) == 0 {
		return ErrSecretValueMissing
	}
	_, err := s.client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{SecretId: aws.String(name), SecretString: aws.String(string(value))})
	if err == nil {
		return nil
	}
	var notFound *secretstypes.ResourceNotFoundException
	if errors.As(err, &notFound) {
		_, createErr := s.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{Name: aws.String(name), SecretString: aws.String(string(value))})
		if createErr == nil {
			return nil
		}
		return fmt.Errorf("create AWS secret: %w", createErr)
	}
	return fmt.Errorf("put AWS secret value: %w", err)
}

func (s *Store) Remove(ctx context.Context, name string) error {
	if s == nil || s.client == nil {
		return errors.New("Secrets Manager client is not configured")
	}
	if name == "" {
		return ErrSecretNameRequired
	}
	_, err := s.client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{SecretId: aws.String(name), RecoveryWindowInDays: aws.Int64(30)})
	if err != nil {
		return fmt.Errorf("schedule AWS secret deletion: %w", err)
	}
	return nil
}

func (r *Resolver) GetSecretValue(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, ErrSecretNameRequired
	}
	if r == nil || r.secrets == nil {
		return nil, errors.New("Secrets Manager client is not configured")
	}

	output, err := r.secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(name),
	})
	if err != nil {
		return nil, fmt.Errorf("get value from AWS Secrets Manager: %w", err)
	}
	if output == nil {
		return nil, ErrSecretValueMissing
	}
	if output.SecretString != nil {
		return []byte(*output.SecretString), nil
	}
	if output.SecretBinary != nil {
		return append([]byte(nil), output.SecretBinary...), nil
	}

	return nil, ErrSecretValueMissing
}

func (r *Resolver) GetParameter(ctx context.Context, name string) ([]byte, error) {
	if name == "" {
		return nil, ErrParameterNameRequired
	}
	if r == nil || r.parameters == nil {
		return nil, errors.New("SSM client is not configured")
	}

	output, err := r.parameters.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("get value from AWS Systems Manager Parameter Store: %w", err)
	}
	if output == nil || output.Parameter == nil || output.Parameter.Value == nil {
		return nil, ErrParameterValueMissing
	}

	return []byte(*output.Parameter.Value), nil
}
