package network

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:aws:Network"

// NAT egress modes. nat-gateway is the certified managed path; fck-nat is an
// explicit low-cost alternative that runs a small EC2 instance as a NAT.
const (
	NatModeGateway = "nat-gateway"
	NatModeFckNat  = "fck-nat"

	NatTopologySingleAZ = "single-az"
	NatTopologyMultiAZ  = "multi-az"

	NatReplacementNone        = "none"
	NatReplacementAutoScaling = "auto-scaling"

	FckNatAMIOwner      = "568608671756"
	FckNatAMINamePrefix = "fck-nat-al2023-*-arm64-ebs"
	FckNatInstanceType  = "t4g.nano"
)

var existingVPCID = regexp.MustCompile(`^vpc-[A-Za-z0-9-]+$`)
var existingSubnetID = regexp.MustCompile(`^subnet-[A-Za-z0-9-]+$`)
var arm64NatInstanceType = regexp.MustCompile(`^(?:a1|[a-z0-9]+g[a-z0-9]*)\.[a-z0-9]+$`)

type GatewayEndpointService string
type InterfaceEndpointService string
type EndpointRationale string

const (
	GatewayEndpointS3        GatewayEndpointService   = "s3"
	InterfaceEndpointECRAPI  InterfaceEndpointService = "ecr.api"
	InterfaceEndpointECRDKR  InterfaceEndpointService = "ecr.dkr"
	InterfaceEndpointLogs    InterfaceEndpointService = "logs"
	InterfaceEndpointSecrets InterfaceEndpointService = "secretsmanager"
	InterfaceEndpointSSM     InterfaceEndpointService = "ssm"
	InterfaceEndpointSSMMsg  InterfaceEndpointService = "ssmmessages"
	InterfaceEndpointEC2Msg  InterfaceEndpointService = "ec2messages"
	ReduceNATCostAndExposure EndpointRationale        = "reduce-nat-cost-and-exposure"
)

type GatewayEndpoint struct {
	Service   GatewayEndpointService
	Rationale EndpointRationale
}

type InterfaceEndpoint struct {
	Service   InterfaceEndpointService
	Rationale EndpointRationale
}

type Args struct {
	Preset             sdk.PresetID
	Region             string
	VPCCIDR            string
	AvailabilityZones  []string
	NatMode            string
	NatTopology        string
	NatReplacementMode string
	NatInstanceType    string
	Existing           *ExistingNetwork
	GatewayEndpoints   []GatewayEndpoint
	InterfaceEndpoints []InterfaceEndpoint
	Tags               map[string]string
}

type ExistingNetwork struct {
	VPCID            string
	PublicSubnetIDs  []string
	PrivateSubnetIDs []string
	DataSubnetIDs    []string
}

