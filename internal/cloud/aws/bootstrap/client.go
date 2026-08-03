package bootstrap

import (
	"context"
	"errors"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
)

var (
	ErrAccountVerification = errors.New("AWS account verification failed")
	ErrAccountMismatch     = errors.New("active AWS account does not match the selected environment")
)

// NewAWS creates clients for the account and region selected by the caller.
// Credential and role selection remains with the AWS SDK default chain.
func NewAWS(ctx context.Context, region string) (*Bootstrapper, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	configuration, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return New(s3.NewFromConfig(configuration, func(options *s3.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
			options.UsePathStyle = true
		}
	}), kms.NewFromConfig(configuration, func(options *kms.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	})), nil
}

type IdentityEnsurer interface {
	Ensure(context.Context, IdentityPlan) error
}

func NewAWSIdentity(ctx context.Context, region string) (*IdentityBootstrapper, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	configuration, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return NewIdentity(iam.NewFromConfig(configuration, func(options *iam.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}), ssm.NewFromConfig(configuration, func(options *ssm.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	})), nil
}

func VerifyAccount(ctx context.Context, region, expectedAccount string) error {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return err
	}
	configuration, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return ErrAccountVerification
	}
	identity, err := sts.NewFromConfig(configuration, func(options *sts.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return cause
		}
		return ErrAccountVerification
	}
	if identity == nil || awssdk.ToString(identity.Account) == "" {
		return ErrAccountVerification
	}
	if awssdk.ToString(identity.Account) != expectedAccount {
		return ErrAccountMismatch
	}
	return nil
}
