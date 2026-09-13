package kube

import (
	"strings"
	"testing"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type webShapeMocks struct {
	podSpec resource.PropertyMap
}

func (m *webShapeMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	if args.TypeToken == "kubernetes:apps/v1:Deployment" {
		spec := args.Inputs[resource.PropertyKey("spec")].ObjectValue()
		template := spec[resource.PropertyKey("template")].ObjectValue()
		m.podSpec = template[resource.PropertyKey("spec")].ObjectValue()
	}
	return args.Name + "-id", args.Inputs.Copy(), nil
}

func (*webShapeMocks) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func TestNginxFPMContainersServeHTTPAndKeepSecretsInPHP(t *testing.T) {
	mocks := &webShapeMocks{}
	env := corev1.EnvVarArray{&corev1.EnvVarArgs{Name: pulumi.String("DB_HOST"), Value: pulumi.String("secret")}}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := appsv1.NewDeployment(ctx, "shop-web", &appsv1.DeploymentArgs{
			Spec: &appsv1.DeploymentSpecArgs{
				Template: &corev1.PodTemplateSpecArgs{Spec: &corev1.PodSpecArgs{
					Containers: NginxFPMContainers("ghcr.io/magelift/shop@sha256:"+strings.Repeat("a", 64), env, "500m", "1Gi"),
				}},
			},
		})
		return err
	}, pulumi.WithMocks("magelift", "web", mocks))
	if err != nil {
		t.Fatal(err)
	}

	containers := mocks.podSpec[resource.PropertyKey("containers")].ArrayValue()
	if len(containers) != 2 {
		t.Fatalf("nginx-fpm pod must have php-fpm and nginx containers, got %d", len(containers))
	}
	php := containers[0].ObjectValue()
	if php[resource.PropertyKey("name")].StringValue() != "php-fpm" {
		t.Fatalf("first container is not php-fpm: %v", php)
	}
	if len(php[resource.PropertyKey("env")].ArrayValue()) != 1 {
		t.Fatalf("PHP-FPM container must receive Magento env: %v", php)
	}
	nginx := containers[1].ObjectValue()
	if nginx[resource.PropertyKey("name")].StringValue() != "web" {
		t.Fatalf("second container is not web: %v", nginx)
	}
	command := nginx[resource.PropertyKey("command")].ArrayValue()
	if len(command) != 3 || command[0].StringValue() != "nginx" || command[2].StringValue() != "daemon off;" {
		t.Fatalf("nginx command = %v", command)
	}
	probe := nginx[resource.PropertyKey("readinessProbe")].ObjectValue()
	httpGet := probe[resource.PropertyKey("httpGet")].ObjectValue()
	if httpGet[resource.PropertyKey("path")].StringValue() != "/health" || httpGet[resource.PropertyKey("port")].NumberValue() != applicationPort {
		t.Fatalf("nginx readiness probe = %v", httpGet)
	}
	if _, ok := nginx[resource.PropertyKey("env")]; ok {
		t.Fatal("nginx sidecar must not receive Magento secrets")
	}
}

func TestProcessWebRuntimeContainersServeHTTPWithMagentoEnvironment(t *testing.T) {
	for _, test := range []struct {
		webRuntime string
		command    string
	}{
		{webRuntime: "frankenphp-classic", command: "frankenphp"},
		{webRuntime: "php-apache", command: "sh"},
	} {
		test := test
		t.Run(test.webRuntime, func(t *testing.T) {
			mocks := &webShapeMocks{}
			env := corev1.EnvVarArray{&corev1.EnvVarArgs{Name: pulumi.String("DB_HOST"), Value: pulumi.String("secret")}}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				containers, err := WebRuntimeContainers(test.webRuntime, "ghcr.io/magelift/shop@sha256:"+strings.Repeat("a", 64), env, "500m", "1Gi")
				if err != nil {
					return err
				}
				_, err = appsv1.NewDeployment(ctx, "shop-web", &appsv1.DeploymentArgs{
					Spec: &appsv1.DeploymentSpecArgs{
						Template: &corev1.PodTemplateSpecArgs{Spec: &corev1.PodSpecArgs{Containers: containers}},
					},
				})
				return err
			}, pulumi.WithMocks("magelift", "web", mocks))
			if err != nil {
				t.Fatal(err)
			}
			containers := mocks.podSpec[resource.PropertyKey("containers")].ArrayValue()
			if len(containers) != 1 {
				t.Fatalf("%s pod containers = %d", test.webRuntime, len(containers))
			}
			web := containers[0].ObjectValue()
			if web[resource.PropertyKey("name")].StringValue() != "web" || web[resource.PropertyKey("command")].ArrayValue()[0].StringValue() != test.command {
				t.Fatalf("%s web container = %v", test.webRuntime, web)
			}
			if len(web[resource.PropertyKey("env")].ArrayValue()) != 1 {
				t.Fatalf("%s web container must receive Magento environment: %v", test.webRuntime, web)
			}
			probe := web[resource.PropertyKey("readinessProbe")].ObjectValue()
			httpGet := probe[resource.PropertyKey("httpGet")].ObjectValue()
			if httpGet[resource.PropertyKey("path")].StringValue() != "/health" || httpGet[resource.PropertyKey("port")].NumberValue() != applicationPort {
				t.Fatalf("%s readiness probe = %v", test.webRuntime, httpGet)
			}
		})
	}
	if _, err := WebRuntimeContainers("frankenphp-worker", "image", nil, "500m", "1Gi"); err == nil || !strings.Contains(err.Error(), `plugin "frankenphp-worker" is not registered`) {
		t.Fatalf("frankenphp-worker error = %v", err)
	}
}
