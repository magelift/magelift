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
	GatewayID          pulumi.StringOutput
	GatewayIP          pulumi.StringOutput
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

	component := &Component{
		// Keep optional outputs concrete when this region uses floating IPs and
		// no custom gateway resource is created.
		GatewayID: pulumi.String("").ToStringOutput(),
		GatewayIP: pulumi.String("").ToStringOutput(),
	}
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
	subnets := make([]pulumi.Resource, 0, len(args.Zones))
	for index, zone := range args.Zones {
		cidr, err := subnetCIDR(prefix, index)
		if err != nil {
			return nil, err
		}
		gatewayIP, err := subnetGatewayIP(cidr)
		if err != nil {
			return nil, err
		}
		subnetName := fmt.Sprintf("%s-%s", name, strings.ToLower(zone))
		subnet, err := cloudproject.NewNetworkPrivateSubnetV2(ctx, subnetName, &cloudproject.NetworkPrivateSubnetV2Args{
			ServiceName: pulumi.String(args.ServiceName),
			// OVH's global private-network ID has the pn-* form. Regional
			// subnet, database, and MKS APIs require the region's OpenStack UUID.
			NetworkId:       network.RegionsOpenstackIds.MapIndex(pulumi.String(args.Region)),
			Name:            pulumi.String(subnetName),
			Region:          pulumi.String(args.Region),
			Cidr:            pulumi.String(cidr),
			Dhcp:            pulumi.Bool(true),
			EnableGatewayIp: pulumi.Bool(true),
			GatewayIp:       pulumi.String(gatewayIP),
		}, parent, pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return nil, fmt.Errorf("create OVH subnet %s: %w", zone, err)
		}
		subnets = append(subnets, subnet)
		subnetIDs = append(subnetIDs, subnet.ID().ToStringOutput())
		subnetNames = append(subnetNames, subnet.Name)
	}

	// Consumers of this component call regional OVH APIs, which expect the
	// OpenStack UUID rather than NetworkPrivate's global pn-* resource ID.
	component.NetworkID = network.RegionsOpenstackIds.MapIndex(pulumi.String(args.Region))
	// MKS requires a real OpenStack gateway on a nodes subnet. The subnet's
	// gatewayIp is only DHCP metadata and does not satisfy that requirement.
	// Keep the gateway inside this component so it is deleted with the test
	// network and never becomes an untracked billable orphan.
	gateway, err := cloudproject.NewGateway(ctx, name+"-gateway", &cloudproject.GatewayArgs{
		ServiceName: pulumi.String(args.ServiceName),
		Name:        pulumi.String(name + "-gateway"),
		Model:       pulumi.String("s"),
		NetworkId:   network.RegionsOpenstackIds.MapIndex(pulumi.String(args.Region)),
		Region:      pulumi.String(args.Region),
		SubnetId:    subnetIDs[0],
	}, parent, pulumi.DependsOn(subnets))
	if err != nil {
		return nil, fmt.Errorf("create OVH network gateway: %w", err)
	}
	component.GatewayID = gateway.ID().ToStringOutput()
	// OVH MKS expects the gateway IP advertised by the nodes subnet, not the
	// gateway resource ID. The first subnet is the MKS nodes subnet.
	firstSubnetCIDR, err := subnetCIDR(prefix, 0)
	if err != nil {
		return nil, err
	}
	firstGatewayIP, err := subnetGatewayIP(firstSubnetCIDR)
	if err != nil {
		return nil, err
	}
	component.GatewayIP = pulumi.String(firstGatewayIP).ToStringOutput()
	component.NetworkName = network.Name
	component.PrivateSubnetIDs = subnetIDs.ToStringArrayOutput()
	component.PrivateSubnetNames = subnetNames.ToStringArrayOutput()

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"networkId":        component.NetworkID,
		"gatewayId":        component.GatewayID,
		"gatewayIp":        component.GatewayIP,
		"privateSubnetIds": component.PrivateSubnetIDs,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func subnetGatewayIP(cidr string) (string, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil || !prefix.Addr().Is4() {
		return "", fmt.Errorf("subnet CIDR must be IPv4: %w", err)
	}
	gateway := prefix.Addr().Next()
	if !gateway.IsValid() || !prefix.Contains(gateway) {
		return "", fmt.Errorf("subnet CIDR %s has no usable gateway address", cidr)
	}
	return gateway.String(), nil
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