type Component struct {
	pulumi.ResourceState
	VpcID                    pulumi.IDOutput
	PublicSubnetIDs          []pulumi.IDOutput
	PrivateSubnetIDs         []pulumi.IDOutput
	DataSubnetIDs            []pulumi.IDOutput
	NATGatewayIDs            []pulumi.IDOutput
	NATNetworkInterfaceIDs   []pulumi.StringOutput
	NATAutoScalingGroupNames []pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	cidrs, err := validate(args)
	if err != nil {
		return nil, err
	}
	component := &Component{}
	inputs := pulumi.Map{
		"preset":                pulumi.String(args.Preset),
		"region":                pulumi.String(args.Region),
		"vpcCidr":               pulumi.String(args.VPCCIDR),
		"availabilityZones":     pulumi.ToStringArray(args.AvailabilityZones),
		"natMode":               pulumi.String(resolveNatMode(args.NatMode)),
		"natTopology":           pulumi.String(resolveNatTopology(args.NatTopology, args.Preset)),
		"natReplacementMode":    pulumi.String(resolveNatReplacementMode(args.NatReplacementMode, args.Preset, args.NatMode, args.NatTopology)),
		"fckNatAmiOwner":        pulumi.String(FckNatAMIOwner),
		"fckNatAmiArchitecture": pulumi.String("arm64"),
		"fckNatInstanceType":    pulumi.String(resolveNatInstanceType(args.NatInstanceType, args.NatMode)),
	}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, inputs, component, opts...); err != nil {
		return nil, err
	}
	if args.Existing != nil {
		component.VpcID = pulumi.ID(args.Existing.VPCID).ToIDOutput()
		component.PublicSubnetIDs = idInputs(args.Existing.PublicSubnetIDs)
		component.PrivateSubnetIDs = idInputs(args.Existing.PrivateSubnetIDs)
		component.DataSubnetIDs = idInputs(args.Existing.DataSubnetIDs)
		if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
			"vpcId": pulumi.String(args.Existing.VPCID), "publicSubnetIds": idArray(component.PublicSubnetIDs), "privateSubnetIds": idArray(component.PrivateSubnetIDs),
			"dataSubnetIds": idArray(component.DataSubnetIDs), "natGatewayIds": pulumi.Array{}, "natNetworkInterfaceIds": pulumi.Array{}, "natAutoScalingGroupNames": pulumi.Array{},
		}); err != nil {
			return nil, err
		}
		return component, nil
	}
	child := pulumi.Parent(component)
	vpc, err := ec2.NewVpc(ctx, name+"-vpc", &ec2.VpcArgs{
		CidrBlock: pulumi.String(args.VPCCIDR), EnableDnsSupport: pulumi.Bool(true), EnableDnsHostnames: pulumi.Bool(true),
		Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "vpc", "", ""),
	}, child)
	if err != nil {
		return nil, err
	}
	component.VpcID = vpc.ID()
	igw, err := ec2.NewInternetGateway(ctx, name+"-igw", &ec2.InternetGatewayArgs{
		VpcId: vpc.ID(), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "internet-gateway", "", ""),
	}, child)
	if err != nil {
		return nil, err
	}

	publicTables := make([]*ec2.RouteTable, len(args.AvailabilityZones))
	privateTables := make([]*ec2.RouteTable, len(args.AvailabilityZones))
	dataTables := make([]*ec2.RouteTable, len(args.AvailabilityZones))
	for index, zone := range args.AvailabilityZones {
		suffix := fmt.Sprintf("%02d", index+1)
		public, err := subnet(ctx, name, "public", suffix, zone, cidrs[index*3], vpc, args, component)
		if err != nil {
			return nil, err
		}
		private, err := subnet(ctx, name, "private", suffix, zone, cidrs[index*3+1], vpc, args, component)
		if err != nil {
			return nil, err
		}
		data, err := subnet(ctx, name, "data", suffix, zone, cidrs[index*3+2], vpc, args, component)
		if err != nil {
			return nil, err
		}
		component.PublicSubnetIDs = append(component.PublicSubnetIDs, public.ID())
		component.PrivateSubnetIDs = append(component.PrivateSubnetIDs, private.ID())
		component.DataSubnetIDs = append(component.DataSubnetIDs, data.ID())
		publicTables[index], err = routeTable(ctx, name, "public", suffix, zone, vpc, args, component)
		if err != nil {
			return nil, err
		}
		privateTables[index], err = routeTable(ctx, name, "private", suffix, zone, vpc, args, component)
		if err != nil {
			return nil, err
		}
		dataTables[index], err = routeTable(ctx, name, "data", suffix, zone, vpc, args, component)
		if err != nil {
			return nil, err
		}
		for tier, pair := range map[string]struct {
			subnet *ec2.Subnet
			table  *ec2.RouteTable
		}{
			"public": {public, publicTables[index]}, "private": {private, privateTables[index]}, "data": {data, dataTables[index]},
		} {
			_, err = ec2.NewRouteTableAssociation(ctx, name+"-"+tier+"-association-"+suffix, &ec2.RouteTableAssociationArgs{
				SubnetId: pair.subnet.ID(), RouteTableId: pair.table.ID(), Region: pulumi.String(args.Region),
			}, child)
			if err != nil {
				return nil, err
			}
		}
		_, err = ec2.NewRoute(ctx, name+"-public-default-"+suffix, &ec2.RouteArgs{
			RouteTableId: publicTables[index].ID(), DestinationCidrBlock: pulumi.String("0.0.0.0/0"), GatewayId: igw.ID(), Region: pulumi.String(args.Region),
		}, child)
		if err != nil {
			return nil, err
		}
	}

	natTopology := resolveNatTopology(args.NatTopology, args.Preset)
	natCount := 1
	if natTopology == NatTopologyMultiAZ {
		natCount = len(args.AvailabilityZones)
	}
	natMode := resolveNatMode(args.NatMode)
	natReplacementMode := resolveNatReplacementMode(args.NatReplacementMode, args.Preset, natMode, natTopology)
	var privateDefaultRouteTargets []pulumi.StringOutput
	switch natMode {
	case NatModeGateway:
		nats := make([]*ec2.NatGateway, natCount)
		for index := 0; index < natCount; index++ {
			suffix := fmt.Sprintf("%02d", index+1)
			eip, err := ec2.NewEip(ctx, name+"-nat-eip-"+suffix, &ec2.EipArgs{
				Domain: pulumi.String("vpc"), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "nat-eip", suffix, args.AvailabilityZones[index]),
			}, child, pulumi.DependsOn([]pulumi.Resource{igw}))
			if err != nil {
				return nil, err
			}
			nats[index], err = ec2.NewNatGateway(ctx, name+"-nat-"+suffix, &ec2.NatGatewayArgs{
				AllocationId: eip.ID(), SubnetId: component.PublicSubnetIDs[index], ConnectivityType: pulumi.String("public"), AvailabilityMode: pulumi.String("zonal"),
				Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "nat-gateway", suffix, args.AvailabilityZones[index]),
			}, child, pulumi.DependsOn([]pulumi.Resource{igw}))
			if err != nil {
				return nil, err
			}
			component.NATGatewayIDs = append(component.NATGatewayIDs, nats[index].ID())
			privateDefaultRouteTargets = append(privateDefaultRouteTargets, nats[index].ID().ToStringOutput())
		}
		for index := range privateTables {
			natIndex := index
			if natTopology == NatTopologySingleAZ {
				natIndex = 0
			}
			_, err := ec2.NewRoute(ctx, name+"-private-default-"+fmt.Sprintf("%02d", index+1), &ec2.RouteArgs{
				RouteTableId: privateTables[index].ID(), DestinationCidrBlock: pulumi.String("0.0.0.0/0"), NatGatewayId: privateDefaultRouteTargets[natIndex], Region: pulumi.String(args.Region),
			}, child)
			if err != nil {
				return nil, err
			}
		}
	case NatModeFckNat:
		ami, err := ec2.LookupAmi(ctx, &ec2.LookupAmiArgs{
			Owners:     []string{FckNatAMIOwner},
			MostRecent: pulumi.BoolRef(true),
			Filters: []ec2.GetAmiFilter{
				{Name: "name", Values: []string{FckNatAMINamePrefix}},
				{Name: "state", Values: []string{"available"}},
				{Name: "architecture", Values: []string{"arm64"}},
			},
		}, nil)
		if err != nil {
			return nil, fmt.Errorf("lookup fck-nat AMI: %w", err)
		}
		securityGroup, err := ec2.NewSecurityGroup(ctx, name+"-fck-nat-sg", &ec2.SecurityGroupArgs{
			VpcId:       vpc.ID(),
			Description: pulumi.String("MageLift fck-nat egress instance"),
			Ingress: ec2.SecurityGroupIngressArray{
				ec2.SecurityGroupIngressArgs{Protocol: pulumi.String("-1"), FromPort: pulumi.Int(0), ToPort: pulumi.Int(0), CidrBlocks: pulumi.StringArray{pulumi.String(args.VPCCIDR)}},
			},
			Egress: ec2.SecurityGroupEgressArray{
				ec2.SecurityGroupEgressArgs{Protocol: pulumi.String("-1"), FromPort: pulumi.Int(0), ToPort: pulumi.Int(0), CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")}},
			},
			Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "fck-nat-security-group", "", ""),
		}, child)
		if err != nil {
			return nil, err
		}
		eniIDs := make([]pulumi.StringOutput, natCount)
		if natReplacementMode == NatReplacementAutoScaling {
			profile, policy, err := newFckNatInstanceProfile(ctx, name, args, component)
			if err != nil {
				return nil, err
			}
			for index := 0; index < natCount; index++ {
				suffix := fmt.Sprintf("%02d", index+1)
				fixedInterface, err := ec2.NewNetworkInterface(ctx, name+"-fck-nat-eni-"+suffix, &ec2.NetworkInterfaceArgs{
					SubnetId: component.PublicSubnetIDs[index], SecurityGroups: pulumi.StringArray{securityGroup.ID()}, SourceDestCheck: pulumi.Bool(false),
					Description: pulumi.String("MageLift fck-nat stable egress interface"), Region: pulumi.String(args.Region),
					Tags: tags(args.Tags, name, "fck-nat-eni", suffix, args.AvailabilityZones[index]),
				}, child, pulumi.DependsOn([]pulumi.Resource{igw}))
				if err != nil {
					return nil, err
				}
				component.NATNetworkInterfaceIDs = append(component.NATNetworkInterfaceIDs, fixedInterface.ID().ToStringOutput())
				eniIDs[index] = fixedInterface.ID().ToStringOutput()
				eip, err := ec2.NewEip(ctx, name+"-fck-nat-eip-"+suffix, &ec2.EipArgs{
					Domain: pulumi.String("vpc"), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "fck-nat-eip", suffix, args.AvailabilityZones[index]),
				}, child, pulumi.DependsOn([]pulumi.Resource{igw}))
				if err != nil {
					return nil, err
				}
				if _, err = ec2.NewEipAssociation(ctx, name+"-fck-nat-eip-assoc-"+suffix, &ec2.EipAssociationArgs{
					AllocationId: eip.ID(), NetworkInterfaceId: fixedInterface.ID(), Region: pulumi.String(args.Region),
				}, child); err != nil {
					return nil, err
				}
				launchTemplate, err := ec2.NewLaunchTemplate(ctx, name+"-fck-nat-launch-template-"+suffix, &ec2.LaunchTemplateArgs{
					ImageId: pulumi.String(ami.Id), InstanceType: pulumi.String(resolveNatInstanceType(args.NatInstanceType, args.NatMode)),
					IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileArgs{Name: profile.Name},
					NetworkInterfaces: ec2.LaunchTemplateNetworkInterfaceArray{
						&ec2.LaunchTemplateNetworkInterfaceArgs{
							DeviceIndex: pulumi.Int(0), SubnetId: component.PublicSubnetIDs[index], AssociatePublicIpAddress: pulumi.String("true"),
							SecurityGroups: pulumi.StringArray{securityGroup.ID()}, DeleteOnTermination: pulumi.String("true"),
						},
					},
					UserData: fckNatUserData(fixedInterface.ID()), Region: pulumi.String(args.Region),
					Tags: tags(args.Tags, name, "fck-nat-launch-template", suffix, args.AvailabilityZones[index]),
					TagSpecifications: ec2.LaunchTemplateTagSpecificationArray{
						&ec2.LaunchTemplateTagSpecificationArgs{ResourceType: pulumi.String("instance"), Tags: tags(args.Tags, name, "fck-nat", suffix, args.AvailabilityZones[index])},
					},
				}, child, pulumi.DependsOn([]pulumi.Resource{igw, policy, fixedInterface}))
				if err != nil {
					return nil, err
				}
				group, err := autoscaling.NewGroup(ctx, name+"-fck-nat-asg-"+suffix, &autoscaling.GroupArgs{
					MinSize: pulumi.Int(1), DesiredCapacity: pulumi.Int(1), MaxSize: pulumi.Int(1),
					VpcZoneIdentifiers: pulumi.StringArray{component.PublicSubnetIDs[index]},
					LaunchTemplate:     &autoscaling.GroupLaunchTemplateArgs{Id: launchTemplate.ID(), Version: pulumi.String("$Latest")},
					HealthCheckType:    pulumi.String("EC2"), HealthCheckGracePeriod: pulumi.Int(300), DefaultInstanceWarmup: pulumi.Int(300),
					ForceDelete: pulumi.Bool(false), WaitForCapacityTimeout: pulumi.String("15m"), Region: pulumi.String(args.Region),
					TerminationPolicies: pulumi.StringArray{pulumi.String("OldestInstance")}, Tags: groupTags(args.Tags, name, "fck-nat", suffix, args.AvailabilityZones[index]),
				}, child, pulumi.DependsOn([]pulumi.Resource{launchTemplate, policy, fixedInterface}))
				if err != nil {
					return nil, err
				}
				component.NATAutoScalingGroupNames = append(component.NATAutoScalingGroupNames, group.Name)
			}
		} else {
			for index := 0; index < natCount; index++ {
				suffix := fmt.Sprintf("%02d", index+1)
				eip, err := ec2.NewEip(ctx, name+"-fck-nat-eip-"+suffix, &ec2.EipArgs{
					Domain: pulumi.String("vpc"), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "fck-nat-eip", suffix, args.AvailabilityZones[index]),
				}, child, pulumi.DependsOn([]pulumi.Resource{igw}))
				if err != nil {
					return nil, err
				}
				instance, err := ec2.NewInstance(ctx, name+"-fck-nat-"+suffix, &ec2.InstanceArgs{
					Ami: pulumi.String(ami.Id), InstanceType: pulumi.String(resolveNatInstanceType(args.NatInstanceType, args.NatMode)), SubnetId: component.PublicSubnetIDs[index],
					VpcSecurityGroupIds: pulumi.StringArray{securityGroup.ID()}, SourceDestCheck: pulumi.Bool(false),
					AssociatePublicIpAddress: pulumi.Bool(true), Region: pulumi.String(args.Region),
					Tags: tags(args.Tags, name, "fck-nat", suffix, args.AvailabilityZones[index]),
				}, child, pulumi.DependsOn([]pulumi.Resource{igw}))
				if err != nil {
					return nil, err
				}
				if _, err = ec2.NewEipAssociation(ctx, name+"-fck-nat-eip-assoc-"+suffix, &ec2.EipAssociationArgs{
					AllocationId: eip.ID(), InstanceId: instance.ID(), Region: pulumi.String(args.Region),
				}, child); err != nil {
					return nil, err
				}
				eniIDs[index] = instance.PrimaryNetworkInterfaceId
				component.NATNetworkInterfaceIDs = append(component.NATNetworkInterfaceIDs, instance.PrimaryNetworkInterfaceId)
			}
		}
		for index := range privateTables {
			natIndex := index
			if natTopology == NatTopologySingleAZ {
				natIndex = 0
			}
			_, err := ec2.NewRoute(ctx, name+"-private-default-"+fmt.Sprintf("%02d", index+1), &ec2.RouteArgs{
				RouteTableId: privateTables[index].ID(), DestinationCidrBlock: pulumi.String("0.0.0.0/0"), NetworkInterfaceId: eniIDs[natIndex], Region: pulumi.String(args.Region),
			}, child)
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("unsupported natMode %q", natMode)
	}

	for _, endpoint := range args.GatewayEndpoints {
		routeTables := pulumi.StringArray{}
		for _, table := range append(privateTables, dataTables...) {
			routeTables = append(routeTables, table.ID())
		}
		_, err := ec2.NewVpcEndpoint(ctx, name+"-endpoint-"+string(endpoint.Service), &ec2.VpcEndpointArgs{
			VpcId: vpc.ID(), ServiceName: pulumi.Sprintf("com.amazonaws.%s.%s", args.Region, endpoint.Service), VpcEndpointType: pulumi.String("Gateway"),
			RouteTableIds: routeTables, Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "gateway-endpoint", string(endpoint.Service), ""),
		}, child)
		if err != nil {
			return nil, err
		}
	}
	if len(args.InterfaceEndpoints) > 0 {
		endpointSecurityGroup, err := ec2.NewSecurityGroup(ctx, name+"-interface-endpoints-sg", &ec2.SecurityGroupArgs{
			VpcId: vpc.ID(), Description: pulumi.String("MageLift private interface endpoints"),
			Ingress: ec2.SecurityGroupIngressArray{ec2.SecurityGroupIngressArgs{Protocol: pulumi.String("tcp"), FromPort: pulumi.Int(443), ToPort: pulumi.Int(443), CidrBlocks: pulumi.StringArray{pulumi.String(args.VPCCIDR)}}},
			Egress:  ec2.SecurityGroupEgressArray{ec2.SecurityGroupEgressArgs{Protocol: pulumi.String("-1"), FromPort: pulumi.Int(0), ToPort: pulumi.Int(0), CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")}}},
			Region:  pulumi.String(args.Region), Tags: tags(args.Tags, name, "interface-endpoints-security-group", "", ""),
		}, child)
		if err != nil {
			return nil, err
		}
		interfaceSubnets := pulumi.StringArray{}
		for _, subnet := range component.PrivateSubnetIDs {
			interfaceSubnets = append(interfaceSubnets, subnet)
		}
		for _, subnet := range component.DataSubnetIDs {
			interfaceSubnets = append(interfaceSubnets, subnet)
		}
		interfaceEndpoints := append([]InterfaceEndpoint(nil), args.InterfaceEndpoints...)
		sort.Slice(interfaceEndpoints, func(i, j int) bool { return interfaceEndpoints[i].Service < interfaceEndpoints[j].Service })
		for _, endpoint := range interfaceEndpoints {
			_, err := ec2.NewVpcEndpoint(ctx, name+"-interface-endpoint-"+string(endpoint.Service), &ec2.VpcEndpointArgs{
				VpcId: vpc.ID(), ServiceName: pulumi.Sprintf("com.amazonaws.%s.%s", args.Region, endpoint.Service), VpcEndpointType: pulumi.String("Interface"),
				PrivateDnsEnabled: pulumi.Bool(true), SubnetIds: interfaceSubnets, SecurityGroupIds: pulumi.StringArray{endpointSecurityGroup.ID()}, Region: pulumi.String(args.Region),
				Tags: tags(args.Tags, name, "interface-endpoint", string(endpoint.Service), ""),
			}, child)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"vpcId": vpc.ID(), "publicSubnetIds": idArray(component.PublicSubnetIDs), "privateSubnetIds": idArray(component.PrivateSubnetIDs),
		"dataSubnetIds": idArray(component.DataSubnetIDs), "natGatewayIds": idArray(component.NATGatewayIDs),
		"natNetworkInterfaceIds": stringArray(component.NATNetworkInterfaceIDs), "natAutoScalingGroupNames": stringArray(component.NATAutoScalingGroupNames),
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func validate(args Args) ([]string, error) {
	if strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("AWS region is required")
	}
	natMode := args.NatMode
	if natMode = resolveNatMode(natMode); natMode != NatModeGateway && natMode != NatModeFckNat {
		return nil, errors.New("natMode must be nat-gateway or fck-nat")
	}
	natTopology := resolveNatTopology(args.NatTopology, args.Preset)
	if natTopology != NatTopologySingleAZ && natTopology != NatTopologyMultiAZ {
		return nil, errors.New("natTopology must be single-az or multi-az")
	}
	natReplacementMode := resolveNatReplacementMode(args.NatReplacementMode, args.Preset, natMode, natTopology)
	if natReplacementMode != NatReplacementNone && natReplacementMode != NatReplacementAutoScaling {
		return nil, errors.New("natReplacementMode must be none or auto-scaling")
	}
	if natMode == NatModeGateway && natReplacementMode != NatReplacementNone {
		return nil, errors.New("natReplacementMode is only supported with fck-nat")
	}
	if instanceType := strings.TrimSpace(args.NatInstanceType); instanceType != "" {
		if natMode != NatModeFckNat {
			return nil, errors.New("natInstanceType is only supported with fck-nat")
		}
		if !arm64NatInstanceType.MatchString(instanceType) {
			return nil, errors.New("natInstanceType must be an ARM64-compatible Graviton instance type")
		}
	}
	seen := map[string]bool{}
	for _, zone := range args.AvailabilityZones {
		if strings.TrimSpace(zone) == "" || seen[zone] {
			return nil, errors.New("availability zones must be non-empty and unique")
		}
		seen[zone] = true
	}
	for _, endpoint := range args.GatewayEndpoints {
		if endpoint.Service != GatewayEndpointS3 || endpoint.Rationale != ReduceNATCostAndExposure {
			return nil, errors.New("gateway endpoints require a supported service and cost/exposure rationale")
		}
	}
	seenInterfaces := map[InterfaceEndpointService]bool{}
	for _, endpoint := range args.InterfaceEndpoints {
		if endpoint.Rationale != ReduceNATCostAndExposure || !validInterfaceEndpoint(endpoint.Service) || seenInterfaces[endpoint.Service] {
			return nil, errors.New("interface endpoints require supported services, a cost/exposure rationale, and no duplicates")
		}
		seenInterfaces[endpoint.Service] = true
	}
	prefix, err := netip.ParsePrefix(args.VPCCIDR)
	if err != nil || !prefix.Addr().Is4() || prefix != prefix.Masked() || prefix.Bits() > 24 {
		return nil, errors.New("VPC CIDR must be a canonical IPv4 prefix of /24 or larger")
	}
	// Carve capacity before preset zone counts so a user-supplied list that
	// would emit CIDRs outside the VPC fails closed with the isolation error
	// rather than a preset-policy message (six or more zones demand >16 blocks).
	if err := validateCarveCapacity(prefix, len(args.AvailabilityZones)); err != nil {
		return nil, err
	}
	// Aurora requires a DB subnet group spanning two zones, even for disposable
	// preview environments. Web workloads can still use only the first zone.
	wantZones := map[sdk.PresetID]int{sdk.PresetPreview: 2, sdk.PresetStandard: 2, sdk.PresetHighAvailability: 3}[args.Preset]
	queueLayout3AZ := (args.Preset == sdk.PresetPreview || args.Preset == sdk.PresetStandard) && len(args.AvailabilityZones) == 3
	if wantZones == 0 || (len(args.AvailabilityZones) != wantZones && !queueLayout3AZ) {
		return nil, fmt.Errorf("preset %q requires exactly %d availability zones, or three for an Amazon MQ queue layout", args.Preset, wantZones)
	}
	if args.Existing != nil {
		if strings.TrimSpace(args.NatMode) == NatModeFckNat || strings.TrimSpace(args.NatTopology) != "" || strings.TrimSpace(args.NatReplacementMode) != "" || strings.TrimSpace(args.NatInstanceType) != "" {
			return nil, errors.New("existing network owns egress; fck-nat and NAT topology/replacement settings cannot be selected")
		}
		if err := validateExistingNetwork(args, args.Existing); err != nil {
			return nil, err
		}
	}
	return subnetCIDRs(prefix, len(args.AvailabilityZones)*3), nil
}

func resolveNatMode(value string) string {
	if strings.TrimSpace(value) == "" {
		return NatModeGateway
	}
	return strings.TrimSpace(value)
}

func resolveNatTopology(value string, preset sdk.PresetID) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if preset == sdk.PresetPreview {
		return NatTopologySingleAZ
	}
	return NatTopologyMultiAZ
}

func resolveNatReplacementMode(value string, preset sdk.PresetID, natMode, topology string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if resolveNatMode(natMode) != NatModeFckNat || resolveNatTopology(topology, preset) != NatTopologyMultiAZ {
		return NatReplacementNone
	}
	return NatReplacementAutoScaling
}

func resolveNatInstanceType(value, natMode string) string {
	if resolveNatMode(natMode) != NatModeFckNat {
		return ""
	}
	if strings.TrimSpace(value) == "" {
		return FckNatInstanceType
	}
	return strings.TrimSpace(value)
}

// ResolveNatTopology returns the stable topology used by the AWS plan when
// YAML omits the provider-specific choice.
func ResolveNatTopology(value string, preset sdk.PresetID) string {
	return resolveNatTopology(value, preset)
}

// ResolveNatReplacementMode returns the stable fck-nat repair policy used by
// the AWS plan when YAML omits the provider-specific choice.
func ResolveNatReplacementMode(value string, preset sdk.PresetID, natMode, topology string) string {
	return resolveNatReplacementMode(value, preset, natMode, topology)
}

func newFckNatInstanceProfile(ctx *pulumi.Context, name string, args Args, parent pulumi.Resource) (*iam.InstanceProfile, *iam.RolePolicy, error) {
	role, err := iam.NewRole(ctx, name+"-fck-nat-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(ec2AssumeRolePolicy), Description: pulumi.String("MageLift fck-nat attachment role"),
		Tags: tags(args.Tags, name, "fck-nat-role", "", ""),
	}, pulumi.Parent(parent))
	if err != nil {
		return nil, nil, err
	}
	policy, err := iam.NewRolePolicy(ctx, name+"-fck-nat-role-policy", &iam.RolePolicyArgs{
		Role: role.Name, Policy: pulumi.String(fckNatRolePolicy), Name: pulumi.String(name + "-fck-nat-attachment"),
	}, pulumi.Parent(parent))
	if err != nil {
		return nil, nil, err
	}
	profile, err := iam.NewInstanceProfile(ctx, name+"-fck-nat-instance-profile", &iam.InstanceProfileArgs{
		Role: role.Name, Name: pulumi.String(name + "-fck-nat"), Tags: tags(args.Tags, name, "fck-nat-instance-profile", "", ""),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{policy}))
	if err != nil {
		return nil, nil, err
	}
	return profile, policy, nil
}

const ec2AssumeRolePolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

const fckNatRolePolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["ec2:AttachNetworkInterface","ec2:ModifyNetworkInterfaceAttribute","ec2:AssociateAddress","ec2:DisassociateAddress"],"Resource":"*"}]}`

func fckNatUserData(networkInterfaceID pulumi.StringInput) pulumi.StringOutput {
	return networkInterfaceID.ToStringOutput().ApplyT(func(id string) string {
		content := fmt.Sprintf("#!/bin/bash\nset -eu\necho \"eni_id=%s\" >> /etc/fck-nat.conf\nsystemctl restart fck-nat\n", id)
		return base64.StdEncoding.EncodeToString([]byte(content))
	}).(pulumi.StringOutput)
}

func groupTags(input map[string]string, component, role, suffix, zone string) autoscaling.GroupTagArray {
	values := tags(input, component, role, suffix, zone)
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make(autoscaling.GroupTagArray, 0, len(keys))
	for _, key := range keys {
		result = append(result, &autoscaling.GroupTagArgs{Key: pulumi.String(key), Value: values[key], PropagateAtLaunch: pulumi.Bool(true)})
	}
	return result
}

// validateCarveCapacity rejects a zone count whose public/private/data carve
// would demand more blocks than prefix.Bits()+4 can hold.
func validateCarveCapacity(prefix netip.Prefix, zoneCount int) error {
	const blocksPerZone = 3
	bits := prefix.Bits() + 4
	available := 1 << uint(bits-prefix.Bits())
	demand := zoneCount * blocksPerZone
	if demand > available {
		return fmt.Errorf("VPC CIDR %s has room for %d subnet blocks but %d are demanded (%d zones × %d)", prefix, available, demand, zoneCount, blocksPerZone)
	}
	return nil
}

func validateExistingNetwork(args Args, existing *ExistingNetwork) error {
	if existing == nil || !existingVPCID.MatchString(existing.VPCID) {
		return errors.New("existing VPC ID is invalid")
	}
	if len(existing.PublicSubnetIDs) != len(args.AvailabilityZones) || len(existing.PrivateSubnetIDs) != len(args.AvailabilityZones) || len(existing.DataSubnetIDs) != len(args.AvailabilityZones) {
		return errors.New("existing VPC requires one public, private, and data subnet per availability zone")
	}
	for _, group := range []struct {
		name   string
		values []string
	}{
		{name: "public", values: existing.PublicSubnetIDs},
		{name: "private", values: existing.PrivateSubnetIDs},
		{name: "data", values: existing.DataSubnetIDs},
	} {
		seen := map[string]struct{}{}
		for _, value := range group.values {
			if !existingSubnetID.MatchString(value) {
				return fmt.Errorf("existing %s subnet ID %q is invalid", group.name, value)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("existing %s subnet IDs must be unique", group.name)
			}
			seen[value] = struct{}{}
		}
	}
	if len(args.GatewayEndpoints) != 0 || len(args.InterfaceEndpoints) != 0 {
		return errors.New("existing VPC imports cannot create network endpoints")
	}
	return nil
}

func validInterfaceEndpoint(service InterfaceEndpointService) bool {
	switch service {
	case InterfaceEndpointECRAPI, InterfaceEndpointECRDKR, InterfaceEndpointLogs, InterfaceEndpointSecrets, InterfaceEndpointSSM, InterfaceEndpointSSMMsg, InterfaceEndpointEC2Msg:
		return true
	default:
		return false
	}
}

func subnetCIDRs(prefix netip.Prefix, count int) []string {
	bits := prefix.Bits() + 4
	base := binary.BigEndian.Uint32(prefix.Addr().AsSlice())
	step := uint32(1) << uint32(32-bits)
	result := make([]string, count)
	for index := range result {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], base+uint32(index)*step)
		result[index] = netip.PrefixFrom(netip.AddrFrom4(raw), bits).String()
	}
	return result
}

func subnet(ctx *pulumi.Context, name, tier, suffix, zone, cidr string, vpc *ec2.Vpc, args Args, parent pulumi.Resource) (*ec2.Subnet, error) {
	return ec2.NewSubnet(ctx, name+"-"+tier+"-subnet-"+suffix, &ec2.SubnetArgs{VpcId: vpc.ID(), CidrBlock: pulumi.String(cidr), AvailabilityZone: pulumi.String(zone), MapPublicIpOnLaunch: pulumi.Bool(false), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, tier+"-subnet", suffix, zone)}, pulumi.Parent(parent))
}

func routeTable(ctx *pulumi.Context, name, tier, suffix, zone string, vpc *ec2.Vpc, args Args, parent pulumi.Resource) (*ec2.RouteTable, error) {
	return ec2.NewRouteTable(ctx, name+"-"+tier+"-routes-"+suffix, &ec2.RouteTableArgs{VpcId: vpc.ID(), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, tier+"-routes", suffix, zone)}, pulumi.Parent(parent))
}

func tags(input map[string]string, component, role, suffix, zone string) pulumi.StringMap {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := pulumi.StringMap{}
	for _, key := range keys {
		result[key] = pulumi.String(input[key])
	}
	result["Name"] = pulumi.String(strings.Trim(strings.Join([]string{component, role, suffix}, "-"), "-"))
	result["magelift:component"] = pulumi.String(component)
	result["magelift:role"] = pulumi.String(role)
	if zone != "" {
		result["magelift:availability-zone"] = pulumi.String(zone)
	}
	for key, value := range kubernetesSubnetDiscoveryTags(role) {
		result[key] = pulumi.String(value)
	}
	return result
}

// kubernetesSubnetDiscoveryTags advertises public and private subnets to EKS
// Auto Mode NLB discovery. Without kubernetes.io/role/elb=1, an internet-facing
// Service stays on private subnets (or fails to find a public subnet at all).
func kubernetesSubnetDiscoveryTags(role string) map[string]string {
	switch role {
	case "public-subnet":
		return map[string]string{"kubernetes.io/role/elb": "1"}
	case "private-subnet":
		return map[string]string{"kubernetes.io/role/internal-elb": "1"}
	default:
		return nil
	}
}

func idArray(ids []pulumi.IDOutput) pulumi.Array {
	result := make(pulumi.Array, len(ids))
	for index := range ids {
		result[index] = ids[index]
	}
	return result
}

func stringArray(values []pulumi.StringOutput) pulumi.Array {
	result := make(pulumi.Array, len(values))
	for index := range values {
		result[index] = values[index]
	}
	return result
}

func idInputs(values []string) []pulumi.IDOutput {
	result := make([]pulumi.IDOutput, len(values))
	for index, value := range values {
		result[index] = pulumi.ID(value).ToIDOutput()
	}
	return result
}
