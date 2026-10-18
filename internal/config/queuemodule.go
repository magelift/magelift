package config

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AttachComposerLock records the project composer.lock used to prove a named
// Magento SQS/Pub/Sub module is actually locked.
func (f *File) AttachComposerLock(data []byte) {
	if f == nil {
		return
	}
	f.composerLock = append([]byte(nil), data...)
}

func validateQueueModuleLock(c Config, lock []byte) error {
	transport := strings.TrimSpace(c.Application.Magento.QueueTransport)
	if transport != "sqs" && transport != "pubsub" {
		return nil
	}
	module := strings.TrimSpace(c.Application.Magento.QueueModule)
	if module == "" || !strings.Contains(module, "/") {
		return nil
	}
	if !composerLockContainsPackage(lock, module) {
		return fmt.Errorf("invalid configuration: MageLift: composer.lock does not contain Magento module %q required by application.magento.queueModule; SQS/Pub/Sub is not a Magento queueMode", module)
	}
	return nil
}

func composerLockContainsPackage(lock []byte, name string) bool {
	if len(lock) == 0 || strings.TrimSpace(name) == "" {
		return false
	}
	var parsed struct {
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
		PackagesDev []struct {
			Name string `json:"name"`
		} `json:"packages-dev"`
	}
	if json.Unmarshal(lock, &parsed) != nil {
		return false
	}
	for _, pkg := range append(parsed.Packages, parsed.PackagesDev...) {
		if pkg.Name == name {
			return true
		}
	}
	return false
}
