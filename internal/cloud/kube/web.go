package kube

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	applicationPort                 = 8080
	DefaultApplicationMemoryRequest = "2Gi"
)

// NginxFPMContainers runs the shared PHP runtime in the same shape as the ECS
// adapter: PHP-FPM receives Magento configuration, while nginx owns HTTP and
// exposes the pod readiness endpoint. A single PHP-FPM process cannot serve
// the nginx-fpm image over the Kubernetes Service port by itself.
func NginxFPMContainers(image string, env corev1.EnvVarArrayInput, cpuRequest, memoryRequest string) corev1.ContainerArray {
	phpResources := applicationResources(cpuRequest, memoryRequest)
	nginxResources := applicationResources("100m", "128Mi")
	return corev1.ContainerArray{
		&corev1.ContainerArgs{
			Name: pulumi.String("php-fpm"), Image: pulumi.String(image), Env: env, Resources: phpResources,
		},
		&corev1.ContainerArgs{
			Name: pulumi.String("web"), Image: pulumi.String(image), Command: ToStringArray([]string{"nginx", "-g", "daemon off;"}),
			Ports:          corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(applicationPort)}},
			ReadinessProbe: readinessProbe(),
			Resources:      nginxResources,
		},
	}
}

// FrankenPHPClassicContainers runs FrankenPHP in its classic PHP-server mode.
// Worker mode is deliberately not a supported MageLift web runtime.
func FrankenPHPClassicContainers(image string, env corev1.EnvVarArrayInput, cpuRequest, memoryRequest string) corev1.ContainerArray {
	return processWebContainers(image, env, []string{"frankenphp", "run"}, cpuRequest, memoryRequest)
}

// PHPApacheContainers runs the Apache HTTPD and PHP-FPM runtime image as one
// container. Apache owns the Service port and proxies PHP requests to FPM.
func PHPApacheContainers(image string, env corev1.EnvVarArrayInput, cpuRequest, memoryRequest string) corev1.ContainerArray {
	return processWebContainers(image, env, []string{"sh", "-eu", "-c", "php-fpm --daemonize && exec apache2ctl -D FOREGROUND"}, cpuRequest, memoryRequest)
}

// WebRuntimeContainers maps registered runtime IDs to Kubernetes pod shapes.
// Admission is performed in config; this layer still rejects an unknown ID so
// direct runtime construction cannot silently deploy an unsupported frontend.
func WebRuntimeContainers(webRuntime, image string, env corev1.EnvVarArrayInput, cpuRequest, memoryRequest string) (corev1.ContainerArray, error) {
	switch webRuntime {
	case "nginx-fpm":
		return NginxFPMContainers(image, env, cpuRequest, memoryRequest), nil
	case "frankenphp-classic":
		return FrankenPHPClassicContainers(image, env, cpuRequest, memoryRequest), nil
	case "php-apache":
		return PHPApacheContainers(image, env, cpuRequest, memoryRequest), nil
	default:
		return nil, fmt.Errorf("web runtime plugin %q is not registered", webRuntime)
	}
}

func processWebContainers(image string, env corev1.EnvVarArrayInput, command []string, cpuRequest, memoryRequest string) corev1.ContainerArray {
	return corev1.ContainerArray{
		&corev1.ContainerArgs{
			Name:           pulumi.String("web"),
			Image:          pulumi.String(image),
			Command:        ToStringArray(command),
			Env:            env,
			Ports:          corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(applicationPort)}},
			ReadinessProbe: readinessProbe(),
			Resources:      applicationResources(cpuRequest, memoryRequest),
		},
	}
}

func applicationResources(cpuRequest, memoryRequest string) *corev1.ResourceRequirementsArgs {
	return &corev1.ResourceRequirementsArgs{
		Requests: pulumi.StringMap{"cpu": pulumi.String(cpuRequest), "memory": pulumi.String(memoryRequest)},
	}
}

func readinessProbe() *corev1.ProbeArgs {
	return &corev1.ProbeArgs{
		HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/health"), Port: pulumi.Int(applicationPort)},
		InitialDelaySeconds: pulumi.Int(2), PeriodSeconds: pulumi.Int(5), TimeoutSeconds: pulumi.Int(2), FailureThreshold: pulumi.Int(12),
	}
}
