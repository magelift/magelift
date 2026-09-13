package awsprovider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/mq"
)

type fakeCapabilityAPI struct {
	account             string
	zones               []AvailabilityZone
	instanceTypes       map[string]bool
	instanceOfferings   map[string][]string
	images              map[string]bool
	databaseVersions    map[string]DatabaseEngineVersion
	databaseClasses     map[string]DatabaseInstanceClass
	valkeyVersions      map[string]bool
	valkeyNodeTypes     map[string]bool
	searchVersions      map[string]bool
	searchInstanceTypes map[string]bool
	mqVersions          map[string]bool
	mqInstanceTypes     map[string]bool
	capacityProviders   map[string]bool
	eksVersions         map[string]EKSVersion
	offeringZones       map[string][]string
	fckNatAMI           bool
	calls               map[string]int
}

func (f *fakeCapabilityAPI) call(name string) { f.calls[name]++ }

func (f *fakeCapabilityAPI) CallerAccount(context.Context) (string, error) {
	f.call("CallerAccount")
	return f.account, nil
}
func (f *fakeCapabilityAPI) AvailabilityZones(context.Context) ([]AvailabilityZone, error) {
	f.call("AvailabilityZones")
	return f.zones, nil
}
func (f *fakeCapabilityAPI) InstanceType(_ context.Context, name string) (bool, error) {
	f.call("InstanceType:" + name)
	return f.instanceTypes[name], nil
}
func (f *fakeCapabilityAPI) InstanceTypeOfferings(_ context.Context, name string, zones []string) ([]string, error) {
	f.call("InstanceTypeOfferings:" + name)
	f.offeringZones[name] = append([]string(nil), zones...)
	return f.instanceOfferings[name], nil
}
func (f *fakeCapabilityAPI) Image(_ context.Context, name string) (bool, error) {
	f.call("Image:" + name)
	return f.images[name], nil
}
func (f *fakeCapabilityAPI) FckNatAMI(_ context.Context, ownerID, namePattern, architecture string) (bool, error) {
	f.call("FckNatAMI:" + ownerID + ":" + namePattern + ":" + architecture)
	return f.fckNatAMI, nil
}
func (f *fakeCapabilityAPI) DatabaseEngineVersion(_ context.Context, engine, version string) (DatabaseEngineVersion, error) {
	f.call("DatabaseEngineVersion:" + engine + ":" + version)
	return f.databaseVersions[engine+":"+version], nil
}
func (f *fakeCapabilityAPI) DatabaseInstanceClass(_ context.Context, engine, version, class string) (DatabaseInstanceClass, error) {
	f.call("DatabaseInstanceClass:" + engine + ":" + version + ":" + class)
	return f.databaseClasses[engine+":"+version+":"+class], nil
}
func (f *fakeCapabilityAPI) ValkeyEngineVersion(_ context.Context, version string) (bool, error) {
	f.call("ValkeyEngineVersion:" + version)
	return f.valkeyVersions[version], nil
}
func (f *fakeCapabilityAPI) ValkeyNodeType(_ context.Context, nodeType string) (bool, error) {
	f.call("ValkeyNodeType:" + nodeType)
	return f.valkeyNodeTypes[nodeType], nil
}
func (f *fakeCapabilityAPI) OpenSearchVersion(_ context.Context, version string) (bool, error) {
	f.call("OpenSearchVersion:" + version)
	return f.searchVersions[version], nil
}
func (f *fakeCapabilityAPI) OpenSearchInstanceType(_ context.Context, version, instanceType string) (bool, error) {
	f.call("OpenSearchInstanceType:" + version + ":" + instanceType)
	return f.searchInstanceTypes[version+":"+instanceType], nil
}
func (f *fakeCapabilityAPI) MQEngineVersion(_ context.Context, engine, version string) (bool, error) {
	f.call("MQEngineVersion:" + engine + ":" + version)
	return f.mqVersions[engine+":"+version], nil
}
func (f *fakeCapabilityAPI) MQInstanceType(_ context.Context, engine, version, instanceType string) (bool, error) {
	f.call("MQInstanceType:" + engine + ":" + version + ":" + instanceType)
	return f.mqInstanceTypes[engine+":"+version+":"+instanceType], nil
}
func (f *fakeCapabilityAPI) ECSCapacityProvider(_ context.Context, name string) (bool, error) {
	f.call("ECSCapacityProvider:" + name)
	return f.capacityProviders[name], nil
}
func (f *fakeCapabilityAPI) EKSVersion(_ context.Context, version string) (EKSVersion, error) {
	f.call("EKSVersion:" + version)
	return f.eksVersions[version], nil
}

