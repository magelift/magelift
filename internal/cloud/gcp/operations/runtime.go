package operations

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ServiceHealth is the GKE Deployment readiness snapshot used by Magento health.
type ServiceHealth struct {
	DesiredReplicas int
	ReadyReplicas   int
	Available       bool
}

// RuntimeStore checks GKE Deployment readiness for Magento web.
type RuntimeStore struct {
	project string
	region  string
	check   func(ctx context.Context, project, region, cluster, deployment string) (ServiceHealth, error)
}

func NewRuntime(ctx context.Context, project, region string) (*RuntimeStore, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(region) == "" {
		return nil, errors.New("GCP project and region are required")
	}
	return &RuntimeStore{
		project: project,
		region:  region,
		check: func(ctx context.Context, project, region, cluster, deployment string) (ServiceHealth, error) {
			clientset, err := newGKEClientset(ctx, project, region, cluster)
			if err != nil {
				return ServiceHealth{}, err
			}
			return deploymentHealth(ctx, clientset, "default", deployment)
		},
	}, nil
}

func NewRuntimeFromFunc(check func(context.Context, string, string, string, string) (ServiceHealth, error)) *RuntimeStore {
	return &RuntimeStore{check: check}
}

func (s *RuntimeStore) Check(ctx context.Context, cluster, serviceName string) (ServiceHealth, error) {
	if s == nil || s.check == nil {
		return ServiceHealth{}, errors.New("GKE runtime checker is required")
	}
	if strings.TrimSpace(cluster) == "" || strings.TrimSpace(serviceName) == "" {
		return ServiceHealth{}, errors.New("cluster and service name are required")
	}
	// Stack exports the Service name; web Deployment shares that name.
	return s.check(ctx, s.project, s.region, cluster, serviceName)
}

func deploymentHealth(ctx context.Context, client kubernetes.Interface, namespace, deployment string) (ServiceHealth, error) {
	if client == nil {
		return ServiceHealth{}, errors.New("kubernetes client is required")
	}
	if namespace == "" {
		namespace = "default"
	}
	dep, err := client.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return ServiceHealth{}, fmt.Errorf("get deployment %s: %w", deployment, err)
	}
	desired := 0
	if dep.Spec.Replicas != nil {
		desired = int(*dep.Spec.Replicas)
	}
	available := false
	for _, condition := range dep.Status.Conditions {
		if condition.Type == "Available" && condition.Status == corev1.ConditionTrue {
			available = true
			break
		}
	}
	return ServiceHealth{
		DesiredReplicas: desired,
		ReadyReplicas:   int(dep.Status.ReadyReplicas),
		Available:       available,
	}, nil
}

// ObserveStore implements Magento RuntimeObserve against GKE.
type ObserveStore struct {
	project string
	region  string
}

func NewObserve(_ context.Context, project, region string) (*ObserveStore, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(region) == "" {
		return nil, errors.New("GCP project and region are required")
	}
	return &ObserveStore{project: project, region: region}, nil
}

func (s *ObserveStore) TailLogs(ctx context.Context, cluster, workload string, since time.Time, limit int) ([]LogLine, error) {
	if limit <= 0 {
		limit = 100
	}
	deployment := workload
	if deployment == "" {
		deployment = "web"
	}
	clientset, err := newGKEClientset(ctx, s.project, s.region, cluster)
	if err != nil {
		return nil, err
	}
	opts := &corev1.PodLogOptions{
		TailLines: int64Ptr(int64(limit)),
	}
	if !since.IsZero() {
		opts.SinceTime = &metav1.Time{Time: since.UTC()}
	}
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + deployment,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s: %w", deployment, err)
	}
	events := make([]LogLine, 0, limit)
	now := time.Now().UTC()
	for _, pod := range pods.Items {
		req := clientset.CoreV1().Pods("default").GetLogs(pod.Name, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(stream)
		_ = stream.Close()
		if readErr != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			events = append(events, LogLine{Timestamp: now, Message: line})
			if len(events) >= limit {
				return events, nil
			}
		}
	}
	return events, nil
}

func int64Ptr(v int64) *int64 { return &v }

type LogLine struct {
	Timestamp time.Time
	Message   string
}
