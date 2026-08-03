package network

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:Network"

type Args struct {
	Preset      sdk.PresetID
	Project     string
	Region      string
	NetworkCIDR string
	Zones       []string
	Labels      map[string]string
}

type Component struct {
	pulumi.ResourceState
	NetworkID          pulumi.IDOutput
	NetworkName        pulumi.StringOutput
	NetworkSelfLink    pulumi.StringOutput
	PrivateSubnetIDs   pulumi.StringArrayOutput
	PublicSubnetIDs    pulumi.StringArrayOutput
	PrivateSubnetNames pulumi.StringArrayOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("network name is required")
	}
	if strings.TrimSpace(args.Project) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("GCP project and region are required")
	}
	prefix, err := netip.ParsePrefix(args.NetworkCIDR)
	if err != nil || !prefix.Addr().Is4() {
		return nil, fmt.Errorf("network CIDR must be a valid IPv4 prefix: %w", err)
	}
	if len(args.Zones) == 0 {
		return nil, errors.New("at least one zone is required")
	}
	if prefix.Bits() > 20 {
		return nil, errors.New("network CIDR must be at most /20 so subnets can be carved")
	}
	// Two /24s per zone index; reject before any Pulumi registration so an
	// over-long zone list fails with a capacity message the operator can act on.
	maxZones := subnetIndexCapacity(prefix)
	if len(args.Zones) > maxZones {
		return nil, fmt.Errorf("network CIDR %s has room for %d zone(s) but %d were requested", prefix, maxZones, len(args.Zones))
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project), "region": pulumi.String(args.Region),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	network, err := compute.NewNetwork(ctx, name, &compute.NetworkArgs{
		Project:               pulumi.String(args.Project),
		Name:                  pulumi.String(name),
		AutoCreateSubnetworks: pulumi.Bool(false),
		RoutingMode:           pulumi.String("REGIONAL"),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create VPC network: %w", err)
	}

	privateIDs := make(pulumi.StringArray, 0, len(args.Zones))
	publicIDs := make(pulumi.StringArray, 0, len(args.Zones))
	privateNames := make(pulumi.StringArray, 0, len(args.Zones))
	for index, zone := range args.Zones {
		privateCIDR, publicCIDR, err := subnetCIDRs(prefix, index)
		if err != nil {
			return nil, err
		}
		privateName := fmt.Sprintf("%s-private-%d", name, index)
		publicName := fmt.Sprintf("%s-public-%d", name, index)
		private, err := compute.NewSubnetwork(ctx, privateName, &compute.SubnetworkArgs{
			Project:               pulumi.String(args.Project),
			Name:                  pulumi.String(privateName),
			IpCidrRange:           pulumi.String(privateCIDR),
			Region:                pulumi.String(args.Region),
			Network:               network.ID(),
			PrivateIpGoogleAccess: pulumi.Bool(true),
		}, parent, pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return nil, fmt.Errorf("create private subnet for %s: %w", zone, err)
		}
		public, err := compute.NewSubnetwork(ctx, publicName, &compute.SubnetworkArgs{
			Project:     pulumi.String(args.Project),
			Name:        pulumi.String(publicName),
			IpCidrRange: pulumi.String(publicCIDR),
			Region:      pulumi.String(args.Region),
			Network:     network.ID(),
		}, parent, pulumi.DependsOn([]pulumi.Resource{network}))
		if err != nil {
			return nil, fmt.Errorf("create public subnet for %s: %w", zone, err)
		}
		privateIDs = append(privateIDs, private.ID().ToStringOutput())
		publicIDs = append(publicIDs, public.ID().ToStringOutput())
		privateNames = append(privateNames, private.Name)
	}

	// Autopilot node image pulls and Secret Manager access need egress.
	router, err := compute.NewRouter(ctx, name+"-router", &compute.RouterArgs{
		Project: pulumi.String(args.Project),
		Name:    pulumi.String(name + "-router"),
		Region:  pulumi.String(args.Region),
		Network: network.ID(),
	}, parent, pulumi.DependsOn([]pulumi.Resource{network}))
	if err != nil {
		return nil, fmt.Errorf("create Cloud Router: %w", err)
	}
	_, err = compute.NewRouterNat(ctx, name+"-nat", &compute.RouterNatArgs{
		Project:                       pulumi.String(args.Project),
		Name:                          pulumi.String(name + "-nat"),
		Router:                        router.Name,
		Region:                        pulumi.String(args.Region),
		NatIpAllocateOption:           pulumi.String("AUTO_ONLY"),
		SourceSubnetworkIpRangesToNat: pulumi.String("ALL_SUBNETWORKS_ALL_IP_RANGES"),
	}, parent, pulumi.DependsOn([]pulumi.Resource{router}))
	if err != nil {
		return nil, fmt.Errorf("create Cloud NAT: %w", err)
	}

	component.NetworkID = network.ID()
	component.NetworkName = network.Name
	component.NetworkSelfLink = network.SelfLink
	component.PrivateSubnetIDs = privateIDs.ToStringArrayOutput()
	component.PublicSubnetIDs = publicIDs.ToStringArrayOutput()
	component.PrivateSubnetNames = privateNames.ToStringArrayOutput()
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"networkId": network.ID(), "networkName": network.Name, "privateSubnetIds": component.PrivateSubnetIDs,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

// subnetIndexCapacity is how many zone indices fit when carving two /24s each
// from prefix (even private, odd public).
func subnetIndexCapacity(prefix netip.Prefix) int {
	return (1 << (24 - prefix.Bits())) / 2
}

func subnetCIDRs(prefix netip.Prefix, index int) (privateCIDR, publicCIDR string, err error) {
	if prefix.Bits() > 20 {
		return "", "", errors.New("network CIDR must be at most /20 so subnets can be carved")
	}
	capacity := subnetIndexCapacity(prefix)
	if index < 0 || index >= capacity {
		return "", "", fmt.Errorf("subnet index %d out of range: network CIDR %s holds %d index(es)", index, prefix, capacity)
	}
	base := prefix.Masked().Addr().As4()
	addr := binary.BigEndian.Uint32(base[:])
	// Carve /24s: even index private, odd public, offset by index*2 from the parent.
	private := addr + uint32(index*2)<<8
	public := private + 1<<8
	var privateIP, publicIP [4]byte
	binary.BigEndian.PutUint32(privateIP[:], private)
	binary.BigEndian.PutUint32(publicIP[:], public)
	return netip.PrefixFrom(netip.AddrFrom4(privateIP), 24).String(), netip.PrefixFrom(netip.AddrFrom4(publicIP), 24).String(), nil
}
