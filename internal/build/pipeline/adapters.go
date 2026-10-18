package pipeline

import (
	"context"
	"io"

	"github.com/magelift/magelift/internal/containerrunner"
)

type ContainerAdapter struct {
	Binary          string
	TempRoot        string
	Stderr          io.Writer
	Network         string
	Platform        string
	MageLiftVersion string
}

func (adapter ContainerAdapter) Run(ctx context.Context, image, source string, request []byte, secrets ...containerrunner.Secret) (containerrunner.Result, error) {
	return adapter.runner(image, adapter.Network).Run(ctx, source, request, secrets...)
}

func (adapter ContainerAdapter) RunInOutput(ctx context.Context, image, source, output string, request []byte, secrets ...containerrunner.Secret) (containerrunner.Result, error) {
	return adapter.runner(image, "none").RunInOutput(ctx, source, output, request, secrets...)
}

func (adapter ContainerAdapter) runner(image, network string) containerrunner.Runner {
	return containerrunner.Runner{
		Binary:          adapter.Binary,
		Image:           image,
		Platform:        adapter.Platform,
		MageLiftVersion: adapter.MageLiftVersion,
		TempRoot:        adapter.TempRoot,
		Stderr:          adapter.Stderr,
		Network:         network,
	}
}