func validFakeCapabilityAPI() *fakeCapabilityAPI {
	return &fakeCapabilityAPI{
		account:             "123456789012",
		zones:               []AvailabilityZone{{Name: "eu-west-3a", State: "available"}, {Name: "eu-west-3b", State: "available"}},
		instanceTypes:       map[string]bool{"t4g.nano": true, "m6i.large": true},
		instanceOfferings:   map[string][]string{"t4g.nano": {"eu-west-3a", "eu-west-3b"}, "m6i.large": {"eu-west-3a", "eu-west-3b"}},
		images:              map[string]bool{"ami-1234": true},
		fckNatAMI:           true,
		databaseVersions:    map[string]DatabaseEngineVersion{"aurora-mysql:8.0.mysql_aurora.3.08.2": {EngineVersion: "8.0.mysql_aurora.3.08.2", Status: "available", ServerlessV2MinCapacity: float64Pointer(0), ServerlessV2MaxCapacity: float64Pointer(128)}},
		databaseClasses:     map[string]DatabaseInstanceClass{"aurora-mysql:8.0.mysql_aurora.3.08.2:db.serverless": {Available: true, AvailabilityZone: []string{"eu-west-3a", "eu-west-3b"}}},
		valkeyVersions:      map[string]bool{"8.0": true},
		valkeyNodeTypes:     map[string]bool{"cache.t4g.micro": true},
		searchVersions:      map[string]bool{"OpenSearch_2.19": true},
		searchInstanceTypes: map[string]bool{"OpenSearch_2.19:r7g.large.search": true},
		mqVersions:          map[string]bool{"RABBITMQ:3.13": true},
		mqInstanceTypes:     map[string]bool{"RABBITMQ:3.13:mq.m7g.large": true},
		capacityProviders:   map[string]bool{"FARGATE_SPOT": true},
		eksVersions:         map[string]EKSVersion{"1.36": {Version: "1.36", Status: "STANDARD_SUPPORT"}},
		offeringZones:       make(map[string][]string),
		calls:               make(map[string]int),
	}
}

func float64Pointer(value float64) *float64 { return &value }

func validAdmissionSelection() AdmissionSelection {
	return AdmissionSelection{
		AccountID:         "123456789012",
		AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
		InstanceTypes:     []EC2InstanceSelection{{Name: "t4g.nano", RequiredAvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}}},
		AMIs:              []string{"ami-1234"},
		Database: &DatabaseSelection{
			Engine: "aurora-mysql", Version: "8.0.mysql_aurora.3.08.2", InstanceClass: "db.serverless",
			AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, ServerlessV2: true, MinimumACU: 0, MaximumACU: 4,
		},
		Valkey:            &ValkeySelection{Version: "8.0", NodeType: "cache.t4g.micro"},
		Search:            &SearchSelection{Version: "OpenSearch_2.19", InstanceTypes: []string{"r7g.large.search"}},
		MQ:                &MQSelection{Engine: "RABBITMQ", Version: "3.13", InstanceType: "mq.m7g.large"},
		CapacityProviders: []string{"FARGATE_SPOT"},
		EKSVersion:        "1.36",
	}
}

func TestValidateSelectionUsesOnlyReadOnlyCapabilityPort(t *testing.T) {
	client := validFakeCapabilityAPI()
	if err := ValidateSelection(context.Background(), client, validAdmissionSelection()); err != nil {
		t.Fatalf("ValidateSelection() error = %v", err)
	}
	for _, name := range []string{
		"CallerAccount", "AvailabilityZones", "InstanceType:t4g.nano", "InstanceTypeOfferings:t4g.nano", "Image:ami-1234",
		"DatabaseEngineVersion:aurora-mysql:8.0.mysql_aurora.3.08.2", "DatabaseInstanceClass:aurora-mysql:8.0.mysql_aurora.3.08.2:db.serverless",
		"ValkeyEngineVersion:8.0", "ValkeyNodeType:cache.t4g.micro", "OpenSearchVersion:OpenSearch_2.19",
		"OpenSearchInstanceType:OpenSearch_2.19:r7g.large.search", "MQEngineVersion:RABBITMQ:3.13", "MQInstanceType:RABBITMQ:3.13:mq.m7g.large",
		"ECSCapacityProvider:FARGATE_SPOT", "EKSVersion:1.36",
	} {
		if client.calls[name] != 1 {
			t.Errorf("capability call %q count = %d, want 1", name, client.calls[name])
		}
	}
}

