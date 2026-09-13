package awsprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// AdmissionSelection is the provider-neutral description of the AWS catalog
// entries a planned architecture will use. It keeps stack and EKS planners
// independent of AWS SDK request/response types while sharing one validation
// implementation.
type AdmissionSelection struct {
	AccountID         string
	AvailabilityZones []string
	InstanceTypes     []EC2InstanceSelection
	AMIs              []string
	Database          *DatabaseSelection
	Valkey            *ValkeySelection
	Search            *SearchSelection
	MQ                *MQSelection
	CapacityProviders []string
	EKSVersion        string
	FckNatAMI         *FckNatAMISelection
}

type FckNatAMISelection struct {
	OwnerID      string
	NamePattern  string
	Architecture string
}

type EC2InstanceSelection struct {
	Name string
	// RequiredAvailabilityZones are the exact zones in which this consumer
	// will place instances. An offering proves catalog availability only; it
	// does not prove current capacity or account quota.
	RequiredAvailabilityZones []string
}

type DatabaseSelection struct {
	Engine            string
	Version           string
	InstanceClass     string
	AvailabilityZones []string
	ServerlessV2      bool
	MinimumACU        float64
	MaximumACU        float64
}

type ValkeySelection struct {
	Version  string
	NodeType string
}

type SearchSelection struct {
	Version       string
	InstanceTypes []string
}

type MQSelection struct {
	Engine       string
	Version      string
	InstanceType string
}

