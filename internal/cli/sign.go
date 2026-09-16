package cli

import (
	"context"
	"errors"
	"os/exec"
	"strings"

	"github.com/magelift/magelift/internal/cosign"
	"github.com/magelift/magelift/internal/oidcidentity"
	"github.com/magelift/magelift/internal/usererr"
	"github.com/spf13/cobra"
)

func signCommand(o *options) *cobra.Command {
	var digest, tokenFile string
	command := &cobra.Command{Use: "sign", Short: "Sign a pushed image digest using the current cloud login", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if digest == "" {
			return invalid(errors.New("--digest is required"))
		}
		if err := o.signDigest(cmd.Context(), digest, tokenFile); err != nil {
			if errors.Is(err, cosign.ErrInvalidReference) || errors.Is(err, cosign.ErrInvalidBlobPath) || errors.Is(err, oidcidentity.ErrInvalidSource) || errors.Is(err, oidcidentity.ErrInvalidToken) {
				return invalid(err)
			}
			var guided *usererr.Error
			if errors.As(err, &guided) {
				return invalid(err)
			}
			return &exitError{code: 3, err: err}
		}
		return nil
	}}
	command.Flags().StringVar(&digest, "digest", "", "registry digest reference")
	command.Flags().StringVar(&tokenFile, "identity-token-file", "", "advanced: identity token file; omit to use the current gcloud or CI login")
	return command
}

func (o *options) signDigest(ctx context.Context, reference, tokenFile string) error {
	if err := cosign.ValidateReference(reference); err != nil {
		return err
	}
	options, cleanup, err := o.identitySignOptions(ctx, tokenFile)
	if err != nil {
		return err
	}
	defer cleanup()
	if o != nil && o.signRelease != nil {
		return o.signRelease(ctx, reference, options)
	}
	return cosign.New().SignWithOptions(ctx, reference, options)
}

func getenvOrEmpty(o *options) func(string) string {
	if o == nil || o.getenv == nil {
		return func(string) string { return "" }
	}
	return o.getenv
}

func (o *options) lookPathOrMissing() func(string) (string, error) {
	if o != nil && o.lookPath != nil {
		return o.lookPath
	}
	return exec.LookPath
}

func (o *options) identitySource(tokenFile string) (oidcidentity.Source, bool, error) {
	if tokenFile != "" {
		return oidcidentity.FileSource{Path: tokenFile}, true, nil
	}
	source, err := oidcidentity.SourceFromEnv(getenvOrEmpty(o))
	if err != nil || source != nil {
		return source, source != nil, err
	}
	cloud := oidcidentity.CloudCLISource(getenvOrEmpty(o), o.lookPathOrMissing(), o.gcpSigningServiceAccount())
	return cloud, false, nil
}

func (o *options) identitySignOptions(ctx context.Context, tokenFile string) (cosign.SignOptions, func(), error) {
	nop := func() {}
	source, explicit, err := o.identitySource(tokenFile)
	if err != nil {
		return cosign.SignOptions{}, nop, err
	}
	if source == nil {
		return cosign.SignOptions{}, nop, nil
	}
	path, cleanup, err := oidcidentity.Materialize(ctx, source)
	if err == nil {
		return cosign.SignOptions{IdentityTokenPath: path}, cleanup, nil
	}
	if explicit {
		return cosign.SignOptions{}, nop, err
	}
	if o.gcpSigningServiceAccount() != "" {
		return cosign.SignOptions{}, nop, usererr.Wrap(err, "cannot sign the image with the current cloud login", "run magelift doctor, then magelift bootstrap --env <environment>, then retry magelift build --push", "docs/getting-started.md")
	}
	return cosign.SignOptions{}, nop, nil
}

func (o *options) discoverSigningIdentity(ctx context.Context) (string, string, error) {
	source, _, err := o.identitySource("")
	if err != nil {
		return "", "", err
	}
	if source == nil {
		return "", "", usererr.New("cannot verify the signed image from the current login", "run magelift doctor, then magelift sign for this digest, then magelift promote --digest", "docs/getting-started.md")
	}
	token, err := source.Token(ctx)
	if err != nil {
		if o.gcpSigningServiceAccount() != "" {
			return "", "", usererr.Wrap(err, "cannot verify the signed image with the current cloud login", "run magelift doctor, then magelift bootstrap --env <environment>, then retry magelift promote --digest", "docs/getting-started.md")
		}
		return "", "", usererr.Wrap(err, "cannot verify the signed image from the current login", "run magelift doctor, then magelift sign for this digest, then magelift promote --digest", "docs/getting-started.md")
	}
	defer clear(token)
	claims, err := oidcidentity.Claims(token)
	if err != nil {
		return "", "", err
	}
	return claims.Subject, claims.Issuer, nil
}

func (o *options) gcpSigningServiceAccount() string {
	if o == nil {
		return ""
	}
	file, err := o.load()
	if err != nil {
		return ""
	}
	environment := o.environment
	if environment == "" {
		environment = getenvOrEmpty(o)("MAGELIFT_ENV")
	}
	if environment == "" {
		if names := file.Environments(); len(names) == 1 {
			environment = names[0]
		}
	}
	if environment == "" {
		return ""
	}
	effective, _, err := o.resolveEnvironment(file, environment)
	if err != nil {
		return ""
	}
	if !strings.EqualFold(effective.Config.Target.Provider, "gcp") || effective.Config.Target.GCP == nil {
		return ""
	}
	return gcpCIServiceAccountEmail(effective.Config.Project.Name, environment, effective.Config.Target.GCP.Project)
}

// gcpCIServiceAccountEmail matches providers/gcp/bootstrap.BuildIdentityPlan.
func gcpCIServiceAccountEmail(project, environment, gcpProject string) string {
	project = strings.TrimSpace(project)
	environment = strings.TrimSpace(environment)
	gcpProject = strings.TrimSpace(gcpProject)
	if project == "" || environment == "" || gcpProject == "" {
		return ""
	}
	accountID := "ml-" + project + "-" + environment + "-ci"
	if len(accountID) > 30 {
		return ""
	}
	return accountID + "@" + gcpProject + ".iam.gserviceaccount.com"
}