func TestValidateSelectionUsesExplicitRequiredAvailabilityZones(t *testing.T) {
	client := validFakeCapabilityAPI()
	selection := validAdmissionSelection()
	selection.InstanceTypes = []EC2InstanceSelection{
		{Name: "t4g.nano", RequiredAvailabilityZones: []string{"eu-west-3a"}},
		{Name: "m6i.large", RequiredAvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}},
	}
	if err := ValidateSelection(context.Background(), client, selection); err != nil {
		t.Fatalf("ValidateSelection() error = %v", err)
	}
	if got := client.offeringZones["t4g.nano"]; len(got) != 1 || got[0] != "eu-west-3a" {
		t.Fatalf("fck-nat offering zones = %#v, want [eu-west-3a]", got)
	}
	if got := client.offeringZones["m6i.large"]; len(got) != 2 || got[0] != "eu-west-3a" || got[1] != "eu-west-3b" {
		t.Fatalf("ECS offering zones = %#v, want both selected zones", got)
	}
}

func TestValidateSelectionRequiresExplicitAvailabilityZonesForEC2(t *testing.T) {
	client := validFakeCapabilityAPI()
	selection := validAdmissionSelection()
	selection.InstanceTypes[0].RequiredAvailabilityZones = nil
	if err := ValidateSelection(context.Background(), client, selection); err == nil || !strings.Contains(err.Error(), "requires explicit availability zones") {
		t.Fatalf("ValidateSelection() error = %v, want explicit-zone failure", err)
	}
}

func TestValidateSelectionChecksDynamicFckNatAMI(t *testing.T) {
	client := validFakeCapabilityAPI()
	selection := validAdmissionSelection()
	selection.FckNatAMI = &FckNatAMISelection{OwnerID: "568608671756", NamePattern: "fck-nat-al2023-*-arm64-ebs", Architecture: "arm64"}
	if err := ValidateSelection(context.Background(), client, selection); err != nil {
		t.Fatalf("ValidateSelection() error = %v", err)
	}
	if client.calls["FckNatAMI:568608671756:fck-nat-al2023-*-arm64-ebs:arm64"] != 1 {
		t.Fatalf("FckNatAMI call count = %d, want 1", client.calls["FckNatAMI:568608671756:fck-nat-al2023-*-arm64-ebs:arm64"])
	}
	client.fckNatAMI = false
	if err := ValidateSelection(context.Background(), client, selection); err == nil || !strings.Contains(err.Error(), "fck-nat AMI") {
		t.Fatalf("ValidateSelection() error = %v, want fck-nat AMI failure", err)
	}
}

func TestCanonicalMQEngineAcceptsUserFacingSpellings(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "rabbit mixed case", input: "RabbitMQ", want: "RABBITMQ"},
		{name: "rabbit lower case", input: "rabbitmq", want: "RABBITMQ"},
		{name: "active mixed case", input: "ActiveMQ", want: "ACTIVEMQ"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := canonicalMQEngine(test.input)
			if err != nil || got != test.want {
				t.Fatalf("canonicalMQEngine(%q) = %q, %v; want %q", test.input, got, err, test.want)
			}
		})
	}
	if _, err := canonicalMQEngine("unknown"); err == nil {
		t.Fatal("canonicalMQEngine(unknown) returned nil error")
	}
}

