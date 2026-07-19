package secretref

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const MaxValueSize = 1 << 20

type Kind string

const (
	SecretsManager Kind = "aws-secrets-manager"
	ParameterStore Kind = "ssm"
)

var (
	ErrInvalidReference = errors.New("invalid secret reference")
	ErrProvider         = errors.New("secret provider failed")
	ErrValueTooLarge    = errors.New("resolved secret exceeds 1 MiB")
)

type Reference struct {
	Kind      Kind
	ID        string
	JSONField string
}

func Parse(value string) (Reference, error) {
	scheme, remainder, found := strings.Cut(value, "://")
	if !found {
		return Reference{}, invalid("scheme and // are required")
	}

	kind := Kind(scheme)
	if kind != SecretsManager && kind != ParameterStore {
		return Reference{}, invalid("unsupported scheme")
	}
	if strings.Contains(remainder, "#") {
		return Reference{}, invalid("fragments are not allowed")
	}

	rawID, rawQuery, _ := strings.Cut(remainder, "?")
	if rawID == "" {
		return Reference{}, invalid("secret identifier is required")
	}
	if strings.Contains(rawID, "@") {
		return Reference{}, invalid("userinfo is not allowed")
	}
	id, err := url.PathUnescape(rawID)
	if err != nil || id == "" || strings.IndexByte(id, 0) >= 0 {
		return Reference{}, invalid("secret identifier is invalid")
	}

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return Reference{}, invalid("query is invalid")
	}
	for key, values := range query {
		if len(values) != 1 {
			return Reference{}, invalid("query keys may appear once")
		}
		if kind != SecretsManager || key != "jsonField" {
			return Reference{}, invalid("unknown query key")
		}
	}

	reference := Reference{Kind: kind, ID: id}
	if kind == SecretsManager {
		reference.JSONField = query.Get("jsonField")
		if _, present := query["jsonField"]; present && reference.JSONField == "" {
			return Reference{}, invalid("jsonField cannot be empty")
		}
	}
	return reference, nil
}

func invalid(reason string) error {
	return fmt.Errorf("%w: %s", ErrInvalidReference, reason)
}

type SecretsManagerProvider interface {
	GetSecretValue(context.Context, string) ([]byte, error)
}

type ParameterStoreProvider interface {
	GetParameter(context.Context, string) ([]byte, error)
}

type Resolver struct {
	SecretsManager SecretsManagerProvider
	ParameterStore ParameterStoreProvider
}

func (resolver Resolver) Resolve(ctx context.Context, reference Reference) ([]byte, error) {
	if reference.ID == "" {
		return nil, invalid("secret identifier is required")
	}
	if reference.Kind == ParameterStore && reference.JSONField != "" {
		return nil, invalid("jsonField is only valid for Secrets Manager")
	}

	var value []byte
	var err error

	switch reference.Kind {
	case SecretsManager:
		if resolver.SecretsManager == nil {
			return nil, ErrProvider
		}
		value, err = resolver.SecretsManager.GetSecretValue(ctx, reference.ID)
	case ParameterStore:
		if resolver.ParameterStore == nil {
			return nil, ErrProvider
		}
		value, err = resolver.ParameterStore.GetParameter(ctx, reference.ID)
	default:
		return nil, ErrInvalidReference
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrProvider
	}
	if len(value) > MaxValueSize {
		return nil, ErrValueTooLarge
	}

	if reference.Kind == SecretsManager && reference.JSONField != "" {
		var object map[string]json.RawMessage
		if json.Unmarshal(value, &object) != nil || object == nil {
			return nil, invalid("secret value is not a JSON object")
		}
		raw, exists := object[reference.JSONField]
		if !exists {
			return nil, invalid("JSON field does not exist")
		}
		var selected string
		if json.Unmarshal(raw, &selected) != nil {
			return nil, invalid("JSON field is not a string")
		}
		value = []byte(selected)
		if len(value) > MaxValueSize {
			return nil, ErrValueTooLarge
		}
	}

	return append([]byte(nil), value...), nil
}
