package eksops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cloud/aws/network"
	awsprovider "github.com/magelift/magelift/internal/cloud/aws/provider"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

// RegionAdmission performs the AWS account, EKS, EC2, database, and cache
// checks required by an EKS plan before cluster or data-plane mutation.
type RegionAdmission struct {
	NewClient func(context.Context, string) (awsprovider.CapabilityAPI, error)
}

var _ platform.PlanAdmission = RegionAdmission{}

func (a RegionAdmission) Admit(ctx context.Context, planned platform.PlannedStack) (platform.PlannedStack, error) {
	if ctx == nil {
		return nil, errors.New("AWS EKS plan admission context is required")
	}
	value, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("AWS EKS plan admission received unexpected planned type %T", planned)
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
		return nil, fmt.Errorf("create AWS EKS read-only capability client: %w", err)
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
		EKSVersion:        spec.Catalog.KubernetesVersion,
		Valkey: &awsprovider.ValkeySelection{
			Version:  spec.Catalog.ValkeyVersion,
			NodeType: spec.Catalog.ValkeyNodeType,
		},
	}

	if spec.Policy.NatMode == NatModeFckNat {
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

	computeMode := strings.TrimSpace(spec.Catalog.ComputeMode)
	if computeMode == "" {
		computeMode = ComputeModeAuto
	}
	switch computeMode {
	case ComputeModeManagedNodes, ComputeModeSelfManaged:
		instanceType := strings.TrimSpace(spec.Catalog.NodeInstanceType)
		if instanceType == "" {
			instanceType = "m6i.large"
		}
		selection.InstanceTypes = append(selection.InstanceTypes, awsprovider.EC2InstanceSelection{Name: instanceType, RequiredAvailabilityZones: append([]string(nil), spec.Policy.AvailabilityZones...)})
		if computeMode == ComputeModeSelfManaged {
			selection.AMIs = append(selection.AMIs, spec.Catalog.NodeAMI)
		}
	}

	version := spec.Catalog.AuroraMySQLVersion
	switch spec.Catalog.DatabaseEngine {
	case DatabaseEngineRDSMySQL:
		version = spec.Catalog.MySQLVersion
	case DatabaseEngineRDSMariaDB:
		version = spec.Catalog.MariaDBVersion
	}
	instanceClass := spec.Catalog.InstanceClass
	serverless := spec.Catalog.DatabaseEngine == DatabaseEngineAuroraMySQL && spec.Identity.Preset == sdk.PresetPreview
	if serverless {
		instanceClass = "db.serverless"
	}
	selection.Database = &awsprovider.DatabaseSelection{
		Engine:            spec.Catalog.DatabaseEngine,
		Version:           version,
		InstanceClass:     instanceClass,
		AvailabilityZones: append([]string(nil), spec.Policy.AvailabilityZones...),
		ServerlessV2:      serverless,
		MinimumACU:        spec.Catalog.AuroraMinACU,
		MaximumACU:        spec.Catalog.AuroraMaxACU,
	}
	return selection
}
