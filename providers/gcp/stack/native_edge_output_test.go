package stack

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const nativeEdgeMockIngressIP = "192.0.2.10"

type nativeEdgeOutputMocks struct {
	*stackMocks
	urlMapName string
}

func (m *nativeEdgeOutputMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	id, state, err := m.stackMocks.NewResource(args)
	if err != nil {
		return id, state, err
	}
	if args.TypeToken == "kubernetes:networking.k8s.io/v1:Ingress" {
		metadata := state["metadata"].ObjectValue().Copy()
		metadata["namespace"] = resource.NewStringProperty("default")
		annotations := metadata["annotations"].ObjectValue().Copy()
		annotations["ingress.kubernetes.io/redirect-url-map"] = resource.NewStringProperty("redirect-only")
		if m.urlMapName != "" {
			annotations["ingress.kubernetes.io/url-map"] = resource.NewStringProperty(m.urlMapName)
		}
		metadata["annotations"] = resource.NewObjectProperty(annotations)
		state["metadata"] = resource.NewObjectProperty(metadata)
		state["status"] = resource.NewObjectProperty(resource.PropertyMap{
			"loadBalancer": resource.NewObjectProperty(resource.PropertyMap{
				"ingress": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{"ip": resource.NewStringProperty(nativeEdgeMockIngressIP)}),
				}),
			}),
		})
	}
	return id, state, nil
}

func TestProgramNativeEdgeOutputsUseDomainAndRawAddress(t *testing.T) {
	t.Run("serving URL map", func(t *testing.T) { checkNativeEdgeOutputs(t, "k8s2-um-serving-shop") })
	t.Run("controller has not published the URL map", func(t *testing.T) { checkNativeEdgeOutputs(t, "") })
}

func checkNativeEdgeOutputs(t *testing.T, urlMapName string) {
	t.Helper()
	in := gcpDeploymentInputs()
	in.Envelope.Domain = "shop.example.com"
	in.Edge = sdk.EdgeIntent{
		Mode: "native", NativeProvider: "cloud-cdn", Domains: []string{"shop.example.com"},
		TLS: true, TLSMode: "gke-managed", DNSMode: "customer-managed", OriginHealthRef: "health/shop",
		OwnershipMarker: "shop/staging",
	}
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	mocks := &nativeEdgeOutputMocks{stackMocks: &stackMocks{}, urlMapName: urlMapName}
	var applicationURL, ingressAddress string
	var ingressOwner, ingressNamespace, servingMap string
	err = pulumi.RunErr(func(ctx *pulumi.Context) error {
		if err := Program(spec)(ctx); err != nil {
			return err
		}
		exports := ctx.GetCurrentExportMap()
		address, ok := exports["nativeEdgeIngressAddress"]
		if !ok || address == nil {
			t.Fatal("stack outputs missing nativeEdgeIngressAddress")
		}
		pulumi.All(exports["applicationURL"], address, exports["nativeEdgeIngressOwner"], exports["nativeEdgeIngressNamespace"], exports["nativeEdgeURLMapName"]).ApplyT(func(values []interface{}) string {
			applicationURL, _ = values[0].(string)
			ingressAddress, _ = values[1].(string)
			ingressOwner, _ = values[2].(string)
			ingressNamespace, _ = values[3].(string)
			servingMap, _ = values[4].(string)
			return ""
		})
		return nil
	}, pulumi.WithMocks("magelift", "shop-staging", mocks))
	if err != nil {
		t.Fatal(err)
	}
	if applicationURL != "https://shop.example.com" {
		t.Fatalf("applicationURL = %q", applicationURL)
	}
	if strings.Contains(applicationURL, nativeEdgeMockIngressIP) {
		t.Fatalf("applicationURL contains the load balancer address: %q", applicationURL)
	}
	if ingressAddress != nativeEdgeMockIngressIP {
		t.Fatalf("nativeEdgeIngressAddress = %q", ingressAddress)
	}
	if ingressOwner != in.Edge.OwnershipMarker || ingressNamespace != "default" || servingMap != urlMapName {
		t.Fatalf("Ingress owner=%q namespace=%q serving URL map=%q", ingressOwner, ingressNamespace, servingMap)
	}
	ingresses := 0
	for _, res := range mocks.resources {
		if res.TypeToken == "kubernetes:networking.k8s.io/v1:Ingress" {
			ingresses++
		}
		if strings.Contains(res.TypeToken, "dns") || strings.Contains(res.Name, "native-edge-ingress-address") {
			t.Fatalf("native edge address output created a resource: %s %s", res.TypeToken, res.Name)
		}
	}
	if ingresses != 1 {
		t.Fatalf("ingress resources = %d", ingresses)
	}
}
