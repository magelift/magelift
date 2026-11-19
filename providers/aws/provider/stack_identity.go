package awsprovider

import (
	"errors"
	"fmt"
	"github.com/magelift/magelift/sdk"
	"regexp"
)

var (
	stableName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	accountID  = regexp.MustCompile(`^[0-9]{12}$`)
	regionName = regexp.MustCompile(`^[a-z]{2}(?:-gov)?-[a-z]+-[0-9]+$`)
)

// StackIdentity names the deployment and the environment that owns its state bucket.
type StackIdentity struct {
	StateEnvironment string
	Project          string
	Environment      string
	AccountID        string
	Region           string
	EnvironmentClass string
	Preset           sdk.PresetID
	Tags             map[string]string
}

func (i StackIdentity) BootstrapEnvironment() string {
	if i.StateEnvironment != "" {
		return i.StateEnvironment
	}
	return i.Environment
}

func (i StackIdentity) Validate() error {
	var problems []error
	if !stableName.MatchString(i.Project) || !stableName.MatchString(i.Environment) || !stableName.MatchString(i.BootstrapEnvironment()) {
		problems = append(problems, errors.New("stack project and environment must be lowercase stable names"))
	}
	if !accountID.MatchString(i.AccountID) {
		problems = append(problems, errors.New("stack account ID must contain 12 digits"))
	}
	if !regionName.MatchString(i.Region) {
		problems = append(problems, errors.New("stack region is invalid"))
	}
	if i.EnvironmentClass != "preview" && i.EnvironmentClass != "production" && i.EnvironmentClass != "staging" {
		problems = append(problems, fmt.Errorf("unsupported environment class %q", i.EnvironmentClass))
	}
	if i.Preset != sdk.PresetPreview && i.Preset != sdk.PresetStandard && i.Preset != sdk.PresetHighAvailability {
		problems = append(problems, fmt.Errorf("unsupported stack preset %q", i.Preset))
	}
	return errors.Join(problems...)
}
