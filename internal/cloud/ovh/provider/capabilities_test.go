package ovhprovider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCapabilityClientReadsRegionAndNormalizesRegionName(t *testing.T) {
	client, err := NewCapabilityFromClient(apiFunc(func(_ context.Context, path string, response interface{}) error {
		if path != "/cloud/project/project-1/region/EU-WEST-PAR" {
			t.Fatalf("region path = %q", path)
		}
		*response.(*RegionCapability) = RegionCapability{
			RegionName:        "EU-WEST-PAR",
			Status:            "ENABLED",
			AvailabilityZones: []string{"eu-west-par-b", "eu-west-par-a"},
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	capability, err := client.Region(context.Background(), "project-1", "EU-WEST-PAR")
	if err != nil {
		t.Fatal(err)
	}
	if capability.Name != "EU-WEST-PAR" || capability.Status != "ENABLED" || len(capability.AvailabilityZones) != 2 {
		t.Fatalf("capability = %#v", capability)
	}
}

func TestCapabilityClientReadsProjectIdentity(t *testing.T) {
	client, err := NewCapabilityFromClient(apiFunc(func(_ context.Context, path string, response interface{}) error {
		if path != "/cloud/project/project-1" {
			t.Fatalf("project path = %q", path)
		}
		*response.(*ProjectCapability) = ProjectCapability{ID: "project-1"}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	capability, err := client.Project(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if capability.ID != "project-1" {
		t.Fatalf("project capability = %#v", capability)
	}
}

func TestCapabilityClientAcceptsCurrentProjectIDField(t *testing.T) {
	client, err := NewCapabilityFromClient(apiFunc(func(_ context.Context, path string, response interface{}) error {
		if path != "/cloud/project/project-1" {
			t.Fatalf("project path = %q", path)
		}
		*response.(*ProjectCapability) = ProjectCapability{ProjectID: "project-1", ServiceName: "2780567163019982"}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	capability, err := client.Project(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if capability.ID != "project-1" {
		t.Fatalf("project ID = %q, want project-1", capability.ID)
	}
}

func TestCapabilityClientNormalizesProviderErrors(t *testing.T) {
	client, err := NewCapabilityFromClient(apiFunc(func(context.Context, string, interface{}) error {
		return errors.New("provider response contained an application-secret")
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Region(context.Background(), "project-1", "EU-WEST-PAR")
	if err == nil || !strings.Contains(err.Error(), "capability lookup failed") || strings.Contains(err.Error(), "application-secret") {
		t.Fatalf("normalized error = %v", err)
	}
}

func TestCapabilityClientReadsDatabaseAvailability(t *testing.T) {
	client, err := NewCapabilityFromClient(apiFunc(func(_ context.Context, path string, response interface{}) error {
		if path != "/cloud/project/project-1/database/availability" {
			t.Fatalf("database availability path = %q", path)
		}
		*response.(*[]DatabaseAvailability) = []DatabaseAvailability{
			{
				Engine:        "valkey",
				Version:       "8.1",
				Plan:          "discovery",
				Flavor:        "b3-8",
				Region:        "EU-WEST-PAR",
				Network:       "private",
				MinNodeNumber: 1,
				MaxNodeNumber: 1,
			},
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	availability, err := client.DatabaseAvailability(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(availability) != 1 || availability[0].Engine != "valkey" || availability[0].Network != "private" {
		t.Fatalf("database availability = %#v", availability)
	}
}

func TestCapabilityClientNormalizesDatabaseAvailabilityErrors(t *testing.T) {
	client, err := NewCapabilityFromClient(apiFunc(func(context.Context, string, interface{}) error {
		return errors.New("provider response contained an application-secret")
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.DatabaseAvailability(context.Background(), "project-1")
	if err == nil || !strings.Contains(err.Error(), "database availability lookup failed") || strings.Contains(err.Error(), "application-secret") {
		t.Fatalf("normalized database availability error = %v", err)
	}
}
