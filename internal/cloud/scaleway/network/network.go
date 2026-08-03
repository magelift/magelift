package network

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-scaleway/sdk/go/scaleway/network"
)

const TypeToken = "magelift:scaleway:Network"

type Args struct {
	Preset      sdk.PresetID
	ProjectID   string
	Region      string
	NetworkCIDR string
	Zones       []string
	Labels      map[string]string
}

type Component struct {
	pulumi.ResourceState
	NetworkID          pulumi.StringOutput
	PrivateSubnetIDs   pulumi.StringArrayOutput
	PrivateNetworkID   pulumi.StringOutput
	PrivateSubnetNames pulumi.StringArrayOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("network name is required")
	}
	if strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("Scaleway project ID and region are required")
	}
	prefix, err := netip.ParsePrefix(args.NetworkCIDR)
	if err != nil || !prefix.Addr().Is4() {
		return nil, fmt.Errorf("network CIDR must be IPv4: %w", err)
	}
	if len(args.Zones) == 0 {
		return nil, errors.New("at least one zone is required")
	}

	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"projectId": pulumi.String(args.ProjectID),
		"region":    pulumi.String(args.Region),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	tags := make(pulumi.StringArray, 0, len(args.Labels))
	for key, value := range args.Labels {
		tags = append(tags, pulumi.String(key+"="+value))
	}

	vpc, err := network.NewVpc(ctx, name+"-vpc", &network.VpcArgs{
		Name:          pulumi.String(name),
		ProjectId:     pulumi.String(args.ProjectID),
		Region:        pulumi.String(args.Region),
		EnableRouting: pulumi.Bool(true),
		Tags:          tags,
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create Scaleway VPC: %w", err)
	}

	pn, err := network.NewPrivateNetwork(ctx, name+"-pn", &network.PrivateNetworkArgs{
		Name:      pulumi.String(name),
		ProjectId: pulumi.String(args.ProjectID),
		Region:    pulumi.String(args.Region),
		VpcId:     vpc.ID().ToStringOutput().ToStringPtrOutput(),
		Ipv4Subnet: &network.PrivateNetworkIpv4SubnetArgs{
			Subnet: pulumi.String(args.NetworkCIDR),
		},
		Tags: tags,
	}, parent, pulumi.DependsOn([]pulumi.Resource{vpc}))
	if err != nil {
		return nil, fmt.Errorf("create Scaleway private network: %w", err)
	}

	// Magento contract expects subnet IDs; Scaleway PN is the attachment unit.
	component.NetworkID = vpc.ID().ToStringOutput()
	component.PrivateNetworkID = pn.ID().ToStringOutput()
	component.PrivateSubnetIDs = pulumi.StringArray{pn.ID().ToStringOutput()}.ToStringArrayOutput()
	component.PrivateSubnetNames = pulumi.StringArray{pn.Name}.ToStringArrayOutput()

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"networkId":        component.NetworkID,
		"privateSubnetIds": component.PrivateSubnetIDs,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