func TestValidateSelectionCanonicalizesUserFacingRDSDatabaseEngine(t *testing.T) {
	client := validFakeCapabilityAPI()
	selection := validAdmissionSelection()
	selection.Database = &DatabaseSelection{
		Engine: "rds-mysql", Version: "8.4.10", InstanceClass: "db.t4g.micro",
		AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
	}
	client.databaseVersions["mysql:8.4.10"] = DatabaseEngineVersion{EngineVersion: "8.4.10", Status: "available"}
	client.databaseClasses["mysql:8.4.10:db.t4g.micro"] = DatabaseInstanceClass{Available: true, AvailabilityZone: []string{"eu-west-3a", "eu-west-3b"}}

	if err := ValidateSelection(context.Background(), client, selection); err != nil {
		t.Fatalf("ValidateSelection() error = %v", err)
	}
	if client.calls["DatabaseEngineVersion:mysql:8.4.10"] != 1 {
		t.Fatalf("RDS engine admission call count = %d, want 1", client.calls["DatabaseEngineVersion:mysql:8.4.10"])
	}
	if client.calls["DatabaseInstanceClass:mysql:8.4.10:db.t4g.micro"] != 1 {
		t.Fatalf("RDS instance-class admission call count = %d, want 1", client.calls["DatabaseInstanceClass:mysql:8.4.10:db.t4g.micro"])
	}
}

func TestSDKCapabilityClientEKSVersionUsesOneSelectionFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/cluster-versions" {
			t.Errorf("EKS request = %s %s, want GET /cluster-versions", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("clusterVersions"); got != "1.36" {
			t.Errorf("clusterVersions = %q, want 1.36", got)
		}
		if _, ok := query["includeAll"]; ok {
			t.Error("EKS request sent includeAll together with clusterVersions")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"clusterVersions":[{"clusterVersion":"1.36","versionStatus":"STANDARD_SUPPORT"}]}`)
	}))
	defer server.Close()

	configuration := testAWSConfig(server.Client())
	client := &SDKCapabilityClient{eks: eks.NewFromConfig(configuration, eksEndpoint(server.URL))}
	got, err := client.EKSVersion(context.Background(), "1.36")
	if err != nil {
		t.Fatalf("EKSVersion() error = %v", err)
	}
	if got.Version != "1.36" || got.Status != "STANDARD_SUPPORT" {
		t.Fatalf("EKSVersion() = %#v, want version/status 1.36/STANDARD_SUPPORT", got)
	}
}

func TestSDKCapabilityClientInstanceTypeOfferingsPaginates(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/" {
			t.Errorf("EC2 request = %s %s, want POST /", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse EC2 request: %v", err)
			return
		}
		if got := r.Form.Get("Action"); got != "DescribeInstanceTypeOfferings" {
			t.Errorf("EC2 action = %q", got)
		}
		if got := r.Form.Get("Filter.1.Value.1"); got != "t4g.nano" {
			t.Errorf("EC2 instance-type filter = %q", got)
		}
		token := r.Form.Get("NextToken")
		requests = append(requests, token)
		w.Header().Set("Content-Type", "text/xml")
		if token == "" {
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><DescribeInstanceTypeOfferingsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15"><instanceTypeOfferingSet><item><instanceType>t4g.nano</instanceType><locationType>availability-zone</locationType><location>eu-west-3a</location></item></instanceTypeOfferingSet><nextToken>page-2</nextToken></DescribeInstanceTypeOfferingsResponse>`)
			return
		}
		if token != "page-2" {
			t.Errorf("second EC2 token = %q, want page-2", token)
		}
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><DescribeInstanceTypeOfferingsResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15"><instanceTypeOfferingSet><item><instanceType>t4g.nano</instanceType><locationType>availability-zone</locationType><location>eu-west-3b</location></item></instanceTypeOfferingSet></DescribeInstanceTypeOfferingsResponse>`)
	}))
	defer server.Close()

	configuration := testAWSConfig(server.Client())
	client := &SDKCapabilityClient{ec2: ec2.NewFromConfig(configuration, ec2Endpoint(server.URL))}
	got, err := client.InstanceTypeOfferings(context.Background(), "t4g.nano", []string{"eu-west-3a", "eu-west-3b"})
	if err != nil {
		t.Fatalf("InstanceTypeOfferings() error = %v", err)
	}
	if len(requests) != 2 || requests[0] != "" || requests[1] != "page-2" {
		t.Fatalf("EC2 pagination tokens = %#v, want [\"\", \"page-2\"]", requests)
	}
	if got := strings.Join(got, ","); got != "eu-west-3a,eu-west-3b" {
		t.Fatalf("InstanceTypeOfferings() = %q, want both pages", got)
	}
}

