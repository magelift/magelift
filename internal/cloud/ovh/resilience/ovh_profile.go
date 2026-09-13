package resilience

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ovh/go-ovh/ovh"
	"gopkg.in/ini.v1"
)

// NewOVHClientFromProfile loads an official OVH API client from the shared
// ovh.conf profile layout used by ovhcloud and go-ovh. Credential material
// stays in the local profile file and never enters MageLift operation state.
func NewOVHClientFromProfile(profile string) (*ovh.Client, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = "default"
	}
	if strings.ContainsAny(profile, "\r\n\x00/") {
		return nil, errors.New("OVHcloud profile name contains unsupported characters")
	}
	cfg, err := loadOVHINI()
	if err != nil {
		return nil, fmt.Errorf("load OVHcloud profile configuration: %w", err)
	}
	endpoint := strings.TrimSpace(cfg.Section(profile).Key("endpoint").String())
	if endpoint == "" {
		endpoint = "ovh-eu"
	}
	return ovh.NewEndpointClient(endpoint)
}

func loadOVHINI() (*ini.File, error) {
	if runtime.GOARCH == "wasm" && runtime.GOOS == "js" {
		return ini.Empty(), nil
	}
	paths := make([]interface{}, 0, 3)
	paths = append(paths, "/etc/ovh.conf")
	if home, err := currentUserHome(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".ovh.conf"))
	}
	paths = append(paths, "ovh.conf")
	if len(paths) == 0 {
		return ini.Empty(), nil
	}
	return ini.LooseLoad(paths[0], paths[1:]...)
}

func currentUserHome() (string, error) {
	usr, err := user.Current()
	if err != nil {
		if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
			return home, nil
		}
		return "", err
	}
	return usr.HomeDir, nil
}