// ValidateSelection performs all AWS checks that must complete before a
// Pulumi/provider mutation. Empty or incomplete provider responses fail
// closed because an unavailable catalog is not evidence that a paid shape is
// usable.
func ValidateSelection(ctx context.Context, client CapabilityAPI, selection AdmissionSelection) error {
	if ctx == nil {
		return errors.New("AWS plan admission context is required")
	}
	if client == nil {
		return errors.New("AWS capability client is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	accountID := strings.TrimSpace(selection.AccountID)
	if accountID == "" {
		return errors.New("AWS plan admission account ID is required")
	}
	callerAccount, err := client.CallerAccount(ctx)
	if err != nil {
		return fmt.Errorf("read AWS account identity: %w", err)
	}
	if strings.TrimSpace(callerAccount) == "" || strings.TrimSpace(callerAccount) != accountID {
		return fmt.Errorf("AWS credentials resolve to account %q, but the plan targets account %q", strings.TrimSpace(callerAccount), accountID)
	}

	availableZones, err := client.AvailabilityZones(ctx)
	if err != nil {
		return fmt.Errorf("read AWS availability zones: %w", err)
	}
	if err := validateAvailabilityZones(selection.AvailabilityZones, availableZones); err != nil {
		return err
	}

	for _, instanceSelection := range selection.InstanceTypes {
		instanceType := strings.TrimSpace(instanceSelection.Name)
		if instanceType == "" {
			return errors.New("AWS plan contains an empty EC2 instance type")
		}
		requiredZones := instanceSelection.RequiredAvailabilityZones
		if len(requiredZones) == 0 {
			return fmt.Errorf("AWS EC2 instance type %q requires explicit availability zones", instanceType)
		}
		if err := requireSelectedAvailabilityZones(selection.AvailabilityZones, requiredZones); err != nil {
			return fmt.Errorf("instance type %q: %w", instanceType, err)
		}
		available, err := client.InstanceType(ctx, instanceType)
		if err != nil {
			return fmt.Errorf("check AWS EC2 instance type %q: %w", instanceType, err)
		}
		if !available {
			return fmt.Errorf("AWS EC2 instance type %q is not available", instanceType)
		}
		offerings, err := client.InstanceTypeOfferings(ctx, instanceType, requiredZones)
		if err != nil {
			return fmt.Errorf("check AWS EC2 instance type %q availability zones: %w", instanceType, err)
		}
		if err := requireNames("AWS EC2 instance type offering", requiredZones, offerings); err != nil {
			return fmt.Errorf("instance type %q: %w", instanceType, err)
		}
	}

	for _, selectedAMI := range selection.AMIs {
		ami := strings.TrimSpace(selectedAMI)
		if ami == "" {
			return errors.New("AWS plan contains an empty AMI")
		}
		available, err := client.Image(ctx, ami)
		if err != nil {
			return fmt.Errorf("check AWS AMI %q: %w", ami, err)
		}
		if !available {
			return fmt.Errorf("AWS AMI %q is not available", ami)
		}
	}
	if selection.FckNatAMI != nil {
		ami := selection.FckNatAMI
		if strings.TrimSpace(ami.OwnerID) == "" || strings.TrimSpace(ami.NamePattern) == "" || strings.TrimSpace(ami.Architecture) == "" {
			return errors.New("AWS fck-nat AMI admission requires owner, name pattern, and architecture")
		}
		available, err := client.FckNatAMI(ctx, ami.OwnerID, ami.NamePattern, ami.Architecture)
		if err != nil {
			return fmt.Errorf("check AWS fck-nat AMI: %w", err)
		}
		if !available {
			return fmt.Errorf("AWS fck-nat AMI %q owned by %q is not currently available for architecture %q", ami.NamePattern, ami.OwnerID, ami.Architecture)
		}
	}

	if selection.Database != nil {
		if err := validateDatabase(ctx, client, *selection.Database); err != nil {
			return err
		}
	}
	if selection.Valkey != nil {
		if err := validateValkey(ctx, client, *selection.Valkey); err != nil {
			return err
		}
	}
	if selection.Search != nil {
		if err := validateSearch(ctx, client, *selection.Search); err != nil {
			return err
		}
	}
	if selection.MQ != nil {
		if err := validateMQ(ctx, client, *selection.MQ); err != nil {
			return err
		}
	}
	for _, name := range selection.CapacityProviders {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("AWS capacity provider name is required")
		}
		available, err := client.ECSCapacityProvider(ctx, name)
		if err != nil {
			return fmt.Errorf("check AWS ECS capacity provider %q: %w", name, err)
		}
		if !available {
			return fmt.Errorf("AWS ECS capacity provider %q is not active", name)
		}
	}
	if version := strings.TrimSpace(selection.EKSVersion); version != "" {
		candidate, err := client.EKSVersion(ctx, version)
		if err != nil {
			return fmt.Errorf("check AWS EKS version %q: %w", version, err)
		}
		if strings.TrimSpace(candidate.Version) == "" || !strings.EqualFold(candidate.Version, version) {
			return fmt.Errorf("AWS EKS version %q is not available", version)
		}
		status := strings.ToUpper(strings.TrimSpace(candidate.Status))
		if status == "" || status == "UNSUPPORTED" {
			return fmt.Errorf("AWS EKS version %q is not supported (status %q)", version, candidate.Status)
		}
	}
	return nil
}

func validateAvailabilityZones(selected []string, available []AvailabilityZone) error {
	if len(selected) == 0 {
		return errors.New("AWS plan admission requires at least one availability zone")
	}
	availableByName := make(map[string]bool, len(available))
	for _, zone := range available {
		name := strings.TrimSpace(zone.Name)
		if name != "" && strings.EqualFold(strings.TrimSpace(zone.State), "available") {
			availableByName[name] = true
		}
	}
	if len(availableByName) == 0 {
		return errors.New("AWS returned no available availability zones")
	}
	seen := make(map[string]struct{}, len(selected))
	for _, zone := range selected {
		zone = strings.TrimSpace(zone)
		if zone == "" {
			return errors.New("AWS plan contains a blank availability zone")
		}
		if _, ok := seen[zone]; ok {
			return fmt.Errorf("AWS plan repeats availability zone %q", zone)
		}
		seen[zone] = struct{}{}
		if !availableByName[zone] {
			return fmt.Errorf("AWS availability zone %q is not currently available", zone)
		}
	}
	return nil
}

func validateDatabase(ctx context.Context, client CapabilityAPI, selection DatabaseSelection) error {
	engineInput := strings.TrimSpace(selection.Engine)
	version := strings.TrimSpace(selection.Version)
	class := strings.TrimSpace(selection.InstanceClass)
	if engineInput == "" || version == "" || class == "" {
		return errors.New("AWS database admission requires engine, version, and instance class")
	}
	engine, err := canonicalDatabaseEngine(engineInput)
	if err != nil {
		return err
	}
	candidate, err := client.DatabaseEngineVersion(ctx, engine, version)
	if err != nil {
		return fmt.Errorf("check AWS database engine %q version %q: %w", engineInput, version, err)
	}
	if strings.TrimSpace(candidate.EngineVersion) == "" || !strings.EqualFold(candidate.EngineVersion, version) || !strings.EqualFold(strings.TrimSpace(candidate.Status), "available") {
		return fmt.Errorf("AWS database engine %q version %q is not available", engineInput, version)
	}
	if selection.ServerlessV2 {
		if class != "db.serverless" {
			return fmt.Errorf("AWS Aurora Serverless v2 requires instance class db.serverless, got %q", class)
		}
		if candidate.ServerlessV2MinCapacity == nil || candidate.ServerlessV2MaxCapacity == nil {
			return fmt.Errorf("AWS database engine %q version %q does not advertise Serverless v2 capacity limits", engineInput, version)
		}
		if selection.MinimumACU < *candidate.ServerlessV2MinCapacity || selection.MaximumACU > *candidate.ServerlessV2MaxCapacity || selection.MinimumACU > selection.MaximumACU {
			return fmt.Errorf("AWS Aurora Serverless v2 capacity %.1f-%.1f is outside provider limits %.1f-%.1f", selection.MinimumACU, selection.MaximumACU, *candidate.ServerlessV2MinCapacity, *candidate.ServerlessV2MaxCapacity)
		}
	}
	orderable, err := client.DatabaseInstanceClass(ctx, engine, version, class)
	if err != nil {
		return fmt.Errorf("check AWS database instance class %q: %w", class, err)
	}
	if !orderable.Available {
		return fmt.Errorf("AWS database instance class %q is not orderable for %q %q", class, engine, version)
	}
	if err := requireNames("AWS database instance class availability zone", selection.AvailabilityZones, orderable.AvailabilityZone); err != nil {
		return fmt.Errorf("database instance class %q: %w", class, err)
	}
	return nil
}

// canonicalDatabaseEngine translates MageLift's catalog names into the
// identifiers accepted by the AWS RDS API. The public catalog keeps the
// service shape in the name (for example, rds-mysql), while RDS expects the
// engine value mysql.
func canonicalDatabaseEngine(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "aurora-mysql":
		return "aurora-mysql", nil
	case "mysql", "rds-mysql":
		return "mysql", nil
	case "mariadb", "rds-mariadb":
		return "mariadb", nil
	default:
		return "", fmt.Errorf("unsupported AWS database engine %q", value)
	}
}

