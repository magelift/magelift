package runtime

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
)

func appendSearchProxy(args Args, containers []containerDefinition, endpoint string) ([]containerDefinition, error) {
	if args.SearchProxyImage == "" {
		return containers, nil
	}
	host, service := searchProxyTarget(endpoint)
	if host == "" {
		return nil, errors.New("OpenSearch endpoint is required for the signing proxy")
	}
	proxy := containerDefinition{
		Name:                   "search-proxy",
		Image:                  args.SearchProxyImage,
		Command:                []string{"--port", searchProxyListenAddr(), "--name", service, "--region", args.Region, "--host", host, "--sign-host", host, "--upstream-url-scheme", "https"},
		Essential:              searchProxyEssential(containers),
		ReadonlyRootFilesystem: true,
		LinuxParameters:        containerLinux{InitProcessEnabled: false},
		LogConfiguration:       awslogsConfig(args, "web"),
	}
	for index := range containers {
		// nginx-fpm has a web sidecar with no Magento environment; only its
		// PHP-FPM process needs the signed search endpoint. Process runtimes
		// put Magento in web, so web must wait for the proxy instead.
		if containers[index].Name != "web" || args.WebRuntime != "nginx-fpm" {
			containers[index].DependsOn = []containerDependency{{ContainerName: "search-proxy", Condition: "START"}}
		}
	}
	return append(containers, proxy), nil
}

func appendVarnish(args Args, containers []containerDefinition) ([]containerDefinition, error) {
	if args.ApplicationMode != "integrated" {
		return containers, nil
	}
	if !imageDigest.MatchString(args.VarnishImage) {
		return nil, errors.New("integrated runtime requires a Varnish image pinned by a lowercase SHA-256 digest")
	}
	memory, _ := strconv.Atoi(args.TaskMemory)
	// Keep malloc well under task memory. Prior Magento-on-AWS Terraform stacks run
	// Varnish in a dedicated task; colocated preview tasks must stay lean.
	varnishMallocMiB := 64
	if memory >= 2048 {
		varnishMallocMiB = 256
	} else if memory >= 1024 {
		varnishMallocMiB = 128
	}
	return append(containers, containerDefinition{
		Name:      "varnish",
		Image:     args.VarnishImage,
		Essential: true,
		User:      "varnish",
		// Official image entrypoint appends hyphen flags to varnishd. -n must
		// point at writable storage: Fargate /dev/shm is only 64 MiB and is
		// where Varnish places VSM when -n is unset. Use the image /tmp (1777),
		// not a Fargate empty volume; those mount as root:root and break -n.
		Command:                []string{"-n", "/tmp/varnish", "-p", "vsl_space=4m"},
		ReadonlyRootFilesystem: false,
		LinuxParameters:        containerLinux{InitProcessEnabled: false},
		PortMappings:           []containerPort{{ContainerPort: VarnishPort, Protocol: "tcp"}},
		DependsOn:              []containerDependency{{ContainerName: "web", Condition: "HEALTHY"}},
		LogConfiguration:       awslogsConfig(args, "web"),
		Environment: []containerEnvironment{
			{Name: "VARNISH_BACKEND_HOST", Value: "127.0.0.1"},
			{Name: "VARNISH_BACKEND_PORT", Value: strconv.Itoa(ApplicationPort)},
			{Name: "VARNISH_HTTP_PORT", Value: strconv.Itoa(VarnishPort)},
			{Name: "VARNISH_SIZE", Value: strconv.Itoa(varnishMallocMiB) + "m"},
		},
	}), nil
}

// searchProxyListenAddr is the aws-sigv4-proxy --port value. That flag is a
// Go listen address (":8081"), not a bare port; "8081" fatals with
// "listen tcp: address 8081: missing port in address".
func searchProxyListenAddr() string {
	return ":" + strconv.Itoa(searchProxyPort)
}

// searchProxyEssential is false on one-off Magento migrate tasks. The deploy
// container exits 0 after setup:upgrade; an essential signing proxy then
// receives SIGTERM and ECS reports the whole task as failed.
func searchProxyEssential(containers []containerDefinition) bool {
	for _, container := range containers {
		if container.Name == "deploy" {
			return false
		}
	}
	return true
}

func searchProxyTarget(endpoint string) (string, string) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		parsed, _ = url.Parse("https://" + endpoint)
	}
	host := parsed.Hostname()
	service := "es"
	if strings.Contains(host, ".aoss.") {
		service = "aoss"
	}
	return host, service
}
