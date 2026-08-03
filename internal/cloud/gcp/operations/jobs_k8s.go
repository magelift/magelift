package operations

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/magelift/magelift/internal/platform"
)

// k8sJobs implements JobAPI via client-go (no kubectl dependency).
type k8sJobs struct {
	client kubernetes.Interface
}

func (k k8sJobs) CreateJob(ctx context.Context, namespace string, job *batchv1.Job) (string, error) {
	if k.client == nil || job == nil {
		return "", fmt.Errorf("kubernetes job client and job spec are required")
	}
	if namespace == "" {
		namespace = "default"
	}
	created, err := k.client.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create migrate Job: %w", err)
	}
	return created.Name, nil
}

func (k k8sJobs) WaitJob(ctx context.Context, namespace, name string, timeout time.Duration) error {
	if k.client == nil {
		return fmt.Errorf("kubernetes job client is required")
	}
	if namespace == "" {
		namespace = "default"
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		job, err := k.client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("wait for migrate Job: %w", err)
		}
		for _, condition := range job.Status.Conditions {
			switch condition.Type {
			case batchv1.JobComplete:
				if condition.Status == corev1.ConditionTrue {
					return nil
				}
			case batchv1.JobFailed:
				if condition.Status == corev1.ConditionTrue {
					msg := condition.Message
					if msg == "" {
						msg = condition.Reason
					}
					return fmt.Errorf("wait for migrate Job: job failed: %s", msg)
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("wait for migrate Job: timed out after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for migrate Job: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (k k8sJobs) DeleteJob(ctx context.Context, namespace, name string) error {
	if k.client == nil {
		return nil
	}
	if namespace == "" {
		namespace = "default"
	}
	propagation := metav1.DeletePropagationBackground
	err := k.client.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete migrate Job: %w", err)
	}
	return nil
}

func migrationJob(name string, request CandidateRequest) *batchv1.Job {
	cpu := request.CPURequest
	if cpu == "" {
		cpu = "500m"
	}
	memory := request.MemoryRequest
	if memory == "" {
		memory = "1Gi"
	}
	bindings := platform.CoreEnvBindings(platform.CapabilityEndpoints{
		ApplicationMode: request.ApplicationMode,
		WebRuntime:      request.WebRuntime,
		DatabaseWriter:  request.DatabaseWriter,
		DatabaseName:    request.DatabaseName,
		CacheEndpoint:   request.CacheEndpoint,
	})
	env := make([]corev1.EnvVar, 0, len(bindings))
	for _, binding := range bindings {
		env = append(env, corev1.EnvVar{Name: binding.Name, Value: binding.Value})
	}
	backoff := int32(0)
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"magelift.io/workload": "migrate",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoff,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    "deploy",
						Image:   request.ImageDigest,
						Command: platform.MagentoMigrationShell(),
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpu),
								corev1.ResourceMemory: resource.MustParse(memory),
							},
						},
						Env: env,
					}},
				},
			},
		},
	}
}