func validateValkey(ctx context.Context, client CapabilityAPI, selection ValkeySelection) error {
	version := strings.TrimSpace(selection.Version)
	nodeType := strings.TrimSpace(selection.NodeType)
	if version == "" || nodeType == "" {
		return errors.New("AWS Valkey admission requires engine version and node type")
	}
	available, err := client.ValkeyEngineVersion(ctx, version)
	if err != nil {
		return fmt.Errorf("check AWS Valkey version %q: %w", version, err)
	}
	if !available {
		return fmt.Errorf("AWS Valkey version %q is not available", version)
	}
	available, err = client.ValkeyNodeType(ctx, nodeType)
	if err != nil {
		return fmt.Errorf("check AWS Valkey node type %q: %w", nodeType, err)
	}
	if !available {
		return fmt.Errorf("AWS Valkey node type %q is not available", nodeType)
	}
	return nil
}

func validateSearch(ctx context.Context, client CapabilityAPI, selection SearchSelection) error {
	version := strings.TrimSpace(selection.Version)
	if version == "" {
		return errors.New("AWS OpenSearch admission requires an engine version")
	}
	available, err := client.OpenSearchVersion(ctx, version)
	if err != nil {
		return fmt.Errorf("check AWS OpenSearch version %q: %w", version, err)
	}
	if !available {
		return fmt.Errorf("AWS OpenSearch version %q is not available", version)
	}
	for _, instanceType := range selection.InstanceTypes {
		instanceType = strings.TrimSpace(instanceType)
		if instanceType == "" {
			return errors.New("AWS OpenSearch instance type is required")
		}
		available, err := client.OpenSearchInstanceType(ctx, version, instanceType)
		if err != nil {
			return fmt.Errorf("check AWS OpenSearch instance type %q: %w", instanceType, err)
		}
		if !available {
			return fmt.Errorf("AWS OpenSearch instance type %q is not available for %q", instanceType, version)
		}
	}
	return nil
}

