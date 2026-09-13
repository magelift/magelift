package eks

import (
	"encoding/json"
	"testing"
)

func TestEKSNodePolicies(t *testing.T) {
	tests := []struct {
		name string
		mode string
		want []nodePolicy
	}{
		{
			name: "auto mode uses managed worker policy",
			mode: ComputeModeAuto,
			want: []nodePolicy{
				{suffix: "worker", arn: "arn:aws:iam::aws:policy/AmazonEKSWorkerNodeMinimalPolicy"},
				{suffix: "ecr", arn: "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly"},
			},
		},
		{
			name: "managed node groups include CNI and EBS",
			mode: ComputeModeManagedNodes,
			want: []nodePolicy{
				{suffix: "worker", arn: "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"},
				{suffix: "ecr", arn: "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly"},
				{suffix: "cni", arn: "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"},
			},
		},
		{
			name: "self-managed nodes include CNI and EBS",
			mode: ComputeModeSelfManaged,
			want: []nodePolicy{
				{suffix: "worker", arn: "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"},
				{suffix: "ecr", arn: "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly"},
				{suffix: "cni", arn: "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := eksNodePolicies(test.mode)
			if len(got) != len(test.want) {
				t.Fatalf("eksNodePolicies(%q) returned %d policies, want %d: %#v", test.mode, len(got), len(test.want), got)
			}
			for index := range test.want {
				if got[index] != test.want[index] {
					t.Errorf("eksNodePolicies(%q)[%d] = %#v, want %#v", test.mode, index, got[index], test.want[index])
				}
			}
		})
	}
}

func TestEBSCSITrustPolicyScopesToControllerServiceAccount(t *testing.T) {
	policyJSON, err := ebsCSITrustPolicy(
		"https://oidc.eks.eu-west-3.amazonaws.com/id/example/",
		"arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/example",
	)
	if err != nil {
		t.Fatal(err)
	}
	var policy map[string]any
	if err := json.Unmarshal([]byte(policyJSON), &policy); err != nil {
		t.Fatal(err)
	}
	statement := policy["Statement"].([]any)[0].(map[string]any)
	if got := statement["Action"]; got != "sts:AssumeRoleWithWebIdentity" {
		t.Fatalf("trust action = %v", got)
	}
	principal := statement["Principal"].(map[string]any)
	if got := principal["Federated"]; got != "arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/example" {
		t.Fatalf("federated principal = %v", got)
	}
	condition := statement["Condition"].(map[string]any)["StringEquals"].(map[string]any)
	if got := condition["oidc.eks.eu-west-3.amazonaws.com/id/example:aud"]; got != "sts.amazonaws.com" {
		t.Fatalf("audience condition = %v", got)
	}
	if got := condition["oidc.eks.eu-west-3.amazonaws.com/id/example:sub"]; got != "system:serviceaccount:kube-system:ebs-csi-controller-sa" {
		t.Fatalf("subject condition = %v", got)
	}
}

func TestEBSCSITrustPolicyRejectsInvalidIssuer(t *testing.T) {
	if _, err := ebsCSITrustPolicy("oidc.eks.eu-west-3.amazonaws.com/id/example", "arn:aws:iam::123456789012:oidc-provider/example"); err == nil {
		t.Fatal("trust policy accepted an issuer without https")
	}
}

func TestKubeServerURL(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{name: "AWS endpoint", endpoint: "https://cluster.eks.amazonaws.com", want: "https://cluster.eks.amazonaws.com"},
		{name: "bare endpoint", endpoint: "cluster.eks.amazonaws.com", want: "https://cluster.eks.amazonaws.com"},
		{name: "trim whitespace", endpoint: "  https://cluster.eks.amazonaws.com  ", want: "https://cluster.eks.amazonaws.com"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := kubeServerURL(test.endpoint); got != test.want {
				t.Fatalf("kubeServerURL(%q) = %q, want %q", test.endpoint, got, test.want)
			}
		})
	}
}

func TestEKSWebServiceAnnotations(t *testing.T) {
	t.Parallel()
	auto := eksWebServiceAnnotations(ComputeModeAuto)
	if _, ok := auto["pulumi.com/skipAwait"]; !ok {
		t.Fatal("Auto Mode web Service must skip Pulumi LoadBalancer await")
	}
	if _, ok := auto["service.beta.kubernetes.io/aws-load-balancer-scheme"]; !ok {
		t.Fatal("Auto Mode web Service must request an internet-facing NLB")
	}
	managed := eksWebServiceAnnotations(ComputeModeManagedNodes)
	if _, ok := managed["service.beta.kubernetes.io/aws-load-balancer-scheme"]; ok {
		t.Fatal("non-Auto Mode web Service must not set the Auto Mode NLB scheme")
	}
}

func TestStorefrontURLFromLoadBalancer(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		hostname string
		ip       string
		want     string
	}{
		{name: "hostname", hostname: "k8s-shop.elb.amazonaws.com", want: "http://k8s-shop.elb.amazonaws.com"},
		{name: "ip fallback", ip: "203.0.113.10", want: "http://203.0.113.10"},
		{name: "hostname wins", hostname: "k8s-shop.elb.amazonaws.com", ip: "203.0.113.10", want: "http://k8s-shop.elb.amazonaws.com"},
		{name: "empty", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := storefrontURLFromLoadBalancer(test.hostname, test.ip); got != test.want {
				t.Fatalf("storefrontURLFromLoadBalancer(%q, %q) = %q, want %q", test.hostname, test.ip, got, test.want)
			}
		})
	}
}