func TestSDKCapabilityClientFckNatAMICheckUsesOwnerArchitectureAndCurrentState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/" {
			t.Errorf("EC2 request = %s %s, want POST /", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse EC2 request: %v", err)
			return
		}
		if got := r.Form.Get("Action"); got != "DescribeImages" {
			t.Errorf("EC2 action = %q, want DescribeImages", got)
		}
		if got := r.Form.Get("Owner.1"); got != "568608671756" {
			t.Errorf("AMI owner = %q, want fck-nat publisher", got)
		}
		filters := make(map[string]string)
		for index := 1; ; index++ {
			name := r.Form.Get(fmt.Sprintf("Filter.%d.Name", index))
			if name == "" {
				break
			}
			filters[name] = r.Form.Get(fmt.Sprintf("Filter.%d.Value.1", index))
		}
		if filters["name"] != "fck-nat-al2023-*-arm64-ebs" || filters["architecture"] != "arm64" || filters["state"] != "available" {
			t.Errorf("AMI filters = %#v, want name/architecture/state constraints", filters)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><DescribeImagesResponse xmlns="http://ec2.amazonaws.com/doc/2016-11-15"><imagesSet><item><imageId>ami-fck</imageId><imageOwnerId>568608671756</imageOwnerId><architecture>arm64</architecture><name>fck-nat-al2023-20260801-arm64-ebs</name><imageState>available</imageState></item></imagesSet></DescribeImagesResponse>`)
	}))
	defer server.Close()

	configuration := testAWSConfig(server.Client())
	client := &SDKCapabilityClient{ec2: ec2.NewFromConfig(configuration, ec2Endpoint(server.URL))}
	available, err := client.FckNatAMI(context.Background(), "568608671756", "fck-nat-al2023-*-arm64-ebs", "arm64")
	if err != nil {
		t.Fatalf("FckNatAMI() error = %v", err)
	}
	if !available {
		t.Fatal("FckNatAMI() = false, want current arm64 image")
	}
}

func TestSDKCapabilityClientValkeyNodeTypeFiltersAndMatchesValkey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/" {
			t.Errorf("ElastiCache request = %s %s, want POST /", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse ElastiCache request: %v", err)
			return
		}
		if got := r.Form.Get("Action"); got != "DescribeReservedCacheNodesOfferings" {
			t.Errorf("ElastiCache action = %q", got)
		}
		if got := r.Form.Get("ProductDescription"); got != "valkey" {
			t.Errorf("ProductDescription = %q, want valkey", got)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><DescribeReservedCacheNodesOfferingsResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/"><DescribeReservedCacheNodesOfferingsResult><ReservedCacheNodesOfferings><ReservedCacheNodesOffering><CacheNodeType>cache.t4g.micro</CacheNodeType><ProductDescription>valkey</ProductDescription></ReservedCacheNodesOffering></ReservedCacheNodesOfferings></DescribeReservedCacheNodesOfferingsResult></DescribeReservedCacheNodesOfferingsResponse>`)
	}))
	defer server.Close()

	configuration := testAWSConfig(server.Client())
	client := &SDKCapabilityClient{elasticache: elasticache.NewFromConfig(configuration, elasticacheEndpoint(server.URL))}
	available, err := client.ValkeyNodeType(context.Background(), "cache.t4g.micro")
	if err != nil {
		t.Fatalf("ValkeyNodeType() error = %v", err)
	}
	if !available {
		t.Fatal("ValkeyNodeType() = false, want true for a valkey offering")
	}
}

