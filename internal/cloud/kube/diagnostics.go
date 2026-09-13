package kube

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	migrationContainerName = "deploy"
	maxMigrationLogBytes   = 16 * 1024
	migrationLogTailLines  = int64(200)
)

var migrationSensitiveValue = regexp.MustCompile(`(?i)(password|passwd|secret|token|authorization)(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;}\]]+)`)

// JobFailureDiagnostics returns bounded logs from the migration Job's pod.
// It is called before the Job is deleted, so a failed deployment still has a
// useful Magento error attached to the operation result.
func JobFailureDiagnostics(ctx context.Context, client kubernetes.Interface, namespace, name string) string {
	if client == nil || strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
		return ""
	}

	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + name,
	})
	if err != nil {
		return ""
	}

	parts := make([]string, 0, len(pods.Items))
	for _, pod := range pods.Items {
		logs, err := migrationPodLogs(ctx, client, namespace, pod.Name)
		if err != nil || strings.TrimSpace(logs) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("pod %s:\n%s", pod.Name, logs))
	}
	return strings.Join(parts, "\n")
}

func migrationPodLogs(ctx context.Context, client kubernetes.Interface, namespace, podName string) (string, error) {
	tailLines := migrationLogTailLines
	request := client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: migrationContainerName,
		TailLines: &tailLines,
	})
	stream, err := request.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	data, err := io.ReadAll(io.LimitReader(stream, maxMigrationLogBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxMigrationLogBytes {
		data = append(data[:maxMigrationLogBytes], []byte("\n[log output truncated]")...)
	}
	return redactMigrationLogs(strings.TrimSpace(string(data))), nil
}

func redactMigrationLogs(logs string) string {
	return migrationSensitiveValue.ReplaceAllString(logs, "$1$2[redacted]")
}
