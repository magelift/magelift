package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"
)

type clientCommandFactory func(context.Context, string, ...string) *exec.Cmd

type Client struct {
	executable  string
	diagnostics io.Writer
	command     clientCommandFactory
}

func NewClient(executable string, diagnostics io.Writer) *Client {
	return &Client{executable: executable, diagnostics: diagnostics, command: exec.CommandContext}
}

func (c *Client) RoundTrip(ctx context.Context, request Request) (Response, error) {
	if c.executable == "" || c.diagnostics == nil {
		return Response{}, errors.New("build runner executable and diagnostics writer are required")
	}
	payload, err := EncodeRequest(request)
	if err != nil {
		return Response{}, err
	}
	stdout := &limitedBuffer{maximum: MaxResponseSize}
	cmd := c.command(ctx, c.executable)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = stdout
	cmd.Stderr = c.diagnostics
	cmd.WaitDelay = 5 * time.Second
	if runtime.GOOS != "windows" {
		cmd.Cancel = func() error {
			err := cmd.Process.Signal(os.Interrupt)
			if errors.Is(err, os.ErrProcessDone) {
				return os.ErrProcessDone
			}
			return err
		}
	}
	if err := cmd.Run(); err != nil {
		if cause := context.Cause(ctx); cause != nil {
			return Response{}, cause
		}
		if stdout.exceeded {
			return Response{}, fmt.Errorf("build runner response exceeds %d-byte limit", MaxResponseSize)
		}
		return Response{}, fmt.Errorf("build runner failed: %w", err)
	}
	response, err := DecodeResponse(bytes.NewReader(stdout.Bytes()))
	if err != nil {
		return Response{}, err
	}
	if response.Stage != request.Stage {
		return Response{}, fmt.Errorf("build runner returned %q response for %q request", response.Stage, request.Stage)
	}
	return response, nil
}

type limitedBuffer struct {
	bytes.Buffer
	maximum  int64
	exceeded bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	remaining := b.maximum - int64(b.Len())
	if remaining <= 0 || int64(len(data)) > remaining {
		b.exceeded = true
		return 0, errors.New("response size limit exceeded")
	}
	return b.Buffer.Write(data)
}
