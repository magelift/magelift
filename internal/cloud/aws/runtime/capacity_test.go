package runtime

import "testing"

func TestManagedInstanceCPUMemory(t *testing.T) {
	t.Parallel()
	cases := []struct {
		instanceType      string
		wantVCPU, wantMiB int
	}{
		{"m6i.large", 2, 8192},
		{"", 2, 8192},
		{"t3.medium", 2, 4096},
		{"c7i.large", 2, 4096},
		{"C7I.LARGE", 2, 4096},
	}
	for _, tc := range cases {
		t.Run(tc.instanceType, func(t *testing.T) {
			t.Parallel()
			vcpu, memoryMiB := managedInstanceCPUMemory(tc.instanceType)
			if vcpu != tc.wantVCPU || memoryMiB != tc.wantMiB {
				t.Fatalf("managedInstanceCPUMemory(%q) = %d/%d, want %d/%d", tc.instanceType, vcpu, memoryMiB, tc.wantVCPU, tc.wantMiB)
			}
		})
	}
}

func TestShouldSoakManagedInstanceIAM(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                string
		dryRun, underGoTest bool
		want                bool
	}{
		{"live update", false, false, true},
		{"pulumi preview", true, false, false},
		{"unit test mocks", false, true, false},
		{"preview in test", true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldSoakManagedInstanceIAM(tc.dryRun, tc.underGoTest); got != tc.want {
				t.Fatalf("shouldSoakManagedInstanceIAM(%v, %v) = %v, want %v", tc.dryRun, tc.underGoTest, got, tc.want)
			}
		})
	}
}

func TestECSCapacityProviderName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, suffix, want string
	}{
		{"shop", "ec2", "shop-ec2"},
		{"awsap-preview-runtime", "ec2", "ml-awsap-preview-runtime-ec2"},
		{"ecs-foo", "managed", "ml-ecs-foo-managed"},
		{"FargateShop", "ec2", "ml-FargateShop-ec2"},
		{"ml-shop", "ec2", "ml-shop-ec2"},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/"+tc.suffix, func(t *testing.T) {
			t.Parallel()
			if got := ecsCapacityProviderName(tc.name, tc.suffix); got != tc.want {
				t.Fatalf("ecsCapacityProviderName(%q, %q) = %q, want %q", tc.name, tc.suffix, got, tc.want)
			}
		})
	}
}
