package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	"github.com/spf13/cobra"
)

func TestExecuteClosesProviders(t *testing.T) {
	t.Parallel()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name    string
		ctx     context.Context
		run     func(*cobra.Command, []string) error
		wantErr bool
	}{
		{name: "success", ctx: context.Background(), run: func(*cobra.Command, []string) error { return nil }},
		{name: "error", ctx: context.Background(), run: func(*cobra.Command, []string) error { return errors.New("boom") }, wantErr: true},
		{name: "cancel", ctx: cancelled, run: func(cmd *cobra.Command, _ []string) error { return cmd.Context().Err() }, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var moduleClosed, hookClosed bool
			modules := platform.NewModuleRegistry()
			modules.RegisterCloser(func() { moduleClosed = true })
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			cmd := newCommandWithHooks(stdout, stderr, modules, Hooks{Close: func() { hookClosed = true }})
			cmd.RunE = tc.run
			cmd.SetArgs([]string{})
			err := executeContext(tc.ctx, cmd)
			if tc.wantErr && err == nil {
				t.Fatal("expected an error")
			}
			if !tc.wantErr && err != nil {
				t.Fatal(err)
			}
			if !moduleClosed || !hookClosed {
				t.Fatalf("moduleClosed=%v hookClosed=%v", moduleClosed, hookClosed)
			}
		})
	}
}