func validateMQ(ctx context.Context, client CapabilityAPI, selection MQSelection) error {
	engineInput := strings.TrimSpace(selection.Engine)
	version := strings.TrimSpace(selection.Version)
	instanceType := strings.TrimSpace(selection.InstanceType)
	if engineInput == "" || version == "" || instanceType == "" {
		return errors.New("AWS MQ admission requires engine, version, and instance type")
	}
	engine, err := canonicalMQEngine(engineInput)
	if err != nil {
		return err
	}
	available, err := client.MQEngineVersion(ctx, engine, version)
	if err != nil {
		return fmt.Errorf("check AWS MQ engine %q version %q: %w", engine, version, err)
	}
	if !available {
		return fmt.Errorf("AWS MQ engine %q version %q is not available", engine, version)
	}
	available, err = client.MQInstanceType(ctx, engine, version, instanceType)
	if err != nil {
		return fmt.Errorf("check AWS MQ instance type %q: %w", instanceType, err)
	}
	if !available {
		return fmt.Errorf("AWS MQ instance type %q is not available for %q %q", instanceType, engine, version)
	}
	return nil
}

// canonicalMQEngine keeps user-facing engine spellings at the provider
// boundary while ensuring every AWS SDK request uses the exact API enum.
func canonicalMQEngine(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "ACTIVEMQ", "ACTIVE-MQ", "ACTIVE MQ":
		return "ACTIVEMQ", nil
	case "RABBITMQ", "RABBIT-MQ", "RABBIT MQ":
		return "RABBITMQ", nil
	default:
		return "", fmt.Errorf("AWS MQ engine %q is not a supported Amazon MQ API value", strings.TrimSpace(value))
	}
}

func requireSelectedAvailabilityZones(selected, required []string) error {
	selectedSet := make(map[string]struct{}, len(selected))
	for _, zone := range selected {
		selectedSet[strings.TrimSpace(zone)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(required))
	for _, zone := range required {
		zone = strings.TrimSpace(zone)
		if zone == "" {
			return errors.New("required availability zones cannot be blank")
		}
		if _, ok := seen[zone]; ok {
			return fmt.Errorf("required availability zone %q is duplicated", zone)
		}
		seen[zone] = struct{}{}
		if _, ok := selectedSet[zone]; !ok {
			return fmt.Errorf("required availability zone %q is not selected by the plan", zone)
		}
	}
	return nil
}

func requireNames(kind string, required, available []string) error {
	availableSet := make(map[string]struct{}, len(available))
	for _, name := range available {
		name = strings.TrimSpace(name)
		if name != "" {
			availableSet[name] = struct{}{}
		}
	}
	if len(availableSet) == 0 {
		return fmt.Errorf("provider returned no %s entries", kind)
	}
	for _, name := range required {
		if _, ok := availableSet[strings.TrimSpace(name)]; !ok {
			return fmt.Errorf("%s %q is unavailable", kind, strings.TrimSpace(name))
		}
	}
	return nil
}
