package operations

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	container "cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// newGKEClientset builds a kubernetes client for a regional Autopilot cluster using ADC.
func newGKEClientset(ctx context.Context, project, region, cluster string) (kubernetes.Interface, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(region) == "" || strings.TrimSpace(cluster) == "" {
		return nil, errors.New("GCP project, region, and cluster are required")
	}
	cfg, err := restConfigForGKE(ctx, project, region, cluster)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes clientset: %w", err)
	}
	return clientset, nil
}

func restConfigForGKE(ctx context.Context, project, region, cluster string) (*rest.Config, error) {
	ts, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("GCP token source: %w", err)
	}
	client, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GKE client: %w", err)
	}
	defer client.Close()

	name := fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, region, cluster)
	resp, err := client.GetCluster(ctx, &containerpb.GetClusterRequest{Name: name})
	if err != nil {
		return nil, fmt.Errorf("get GKE cluster %s: %w", cluster, err)
	}
	if resp.GetEndpoint() == "" || resp.GetMasterAuth() == nil || resp.GetMasterAuth().GetClusterCaCertificate() == "" {
		return nil, fmt.Errorf("GKE cluster %s is missing endpoint or CA certificate", cluster)
	}
	ca, err := base64.StdEncoding.DecodeString(resp.GetMasterAuth().GetClusterCaCertificate())
	if err != nil {
		return nil, fmt.Errorf("decode GKE CA certificate: %w", err)
	}
	cfg := &rest.Config{
		Host: "https://" + resp.GetEndpoint(),
		TLSClientConfig: rest.TLSClientConfig{
			CAData: ca,
		},
	}
	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return &oauth2.Transport{Source: ts, Base: rt}
	})
	return cfg, nil
}
