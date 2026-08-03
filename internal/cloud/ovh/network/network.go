package network

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/ovh/pulumi-ovh/sdk/v2/go/ovh/cloudproject"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:ovh:Network"

type Args struct {
	Preset      sdk.PresetID
	ServiceName string
	Region      string
	NetworkCIDR string
	Zones       []string
	Labels      map[string]string
}

type Component struct {
	pulumi.ResourceState
	NetworkID          pulumi.StringOutput
	PrivateSubnetIDs   pulumi.StringArrayOutput
	PrivateSubnetNames pulumi.StringArrayOutput
	NetworkName        pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("network name is required")
	}
	if strings.TrimSpace(args.ServiceName) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("OVH service name and region are required")
	}
	prefix, err := validateSubnetCarve(args.NetworkCIDR, args.Zones)
	if err != nil {
		return nil, err
	}

	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"serviceName": pulumi.String(args.ServiceName),
		"region":      pulumi.String(args.Region),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	network, err := cloudproject.NewNetworkPrivate(ctx, name, &cloudproject.NetworkPrivateArgs{
		ServiceName: pulumi.String(args.ServiceName),
		Name:        pulumi.String(name),
		Regions:     pulumi.StringArray{pulumi.String(args.Region)},
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create OVH private network: %w", err)
	}

	subnetIDs := make(pulumi.StringArray, 0, len(args.Zones))
	subnetNames := make(pulumi.StringArray, 0, len(args.Zones))
	for index, zone := range args.Zones {
		cidr, err := subnetCIDR(prefix, index)
		if err != nil {
			return nil, err
		}
		subnetName := fmt.Sprintf("%s-%s", name, strings.ToLower(zone))
		subnet, err := cloudproject.NewNetworkPrivateSubnetV2(ctx, subnetName, &cloudproject.NetworkPrivateSubnetV2Args{
			ServiceName:     pulumi.String(args.ServiceName),
			NetworkId:       network.ID().ToStringOutput(),
			Name:            pulumi.String(subnetName),
			Region:          pulumi.String(args.Region),
			Cidr:            pulumi.String(cidr),
			Dhcp:            pulumi.Bool(true),
			EnableGatewayIp: pulumi.Bool(true),
		}, parent, pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return nil, fmt.Errorf("create OVH subnet %s: %w", zone, err)
		}
		subnetIDs = append(subnetIDs, subnet.ID().ToStringOutput())
		subnetNames = append(subnetNames, subnet.Name)
	}

	component.NetworkID = network.ID().ToStringOutput()
	component.NetworkName = network.Name
	component.PrivateSubnetIDs = subnetIDs.ToStringArrayOutput()
	component.PrivateSubnetNames = subnetNames.ToStringArrayOutput()

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"networkId":        component.NetworkID,
		"privateSubnetIds": component.PrivateSubnetIDs,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

// validateSubnetCarve ensures NetworkCIDR is IPv4 and can hold one /24 per zone.
func validateSubnetCarve(networkCIDR string, zones []string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(networkCIDR)
	if err != nil || !prefix.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("network CIDR must be IPv4: %w", err)
	}
	if len(zones) == 0 {
		return netip.Prefix{}, errors.New("at least one zone is required")
	}
	bits := prefix.Bits()
	if bits > 24 {
		return netip.Prefix{}, fmt.Errorf("network CIDR %s must be /24 or wider to carve per-zone /24 subnets", networkCIDR)
	}
	// subnetCIDR caps index at 15, so never advertise more than 16 slots even for a /16.
	available := 1 << (24 - bits)
	if available > 16 {
		available = 16
	}
	if len(zones) > available {
		return netip.Prefix{}, fmt.Errorf("network CIDR %s has room for %d /24 subnet(s) but %d zone(s) were requested", networkCIDR, available, len(zones))
	}
	return prefix, nil
}

func subnetCIDR(prefix netip.Prefix, index int) (string, error) {
	if index < 0 || index > 15 {
		return "", fmt.Errorf("subnet index %d out of range", index)
	}
	addr := prefix.Addr().As4()
	base := (uint32(addr[0]) << 24) | (uint32(addr[1]) << 16) | (uint32(addr[2]) << 8) | uint32(addr[3])
	bits := prefix.Bits()
	if bits > 24 {
		return "", fmt.Errorf("network CIDR must be /24 or wider for subnet carving")
	}
	step := uint32(1) << (32 - 24)
	next := base + uint32(index)*step
	return fmt.Sprintf("%d.%d.%d.%d/24", byte(next>>24), byte(next>>16), byte(next>>8), byte(next)), nil
}
