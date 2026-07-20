// Package cache provisions the Magento cache capability on Scaleway. Scaleway
// does not offer a managed Valkey product yet, so Redis is the only supported
// engine: Spec.Catalog.CacheMode must be the explicit escape-hatch value
// "redis" (internal/config ScalewayTarget.CacheMode enum) until Scaleway
// ships Valkey.
package cache

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-scaleway/sdk/go/scaleway/redis"
)

const TypeToken = "magelift:scaleway:Cache"

type Args struct {
	ProjectID        string
	Zone             string
	PrivateNetworkID pulumi.StringInput
	NodeType         string
	Labels           map[string]string
}

type Component struct {
	pulumi.ResourceState
	PrimaryEndpoint pulumi.StringOutput
	ClusterName     pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("cache name is required")
	}
	if strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Zone) == "" {
		return nil, errors.New("Scaleway project ID and zone are required")
	}
	if strings.TrimSpace(args.NodeType) == "" {
		args.NodeType = "RED1-MICRO"
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"projectId": pulumi.String(args.ProjectID), "zone": pulumi.String(args.Zone),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	tags := make(pulumi.StringArray, 0, len(args.Labels))
	for key, value := range args.Labels {
		tags = append(tags, pulumi.String(key+"="+value))
	}

	password, err := random.NewRandomPassword(ctx, name+"-password", &random.RandomPasswordArgs{
		Length:          pulumi.Int(32),
		Special:         pulumi.Bool(false),
		OverrideSpecial: pulumi.String(""),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate cache password: %w", err)
	}

	cluster, err := redis.NewCluster(ctx, name, &redis.ClusterArgs{
		Name:        pulumi.String(name),
		Version:     pulumi.String("7.0.5"),
		NodeType:    pulumi.String(args.NodeType),
		ClusterSize: pulumi.Int(1),
		UserName:    pulumi.String("magelift"),
		Password:    password.Result,
		ProjectId:   pulumi.String(args.ProjectID),
		Zone:        pulumi.String(args.Zone),
		Tags:        tags,
		PrivateNetworks: redis.ClusterPrivateNetworkArray{
			&redis.ClusterPrivateNetworkArgs{Id: args.PrivateNetworkID},
		},
	}, parent, pulumi.DependsOn([]pulumi.Resource{password}))
	if err != nil {
		return nil, fmt.Errorf("create Scaleway Redis cluster: %w", err)
	}

	component.PrimaryEndpoint = cluster.ConnectionString.ApplyT(func(connection string) string {
		parsed, err := url.Parse(connection)
		if err != nil {
			return ""
		}
		return parsed.Hostname()
	}).(pulumi.StringOutput)
	component.ClusterName = cluster.Name
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"primaryEndpoint": component.PrimaryEndpoint,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
