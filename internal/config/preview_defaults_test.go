package config

import "testing"

// TestResolvePinsPreviewPromise pins the v1 preview promise per
// certified origin: database-backed queues and a single-AZ failure
// domain on both, disabled search on AWS, the 1-replica OpenSearch
// workload on GCP.
func TestResolvePinsPreviewPromise(t *testing.T) {
	t.Run("aws", func(t *testing.T) {
		input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build: {php: "8.5"}
target: {provider: aws, runtime: ecs-fargate, aws: {}}
defaults: {region: eu-west-3, preset: preview}
environments: {preview: {class: preview}}
`
		file, err := Load([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		effective, err := file.Resolve("preview", ResolveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		aws := effective.Config.Target.AWS
		if aws == nil {
			t.Fatal("resolved AWS target is nil")
		}
		if aws.Catalog.QueueMode != "db" {
			t.Fatalf("queueMode = %q, want db", aws.Catalog.QueueMode)
		}
		if aws.Catalog.SearchMode != "disabled" {
			t.Fatalf("searchMode = %q, want disabled", aws.Catalog.SearchMode)
		}
		if aws.NatTopology != "single-az" {
			t.Fatalf("natTopology = %q, want single-az", aws.NatTopology)
		}
	})

	t.Run("gcp", func(t *testing.T) {
		input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build: {php: "8.5"}
target:
  provider: gcp
  runtime: gke-autopilot
  gcp: {project: example-gcp-project}
defaults: {region: europe-west1, preset: preview}
environments: {preview: {class: preview}}
`
		file, err := Load([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		effective, err := file.Resolve("preview", ResolveOptions{})
		if err != nil {
			t.Fatal(err)
		}
		gcp := effective.Config.Target.GCP
		if gcp == nil {
			t.Fatal("resolved GCP target is nil")
		}
		if gcp.QueueMode != "database" {
			t.Fatalf("queueMode = %q, want database", gcp.QueueMode)
		}
		if gcp.OpenSearchMode != "opensearch" || gcp.OpenSearchReplicas != 1 {
			t.Fatalf("openSearch = %q x%d, want opensearch x1", gcp.OpenSearchMode, gcp.OpenSearchReplicas)
		}
		if gcp.CloudSQLAvailability != "ZONAL" {
			t.Fatalf("cloudSqlAvailability = %q, want ZONAL", gcp.CloudSQLAvailability)
		}
	})
}