func TestSDKCapabilityClientValkeyNodeTypeRejectsWrongProduct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse ElastiCache request: %v", err)
			return
		}
		if got := r.Form.Get("ProductDescription"); got != "valkey" {
			t.Errorf("ProductDescription = %q, want valkey", got)
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><DescribeReservedCacheNodesOfferingsResponse xmlns="http://elasticache.amazonaws.com/doc/2015-02-02/"><DescribeReservedCacheNodesOfferingsResult><ReservedCacheNodesOfferings><ReservedCacheNodesOffering><CacheNodeType>cache.t4g.micro</CacheNodeType><ProductDescription>redis</ProductDescription></ReservedCacheNodesOffering></ReservedCacheNodesOfferings></DescribeReservedCacheNodesOfferingsResult></DescribeReservedCacheNodesOfferingsResponse>`)
	}))
	defer server.Close()

	configuration := testAWSConfig(server.Client())
	client := &SDKCapabilityClient{elasticache: elasticache.NewFromConfig(configuration, elasticacheEndpoint(server.URL))}
	available, err := client.ValkeyNodeType(context.Background(), "cache.t4g.micro")
	if err != nil {
		t.Fatalf("ValkeyNodeType() error = %v", err)
	}
	if available {
		t.Fatal("ValkeyNodeType() = true for a non-Valkey offering")
	}
}

func TestSDKCapabilityClientMQCanonicalizesEngineType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Amazon MQ request method = %s, want GET", r.Method)
		}
		if got := r.URL.Query().Get("engineType"); got != "RABBITMQ" {
			t.Errorf("Amazon MQ engineType = %q, want RABBITMQ", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/broker-engine-types":
			_, _ = io.WriteString(w, `{"brokerEngineTypes":[{"engineType":"RABBITMQ","engineVersions":[{"name":"3.13"}]}],"maxResults":100}`)
		case "/v1/broker-instance-options":
			_, _ = io.WriteString(w, `{"brokerInstanceOptions":[{"engineType":"RABBITMQ","hostInstanceType":"mq.m7g.large","storageType":"EBS","supportedDeploymentModes":["CLUSTER_MULTI_AZ"],"supportedEngineVersions":["3.13"]}],"maxResults":100}`)
		default:
			t.Errorf("Amazon MQ path = %q", r.URL.Path)
		}
	}))
	defer server.Close()

	configuration := testAWSConfig(server.Client())
	client := &SDKCapabilityClient{mq: mq.NewFromConfig(configuration, mqEndpoint(server.URL))}
	available, err := client.MQEngineVersion(context.Background(), "RabbitMQ", "3.13")
	if err != nil {
		t.Fatalf("MQEngineVersion() error = %v", err)
	}
	if !available {
		t.Fatal("MQEngineVersion() = false, want true for RABBITMQ/3.13")
	}
	available, err = client.MQInstanceType(context.Background(), "RabbitMQ", "3.13", "mq.m7g.large")
	if err != nil {
		t.Fatalf("MQInstanceType() error = %v", err)
	}
	if !available {
		t.Fatal("MQInstanceType() = false, want true for RABBITMQ/3.13/mq.m7g.large")
	}
}

func testAWSConfig(httpClient *http.Client) awssdk.Config {
	return awssdk.Config{
		Region:      "eu-west-3",
		Credentials: credentials.NewStaticCredentialsProvider("AKIATEST", "secret", "session"),
		HTTPClient:  httpClient,
	}
}

func TestValidateSelectionFailsClosedForProviderCatalogGaps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeCapabilityAPI, *AdmissionSelection)
		want   string
	}{
		{name: "account mismatch", mutate: func(_ *fakeCapabilityAPI, selection *AdmissionSelection) { selection.AccountID = "000000000000" }, want: "credentials resolve to account"},
		{name: "zone unavailable", mutate: func(_ *fakeCapabilityAPI, selection *AdmissionSelection) {
			selection.AvailabilityZones[1] = "eu-west-3c"
		}, want: "availability zone"},
		{name: "instance absent", mutate: func(client *fakeCapabilityAPI, _ *AdmissionSelection) { client.instanceTypes["t4g.nano"] = false }, want: "instance type"},
		{name: "serverless bounds absent", mutate: func(client *fakeCapabilityAPI, _ *AdmissionSelection) {
			candidate := client.databaseVersions["aurora-mysql:8.0.mysql_aurora.3.08.2"]
			candidate.ServerlessV2MinCapacity = nil
			client.databaseVersions["aurora-mysql:8.0.mysql_aurora.3.08.2"] = candidate
		}, want: "Serverless v2 capacity limits"},
		{name: "unsupported EKS", mutate: func(client *fakeCapabilityAPI, _ *AdmissionSelection) {
			client.eksVersions["1.36"] = EKSVersion{Version: "1.36", Status: "UNSUPPORTED"}
		}, want: "not supported"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := validFakeCapabilityAPI()
			selection := validAdmissionSelection()
			test.mutate(client, &selection)
			if err := ValidateSelection(context.Background(), client, selection); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateSelection() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
