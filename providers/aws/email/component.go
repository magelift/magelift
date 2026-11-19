package email

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/route53"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/sesv2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ssm"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:aws:EmailSender"

var domainPattern = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,63}$`)

var hostedZoneIDPattern = regexp.MustCompile(`^Z[A-Z0-9]+$`)

// dkimTokenCount is the number of CNAME tokens SES Easy DKIM always issues
// per domain identity. The tokens arrive as an Output array; Pulumi resolves
// each record through Outputs, so no apply-time fan-out is needed.
const dkimTokenCount = 3

const dkimTTL = 300

type Args struct {
	Project          string
	Environment      string
	Domain           string
	HostedZoneID     string
	Region           string
	RegionalProvider *awsprovider.Provider
	Tags             map[string]string
}

// SmtpUserName is the deterministic IAM user name. The magelift- prefix lets
// the deployer policy scope user management to MageLift-owned users.
func SmtpUserName(project, environment string) string {
	return "magelift-" + project + "-" + environment + "-ses-smtp"
}

// SmtpParameterName is the deterministic SSM parameter name for the SMTP
// credential JSON. Under /magelift/ so the deployer policy covers it.
func SmtpParameterName(project, environment string) string {
	return "/magelift/" + project + "-" + environment + "/ses-smtp"
}

// SmtpParameterARN renders the static parameter ARN for IAM policy and
// secret references. SSM ARNs carry no random suffix, unlike Secrets
// Manager, so the ARN is computable at plan time.
func SmtpParameterARN(region, account, project, environment string) string {
	return "arn:aws:ssm:" + region + ":" + account + ":parameter" + SmtpParameterName(project, environment)
}

type Component struct {
	pulumi.ResourceState
	IdentityARN  pulumi.StringOutput `pulumi:"identityArn"`
	SmtpHost     pulumi.StringOutput `pulumi:"smtpHost"`
	SmtpPort     pulumi.IntOutput    `pulumi:"smtpPort"`
	SmtpUsername pulumi.StringOutput `pulumi:"smtpUsername"`
	SecretARN    pulumi.StringOutput `pulumi:"secretArn"`
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResource(ComponentToken, name, component, opts...); err != nil {
		return nil, err
	}
	child := []pulumi.ResourceOption{pulumi.Parent(component)}
	if args.RegionalProvider != nil {
		child = append(child, pulumi.Provider(args.RegionalProvider))
	}
	tags := pulumi.ToStringMap(tags(args.Tags, name))

	identity, err := sesv2.NewEmailIdentity(ctx, name+"-identity", &sesv2.EmailIdentityArgs{
		EmailIdentity: pulumi.String(args.Domain),
		Tags:          tags,
	}, child...)
	if err != nil {
		return nil, fmt.Errorf("create SES email identity: %w", err)
	}
	tokens := identity.DkimSigningAttributes.Tokens()
	for i := 0; i < dkimTokenCount; i++ {
		token := tokens.Index(pulumi.Int(i))
		if _, err := route53.NewRecord(ctx, fmt.Sprintf("%s-dkim-%d", name, i), &route53.RecordArgs{
			ZoneId:  pulumi.String(args.HostedZoneID),
			Name:    pulumi.Sprintf("%s._domainkey.%s", token, args.Domain),
			Type:    pulumi.String("CNAME"),
			Ttl:     pulumi.Int(dkimTTL),
			Records: pulumi.StringArray{pulumi.Sprintf("%s.dkim.amazonses.com", token)},
		}, child...); err != nil {
			return nil, fmt.Errorf("create SES DKIM record: %w", err)
		}
	}

	user, err := iam.NewUser(ctx, name+"-smtp-user", &iam.UserArgs{
		Name: pulumi.String(SmtpUserName(args.Project, args.Environment)),
		Tags: tags,
	}, child...)
	if err != nil {
		return nil, fmt.Errorf("create SES SMTP user: %w", err)
	}
	key, err := iam.NewAccessKey(ctx, name+"-smtp-key", &iam.AccessKeyArgs{
		User: user.Name,
	}, child...)
	if err != nil {
		return nil, fmt.Errorf("create SES SMTP access key: %w", err)
	}
	policy, err := sendingPolicy()
	if err != nil {
		return nil, err
	}
	if _, err := iam.NewUserPolicy(ctx, name+"-sending-policy", &iam.UserPolicyArgs{
		User:   user.Name,
		Policy: pulumi.String(policy),
	}, child...); err != nil {
		return nil, fmt.Errorf("attach SES sending policy: %w", err)
	}

	password := key.Secret.ApplyT(func(secret string) string {
		return SmtpPasswordFromSecretAccessKey(secret, args.Region)
	}).(pulumi.StringOutput)
	username := key.ID().ApplyT(func(id pulumi.ID) string { return string(id) }).(pulumi.StringOutput)
	credentials := pulumi.All(username, password).ApplyT(func(values []any) (string, error) {
		return SmtpCredentialsJSON(values[0].(string), values[1].(string))
	}).(pulumi.StringOutput)
	parameter, err := ssm.NewParameter(ctx, name+"-smtp-credentials", &ssm.ParameterArgs{
		Name: pulumi.String(SmtpParameterName(args.Project, args.Environment)),
		Type: pulumi.String("SecureString"),
		// Untagged deliberately: tagging would need three more deployer
		// permissions for a name that already identifies the parameter.
		Value: credentials,
	}, child...)
	if err != nil {
		return nil, fmt.Errorf("store SES SMTP credentials: %w", err)
	}

	component.IdentityARN = identity.Arn
	component.SmtpHost = pulumi.String(SmtpEndpoint(args.Region)).ToStringOutput()
	component.SmtpPort = pulumi.Int(SmtpPort).ToIntOutput()
	component.SmtpUsername = username
	component.SecretARN = parameter.Arn
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"identityArn":  component.IdentityARN,
		"smtpHost":     component.SmtpHost,
		"smtpPort":     component.SmtpPort,
		"smtpUsername": component.SmtpUsername,
		"secretArn":    component.SecretARN,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

// sendingPolicy is the least-privilege SES sending grant. It is inline on
// the per-environment SMTP user (no shared group name to collide across
// environments) and allows only SendRawEmail, per the AWS SES SMTP guide.
func sendingPolicy() (string, error) {
	document := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect":   "Allow",
			"Action":   []string{"ses:SendRawEmail"},
			"Resource": "*",
		}},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode SES sending policy: %w", err)
	}
	return string(encoded), nil
}

func validate(name string, args Args) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("email component name is required")
	}
	if strings.TrimSpace(args.Project) == "" || strings.TrimSpace(args.Environment) == "" {
		return errors.New("email project and environment are required")
	}
	if !domainPattern.MatchString(strings.TrimSpace(args.Domain)) {
		return fmt.Errorf("email domain %q is not a valid DNS domain", args.Domain)
	}
	if !hostedZoneIDPattern.MatchString(strings.TrimSpace(args.HostedZoneID)) {
		return fmt.Errorf("email hosted zone ID %q is not a valid Route 53 zone ID", args.HostedZoneID)
	}
	if strings.TrimSpace(args.Region) == "" {
		return errors.New("email region is required")
	}
	return nil
}

func tags(input map[string]string, name string) map[string]string {
	merged := make(map[string]string, len(input)+1)
	for key, value := range input {
		merged[key] = value
	}
	merged["Name"] = name
	return merged
}
