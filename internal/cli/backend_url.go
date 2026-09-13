package cli

import (
	"strings"

	"github.com/magelift/magelift/internal/platform"
)

type stateBackendURLProvider interface {
	StateBackendURL() string
}

func (o *options) infrastructureBackendURL(planned platform.PlannedStack) string {
	if url := strings.TrimSpace(getenvOrEmpty(o)("PULUMI_BACKEND_URL")); url != "" {
		return url
	}
	if planned == nil {
		return ""
	}
	provider, ok := planned.(stateBackendURLProvider)
	if !ok {
		return ""
	}
	return strings.TrimSpace(provider.StateBackendURL())
}
