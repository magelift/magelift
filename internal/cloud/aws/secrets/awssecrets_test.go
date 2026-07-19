package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type secretsManagerFunc func(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)

func (f secretsManagerFunc) GetSecretValue(ctx context.Context, input *secretsmanager.GetSecretValueInput, options ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return f(ctx, input, options...)
}

type ssmFunc func(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)

func (f ssmFunc) GetParameter(ctx context.Context, input *ssm.GetParameterInput, options ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	return f(ctx, input, options...)
}

type storeClient struct {
	create func(context.Context, *secretsmanager.CreateSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	put    func(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	list   func(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
	delete func(context.Context, *secretsmanager.DeleteSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
}

func (f storeClient) CreateSecret(ctx context.Context, input *secretsmanager.CreateSecretInput, options ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	return f.create(ctx, input, options...)
}

func (f storeClient) PutSecretValue(ctx context.Context, input *secretsmanager.PutSecretValueInput, options ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	return f.put(ctx, input, options...)
}

func (f storeClient) ListSecrets(ctx context.Context, input *secretsmanager.ListSecretsInput, options ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	return f.list(ctx, input, options...)
}

func (f storeClient) DeleteSecret(ctx context.Context, input *secretsmanager.DeleteSecretInput, options ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	return f.delete(ctx, input, options...)
}

func TestGetSecretValueReturnsString(t *testing.T) {
	want := "composer-auth"
	resolver := NewFromClients(secretsManagerFunc(func(ctx context.Context, input *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
		if got := aws.ToString(input.SecretId); got != "projects/shop/composer" {
			t.Fatalf("SecretId = %q", got)
		}
		return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(want)}, nil
	}), nil)

	got, err := resolver.GetSecretValue(context.Background(), "projects/shop/composer")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("value = %q, want %q", got, want)
	}
}

func TestGetSecretValueReturnsBinaryCopy(t *testing.T) {
	value := []byte("binary-secret")
	resolver := NewFromClients(secretsManagerFunc(func(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
		return &secretsmanager.GetSecretValueOutput{SecretBinary: value}, nil
	}), nil)

	got, err := resolver.GetSecretValue(context.Background(), "secret")
	if err != nil {
		t.Fatal(err)
	}
	value[0] = 'X'
	if string(got) != "binary-secret" {
		t.Fatalf("value shares SDK response storage: %q", got)
	}
}

func TestGetSecretValuePreservesContextErrorWithoutLeakingIdentifier(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := NewFromClients(secretsManagerFunc(func(callCtx context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
		return nil, callCtx.Err()
	}), nil)

	_, err := resolver.GetSecretValue(ctx, "sensitive/name")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if err.Error() == "" || strings.Contains(err.Error(), "sensitive/name") {
		t.Fatalf("error leaks identifier: %v", err)
	}
}

func TestGetSecretValueRejectsMissingValue(t *testing.T) {
	resolver := NewFromClients(secretsManagerFunc(func(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
		return &secretsmanager.GetSecretValueOutput{}, nil
	}), nil)

	_, err := resolver.GetSecretValue(context.Background(), "secret")
	if !errors.Is(err, ErrSecretValueMissing) {
		t.Fatalf("error = %v", err)
	}
}

func TestGetParameterRequestsDecryption(t *testing.T) {
	resolver := NewFromClients(nil, ssmFunc(func(ctx context.Context, input *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
		if got := aws.ToString(input.Name); got != "/shop/composer" {
			t.Fatalf("Name = %q", got)
		}
		if !aws.ToBool(input.WithDecryption) {
			t.Fatal("WithDecryption is false")
		}
		return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String("parameter-value")}}, nil
	}))

	got, err := resolver.GetParameter(context.Background(), "/shop/composer")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "parameter-value" {
		t.Fatalf("value = %q", got)
	}
}

func TestGetParameterRejectsMissingValue(t *testing.T) {
	resolver := NewFromClients(nil, ssmFunc(func(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
		return &ssm.GetParameterOutput{}, nil
	}))

	_, err := resolver.GetParameter(context.Background(), "parameter")
	if !errors.Is(err, ErrParameterValueMissing) {
		t.Fatalf("error = %v", err)
	}
}

func TestEmptyNamesDoNotCallAWS(t *testing.T) {
	called := false
	resolver := NewFromClients(secretsManagerFunc(func(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
		called = true
		return nil, nil
	}), ssmFunc(func(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
		called = true
		return nil, nil
	}))

	if _, err := resolver.GetSecretValue(context.Background(), ""); !errors.Is(err, ErrSecretNameRequired) {
		t.Fatalf("secret error = %v", err)
	}
	if _, err := resolver.GetParameter(context.Background(), ""); !errors.Is(err, ErrParameterNameRequired) {
		t.Fatalf("parameter error = %v", err)
	}
	if called {
		t.Fatal("AWS client was called")
	}
}

func TestStoreListPaginatesAndSortsByName(t *testing.T) {
	var tokens []string
	client := storeClient{list: func(_ context.Context, input *secretsmanager.ListSecretsInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
		tokens = append(tokens, aws.ToString(input.NextToken))
		if input.NextToken == nil {
			return &secretsmanager.ListSecretsOutput{NextToken: aws.String("page-2"), SecretList: []secretstypes.SecretListEntry{{Name: aws.String("zeta"), ARN: aws.String("arn:z")}}}, nil
		}
		return &secretsmanager.ListSecretsOutput{SecretList: []secretstypes.SecretListEntry{{Name: aws.String("alpha"), ARN: aws.String("arn:a")}}}, nil
	}}

	secrets, err := NewStoreFromClient(client).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secrets) != 2 || secrets[0].Name != "alpha" || secrets[1].Name != "zeta" {
		t.Fatalf("unexpected secrets: %+v", secrets)
	}
	if strings.Join(tokens, ",") != ",page-2" {
		t.Fatalf("tokens = %q", tokens)
	}
}

func TestStoreSetCreatesMissingSecret(t *testing.T) {
	created := false
	client := storeClient{
		put: func(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
			return nil, &secretstypes.ResourceNotFoundException{}
		},
		create: func(_ context.Context, input *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
			created = true
			if aws.ToString(input.Name) != "shop/api" || aws.ToString(input.SecretString) != "value" {
				t.Fatalf("unexpected create input: %+v", input)
			}
			return &secretsmanager.CreateSecretOutput{}, nil
		},
	}
	if err := NewStoreFromClient(client).Set(context.Background(), "shop/api", []byte("value")); err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("missing secret was not created")
	}
}

func TestStoreRemoveUsesRecoveryWindow(t *testing.T) {
	client := storeClient{delete: func(_ context.Context, input *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
		if aws.ToString(input.SecretId) != "shop/api" || aws.ToInt64(input.RecoveryWindowInDays) != 30 || aws.ToBool(input.ForceDeleteWithoutRecovery) {
			t.Fatalf("unsafe delete input: %+v", input)
		}
		return &secretsmanager.DeleteSecretOutput{}, nil
	}}
	if err := NewStoreFromClient(client).Remove(context.Background(), "shop/api"); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsEmptyValueWithoutCallingAWS(t *testing.T) {
	called := false
	client := storeClient{put: func(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
		called = true
		return nil, nil
	}}
	if err := NewStoreFromClient(client).Set(context.Background(), "shop/api", nil); !errors.Is(err, ErrSecretValueMissing) {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("AWS was called for an empty value")
	}
}
