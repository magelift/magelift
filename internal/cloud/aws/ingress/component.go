package ingress

import (
	"errors"
	"regexp"
	"strings"

	"github.com/magelift/magelift/internal/cloud/aws/naming"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/lb"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:aws:Ingress"

var certificateARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):acm:([a-z0-9-]+):[0-9]{12}:certificate/[A-Za-z0-9-]+$`)

type Args struct {
	Region          string
	VPCID           pulumi.StringInput
	PublicSubnetIDs pulumi.StringArray
	SecurityGroupID pulumi.StringInput
	CertificateARN  pulumi.StringInput
	TargetPort      int
	HealthCheckPath string
	Provider        *awsprovider.Provider
	Tags            map[string]string
}

type Component struct {
	pulumi.ResourceState
	LoadBalancerARN       pulumi.StringOutput `pulumi:"loadBalancerArn"`
	DNSName               pulumi.StringOutput `pulumi:"dnsName"`
	LoadBalancerDimension pulumi.StringOutput `pulumi:"loadBalancerDimension"`
	TargetGroupARN        pulumi.StringOutput `pulumi:"targetGroupArn"`
	ListenerARN           pulumi.StringOutput `pulumi:"listenerArn"`
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	inputs := pulumi.Map{
		"region":          pulumi.String(args.Region),
		"vpcId":           args.VPCID,
		"publicSubnetIds": args.PublicSubnetIDs,
		"securityGroupId": args.SecurityGroupID,
		"certificateArn":  args.CertificateARN,
		"targetPort":      pulumi.Int(args.TargetPort),
		"healthCheckPath": pulumi.String(args.HealthCheckPath),
	}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, inputs, component, opts...); err != nil {
		return nil, err
	}
	child := []pulumi.ResourceOption{pulumi.Parent(component), pulumi.Provider(args.Provider)}
	loadBalancer, err := lb.NewLoadBalancer(ctx, name+"-alb", &lb.LoadBalancerArgs{
		Name:                    pulumi.String(naming.AWSName(name+"-alb", 32)),
		Internal:                pulumi.Bool(false),
		LoadBalancerType:        pulumi.String("application"),
		IpAddressType:           pulumi.String("ipv4"),
		Subnets:                 args.PublicSubnetIDs,
		SecurityGroups:          pulumi.StringArray{args.SecurityGroupID},
		DropInvalidHeaderFields: pulumi.Bool(true),
		EnableHttp2:             pulumi.Bool(true),
		Region:                  pulumi.String(args.Region),
		Tags:                    pulumi.ToStringMap(args.Tags),
	}, child...)
	if err != nil {
		return nil, err
	}
	targetGroup, err := lb.NewTargetGroup(ctx, name+"-web", &lb.TargetGroupArgs{
		Name:        pulumi.String(naming.AWSName(name+"-web", 32)),
		VpcId:       args.VPCID,
		Port:        pulumi.Int(args.TargetPort),
		Protocol:    pulumi.String("HTTP"),
		TargetType:  pulumi.String("ip"),
		Region:      pulumi.String(args.Region),
		HealthCheck: &lb.TargetGroupHealthCheckArgs{Enabled: pulumi.Bool(true), Path: pulumi.String(args.HealthCheckPath), Protocol: pulumi.String("HTTP"), Matcher: pulumi.String("200-399")},
		Tags:        pulumi.ToStringMap(args.Tags),
	}, child...)
	if err != nil {
		return nil, err
	}
	listener, err := lb.NewListener(ctx, name+"-https", &lb.ListenerArgs{
		LoadBalancerArn: loadBalancer.Arn,
		Port:            pulumi.Int(443),
		Protocol:        pulumi.String("HTTPS"),
		CertificateArn:  args.CertificateARN,
		SslPolicy:       pulumi.String("ELBSecurityPolicy-TLS13-1-2-2021-06"),
		Region:          pulumi.String(args.Region),
		DefaultActions: lb.ListenerDefaultActionArray{
			&lb.ListenerDefaultActionArgs{Type: pulumi.String("forward"), TargetGroupArn: targetGroup.Arn},
		},
		Tags: pulumi.ToStringMap(args.Tags),
	}, child...)
	if err != nil {
		return nil, err
	}
	component.LoadBalancerARN = loadBalancer.Arn
	component.DNSName = loadBalancer.DnsName
	component.LoadBalancerDimension = loadBalancer.ArnSuffix
	component.TargetGroupARN = targetGroup.Arn
	component.ListenerARN = listener.Arn
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"loadBalancerArn":       component.LoadBalancerARN,
		"dnsName":               component.DNSName,
		"loadBalancerDimension": component.LoadBalancerDimension,
		"targetGroupArn":        component.TargetGroupARN,
		"listenerArn":           component.ListenerARN,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func validate(name string, args Args) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" || args.Provider == nil {
		return errors.New("ingress name, region, and AWS provider are required")
	}
	if args.VPCID == nil || args.SecurityGroupID == nil {
		return errors.New("ingress VPC and security group are required")
	}
	if len(args.PublicSubnetIDs) < 2 {
		return errors.New("internet-facing ingress requires at least two public subnets")
	}
	knownSubnets := map[string]bool{}
	for _, subnet := range args.PublicSubnetIDs {
		if subnet == nil {
			return errors.New("internet-facing ingress public subnets cannot be nil")
		}
		if known, ok := subnet.(pulumi.String); ok {
			value := string(known)
			if strings.TrimSpace(value) == "" || knownSubnets[value] {
				return errors.New("internet-facing ingress public subnets must be non-empty and unique")
			}
			knownSubnets[value] = true
		}
	}
	if args.CertificateARN == nil {
		return errors.New("HTTPS ingress requires an explicit ACM certificate ARN")
	}
	if certificate, known := args.CertificateARN.(pulumi.String); known {
		matches := certificateARNPattern.FindStringSubmatch(string(certificate))
		if len(matches) != 2 || matches[1] != args.Region {
			return errors.New("HTTPS ingress certificate must be an explicit regional ACM certificate ARN")
		}
	}
	if args.TargetPort < 1 || args.TargetPort > 65535 || !strings.HasPrefix(args.HealthCheckPath, "/") {
		return errors.New("ingress target port and health check path are invalid")
	}
	return nil
}
