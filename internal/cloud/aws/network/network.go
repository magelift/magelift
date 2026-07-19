package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:aws:Network"

var existingVPCID = regexp.MustCompile(`^vpc-[A-Za-z0-9-]+$`)
var existingSubnetID = regexp.MustCompile(`^subnet-[A-Za-z0-9-]+$`)

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
	VpcID            pulumi.IDOutput
	PublicSubnetIDs  []pulumi.IDOutput
	PrivateSubnetIDs []pulumi.IDOutput
	DataSubnetIDs    []pulumi.IDOutput
	NATGatewayIDs    []pulumi.IDOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	cidrs, err := validate(args)
	if err != nil {
		return nil, err
	}
	component := &Component{}
	inputs := pulumi.Map{
		"preset":            pulumi.String(args.Preset),
		"region":            pulumi.String(args.Region),
		"vpcCidr":           pulumi.String(args.VPCCIDR),
		"availabilityZones": pulumi.ToStringArray(args.AvailabilityZones),
	}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, inputs, component, opts...); err != nil {
		return nil, err
	}
	if args.Existing != nil {
		if err := validateExistingNetwork(args, args.Existing); err != nil {
			return nil, err
		}
		component.VpcID = pulumi.ID(args.Existing.VPCID).ToIDOutput()
		component.PublicSubnetIDs = idInputs(args.Existing.PublicSubnetIDs)
		component.PrivateSubnetIDs = idInputs(args.Existing.PrivateSubnetIDs)
		component.DataSubnetIDs = idInputs(args.Existing.DataSubnetIDs)
		if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
			"vpcId": pulumi.String(args.Existing.VPCID), "publicSubnetIds": idArray(component.PublicSubnetIDs), "privateSubnetIds": idArray(component.PrivateSubnetIDs),
			"dataSubnetIds": idArray(component.DataSubnetIDs), "natGatewayIds": pulumi.Array{},
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

	natCount := 1
	if args.Preset != sdk.PresetPreview {
		natCount = len(args.AvailabilityZones)
	}
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
	}
	for index := range privateTables {
		natIndex := index
		if args.Preset == sdk.PresetPreview {
			natIndex = 0
		}
		_, err := ec2.NewRoute(ctx, name+"-private-default-"+fmt.Sprintf("%02d", index+1), &ec2.RouteArgs{
			RouteTableId: privateTables[index].ID(), DestinationCidrBlock: pulumi.String("0.0.0.0/0"), NatGatewayId: nats[natIndex].ID(), Region: pulumi.String(args.Region),
		}, child)
		if err != nil {
			return nil, err
		}
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
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func validate(args Args) ([]string, error) {
	// Aurora requires a DB subnet group spanning two zones, even for disposable
	// preview environments. Web workloads can still use only the first zone.
	wantZones := map[sdk.PresetID]int{sdk.PresetPreview: 2, sdk.PresetStandard: 2, sdk.PresetHighAvailability: 3}[args.Preset]
	standardQueueLayout := args.Preset == sdk.PresetStandard && len(args.AvailabilityZones) == 3
	if wantZones == 0 || (len(args.AvailabilityZones) != wantZones && !standardQueueLayout) {
		return nil, fmt.Errorf("preset %q requires exactly %d availability zones, or three for a standard RabbitMQ queue layout", args.Preset, wantZones)
	}
	if strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("AWS region is required")
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
	return subnetCIDRs(prefix, len(args.AvailabilityZones)*3), nil
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
	return result
}

func idArray(ids []pulumi.IDOutput) pulumi.Array {
	result := make(pulumi.Array, len(ids))
	for index := range ids {
		result[index] = ids[index]
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
