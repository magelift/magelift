package secretref

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want Reference
	}{
		{"secret name", "aws-secrets-manager://commerce/composer", Reference{Kind: SecretsManager, ID: "commerce/composer"}},
		{"secret ARN", "aws-secrets-manager://arn:aws:secretsmanager:eu-west-3:123456789012:secret:composer-AbCd", Reference{Kind: SecretsManager, ID: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:composer-AbCd"}},
		{"JSON field", "aws-secrets-manager://composer?jsonField=http-basic%2Erepo%2Emagento%2Ecom", Reference{Kind: SecretsManager, ID: "composer", JSONField: "http-basic.repo.magento.com"}},
		{"parameter path", "ssm:///magelift/composer/auth", Reference{Kind: ParameterStore, ID: "/magelift/composer/auth"}},
		{"escaped identifier", "ssm://folder%2Fname%40example", Reference{Kind: ParameterStore, ID: "folder/name@example"}},
		{"GCP secret", "gcp-secret-manager://projects/p/secrets/composer/versions/latest", Reference{Kind: GCPSecretManager, ID: "projects/p/secrets/composer/versions/latest"}},
		{"GCP JSON field", "gcp-secret-manager://projects/p/secrets/shared/versions/1?jsonField=composer", Reference{Kind: GCPSecretManager, ID: "projects/p/secrets/shared/versions/1", JSONField: "composer"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(test.text)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("Parse() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseRejectsInvalidReferences(t *testing.T) {
	t.Parallel()
	values := []string{
		"plain-text", "vault://secret", "ssm://", "ssm://user@parameter", "ssm://parameter#field",
		"ssm://parameter?name=value", "ssm://parameter?withDecryption=true",
		"aws-secrets-manager://secret?region=eu-west-3", "aws-secrets-manager://secret?jsonField=", "aws-secrets-manager://%zz",
	}
	for _, value := range values {
		value := value
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if _, err := Parse(value); !errors.Is(err, ErrInvalidReference) {
				t.Fatalf("Parse() error = %v, want ErrInvalidReference", err)
			}
		})
	}
}

type secretsProvider struct {
	value []byte
	err   error
	id    string
}

func (provider *secretsProvider) GetSecretValue(_ context.Context, id string) ([]byte, error) {
	provider.id = id
	return provider.value, provider.err
}

type parameterProvider struct {
	value []byte
	err   error
	id    string
}

func (provider *parameterProvider) GetParameter(_ context.Context, id string) ([]byte, error) {
	provider.id = id
	return provider.value, provider.err
}

func TestResolveSecretsManagerValueAndJSONField(t *testing.T) {
	t.Parallel()
	provider := &secretsProvider{value: []byte(`{"username":"mage","password":"secret"}`)}
	resolver := Resolver{SecretsManager: provider}

	value, err := resolver.Resolve(context.Background(), Reference{Kind: SecretsManager, ID: "composer", JSONField: "password"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if string(value) != "secret" || provider.id != "composer" {
		t.Fatalf("Resolve() = %q, provider id = %q", value, provider.id)
	}

	value[0] = 'X'
	valueAgain, err := resolver.Resolve(context.Background(), Reference{Kind: SecretsManager, ID: "composer"})
	if err != nil {
		t.Fatalf("Resolve() second error = %v", err)
	}
	if valueAgain[0] != '{' {
		t.Fatal("Resolve() returned provider-owned storage")
	}
}

func TestResolveParameter(t *testing.T) {
	t.Parallel()
	provider := &parameterProvider{value: []byte("composer-auth")}
	value, err := (Resolver{ParameterStore: provider}).Resolve(context.Background(), Reference{Kind: ParameterStore, ID: "/auth"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if string(value) != "composer-auth" || provider.id != "/auth" {
		t.Fatalf("unexpected provider call: id=%q", provider.id)
	}
}

func TestResolveRejectsOversizeAndInvalidJSONFields(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value []byte
		field string
		want  error
	}{
		{"oversize", []byte(strings.Repeat("x", MaxValueSize+1)), "", ErrValueTooLarge},
		{"not object", []byte(`[]`), "password", ErrInvalidReference},
		{"missing", []byte(`{"username":"mage"}`), "password", ErrInvalidReference},
		{"not string", []byte(`{"password":42}`), "password", ErrInvalidReference},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			resolver := Resolver{SecretsManager: &secretsProvider{value: test.value}}
			_, err := resolver.Resolve(context.Background(), Reference{Kind: SecretsManager, ID: "secret", JSONField: test.field})
			if !errors.Is(err, test.want) {
				t.Fatalf("Resolve() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestResolveDoesNotExposeProviderErrorOrValue(t *testing.T) {
	t.Parallel()
	secret := "do-not-log-this"
	providerError := errors.New("provider included " + secret)
	resolver := Resolver{SecretsManager: &secretsProvider{value: []byte(secret), err: providerError}}

	_, err := resolver.Resolve(context.Background(), Reference{Kind: SecretsManager, ID: "secret"})
	if !errors.Is(err, ErrProvider) {
		t.Fatalf("Resolve() error = %v, want ErrProvider", err)
	}
	if strings.Contains(err.Error(), secret) || errors.Is(err, providerError) {
		t.Fatalf("Resolve() exposed provider details: %v", err)
	}
}

func TestResolveRejectsMissingProviderAndUnknownKind(t *testing.T) {
	t.Parallel()
	if _, err := (Resolver{}).Resolve(context.Background(), Reference{Kind: SecretsManager, ID: "secret"}); !errors.Is(err, ErrProvider) {
		t.Fatalf("missing provider error = %v", err)
	}
	if _, err := (Resolver{}).Resolve(context.Background(), Reference{Kind: "other", ID: "secret"}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("unknown kind error = %v", err)
	}
	if _, err := (Resolver{}).Resolve(context.Background(), Reference{Kind: ParameterStore, ID: "parameter", JSONField: "field"}); !errors.Is(err, ErrInvalidReference) {
		t.Fatalf("incompatible field error = %v", err)
	}
}

func TestResolvePreservesContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &secretsProvider{err: errors.New("provider failure")}
	_, err := (Resolver{SecretsManager: provider}).Resolve(ctx, Reference{Kind: SecretsManager, ID: "secret"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve() error = %v, want context.Canceled", err)
	}
}
