package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cloud/aws/network"
	awsprovider "github.com/magelift/magelift/internal/cloud/aws/provider"
	"github.com/magelift/magelift/internal/cloud/aws/queue"
	"github.com/magelift/magelift/internal/cloud/aws/runtime"
	"github.com/magelift/magelift/internal/platform"
)

// RegionAdmission performs the AWS account, region, and selected catalog
// checks that cannot be proven from YAML. Its client port exposes only
// read-only AWS operations, so provider mutation cannot happen through this
// boundary.
type RegionAdmission struct {
	NewClient func(context.Context, string) (awsprovider.CapabilityAPI, error)
}

var _ platform.PlanAdmission = RegionAdmission{}

func (a RegionAdmission) Admit(ctx context.Context, planned platform.PlannedStack) (platform.PlannedStack, error) {
	if ctx == nil {
		return nil, errors.New("AWS plan admission context is required")
	}
	value, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("AWS plan admission received unexpected planned type %T", planned)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clientFactory := a.NewClient
	if clientFactory == nil {
		clientFactory = func(ctx context.Context, region string) (awsprovider.CapabilityAPI, error) {
			return awsprovider.NewCapabilityClient(ctx, region)
		}
	}
	client, err := clientFactory(ctx, value.Spec.Identity.Region)
	if err != nil {
		return nil, fmt.Errorf("create AWS read-only capability client: %w", err)
	}
	if err := awsprovider.ValidateSelection(ctx, client, selectionFromSpec(value.Spec)); err != nil {
		return nil, err
	}
	return value, nil
}

func selectionFromSpec(spec Spec) awsprovider.AdmissionSelection {
	selection := awsprovider.AdmissionSelection{
		AccountID:         spec.Identity.AccountID,
		AvailabilityZones: append([]string(nil), spec.Policy.AvailabilityZones...),
		Valkey: &awsprovider.ValkeySelection{
			Version:  spec.Catalog.Versions.Valkey,
			NodeType: spec.Catalog.Valkey.NodeType,
		},
	}

	if spec.Existing.Network == nil && spec.Policy.NatMode == NatModeFckNat {
		instanceType := strings.TrimSpace(spec.Policy.NatInstanceType)
		if instanceType == "" {
			instanceType = network.FckNatInstanceType
		}
		requiredZones := append([]string(nil), spec.Policy.AvailabilityZones...)
		if network.ResolveNatTopology(spec.Policy.NatTopology, spec.Identity.Preset) == network.NatTopologySingleAZ && len(requiredZones) > 0 {
			requiredZones = requiredZones[:1]
		}
		selection.InstanceTypes = append(selection.InstanceTypes, awsprovider.EC2InstanceSelection{
			Name:                      instanceType,
			RequiredAvailabilityZones: requiredZones,
		})
		selection.FckNatAMI = &awsprovider.FckNatAMISelection{OwnerID: network.FckNatAMIOwner, NamePattern: network.FckNatAMINamePrefix, Architecture: "arm64"}
	}

	computeMode := strings.TrimSpace(spec.Catalog.Fargate.ComputeMode)
	if computeMode == "" {
		computeMode = runtime.ComputeModeFargate
	}
	switch computeMode {
	case runtime.ComputeModeEC2AutoScaling:
		instanceType := strings.TrimSpace(spec.Catalog.Fargate.InstanceType)
		if instanceType == "" {
			instanceType = "t3.medium"
		}
		selection.InstanceTypes = append(selection.InstanceTypes, awsprovider.EC2InstanceSelection{Name: instanceType, RequiredAvailabilityZones: append([]string(nil), spec.Policy.AvailabilityZones...)})
		selection.AMIs = append(selection.AMIs, spec.Catalog.Fargate.InstanceAMI)
	case runtime.ComputeModeManagedInstance:
		instanceType := strings.TrimSpace(spec.Catalog.Fargate.InstanceType)
		if instanceType == "" {
			instanceType = "m6i.large"
		}
		selection.InstanceTypes = append(selection.InstanceTypes, awsprovider.EC2InstanceSelection{Name: instanceType, RequiredAvailabilityZones: append([]string(nil), spec.Policy.AvailabilityZones...)})
	case runtime.ComputeModeFargateSpot:
		selection.CapacityProviders = append(selection.CapacityProviders, "FARGATE_SPOT")
	}

	if spec.Existing.Database == nil {
		version := spec.Catalog.Versions.AuroraMySQL
		switch spec.Catalog.DatabaseEngine {
		case DatabaseEngineRDSMySQL:
			version = spec.Catalog.Versions.MySQL
		case DatabaseEngineRDSMariaDB:
			version = spec.Catalog.Versions.MariaDB
		}
		instanceClass := spec.Catalog.AuroraProvisioned.InstanceClass
		serverless := spec.Catalog.DatabaseEngine == DatabaseEngineAuroraMySQL && spec.Identity.Preset == "preview"
		if serverless {
			instanceClass = "db.serverless"
		}
		selection.Database = &awsprovider.DatabaseSelection{
			Engine:            spec.Catalog.DatabaseEngine,
			Version:           version,
			InstanceClass:     instanceClass,
			AvailabilityZones: append([]string(nil), spec.Policy.AvailabilityZones...),
			ServerlessV2:      serverless,
			MinimumACU:        spec.Catalog.Aurora.MinimumACU,
			MaximumACU:        spec.Catalog.Aurora.MaximumACU,
		}
	}

	if spec.Catalog.SearchMode != SearchModeDisabled {
		selection.Search = &awsprovider.SearchSelection{Version: spec.Catalog.Versions.OpenSearch}
		if spec.Catalog.SearchMode == SearchModeProvisioned {
			selection.Search.InstanceTypes = append(selection.Search.InstanceTypes, spec.Catalog.SearchProvisioned.InstanceType)
			if master := strings.TrimSpace(spec.Catalog.SearchProvisioned.DedicatedMasterType); master != "" {
				selection.Search.InstanceTypes = append(selection.Search.InstanceTypes, master)
			}
		}
	}

	if effectiveQueueMode(spec) == queue.ModeAmazonMQ {
		selection.MQ = &awsprovider.MQSelection{
			Engine:       "RABBITMQ",
			Version:      spec.Catalog.Versions.RabbitMQ,
			InstanceType: spec.Catalog.RabbitMQ.InstanceType,
		}
	}
	return selection
}
