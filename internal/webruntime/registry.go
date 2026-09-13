package webruntime

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// DefaultID is the first-party Adobe-supported Magento HTTP frontend.
const DefaultID = string(sdk.WebRuntimeNginxFPM)

// Registry holds in-process WebRuntime plugins. It never searches the working
// directory or executes unsigned files.
type Registry struct {
	mu   sync.RWMutex
	byID map[sdk.WebRuntimeID]sdk.WebRuntime
}

func NewRegistry() *Registry {
	return &Registry{byID: make(map[sdk.WebRuntimeID]sdk.WebRuntime)}
}

// NormalizeID resolves the empty YAML value to nginx-fpm.
func NormalizeID(id string) string {
	if strings.TrimSpace(id) == "" {
		return DefaultID
	}
	return strings.TrimSpace(id)
}

func (r *Registry) Register(runtime sdk.WebRuntime) error {
	if r == nil {
		return errors.New("web-runtime registry is required")
	}
	if runtime == nil {
		return errors.New("web-runtime plugin is required")
	}
	descriptor := runtime.Descriptor()
	if err := sdk.ValidateWebRuntimeDescriptor(descriptor); err != nil {
		return fmt.Errorf("register web-runtime: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byID == nil {
		r.byID = make(map[sdk.WebRuntimeID]sdk.WebRuntime)
	}
	if _, exists := r.byID[descriptor.ID]; exists {
		return fmt.Errorf("web-runtime plugin %q is already registered", descriptor.ID)
	}
	r.byID[descriptor.ID] = runtime
	return nil
}

func (r *Registry) Get(id string) (sdk.WebRuntime, error) {
	if r == nil {
		return nil, errors.New("web-runtime registry is required")
	}
	normalized := sdk.WebRuntimeID(NormalizeID(id))
	r.mu.RLock()
	runtime, found := r.byID[normalized]
	r.mu.RUnlock()
	if !found {
		return nil, fmt.Errorf("web-runtime plugin %q is not registered; install or register that plugin before plan or mutate", normalized)
	}
	return runtime, nil
}

func (r *Registry) List() []sdk.WebRuntimeDescriptor {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	descriptors := make([]sdk.WebRuntimeDescriptor, 0, len(r.byID))
	for _, runtime := range r.byID {
		descriptors = append(descriptors, runtime.Descriptor())
	}
	r.mu.RUnlock()
	return sdk.WebRuntimeDescriptorsByID(descriptors)
}

func (r *Registry) Admit(webRuntimeID, magentoVersion string, allowUnsupported bool) ([]string, error) {
	runtime, err := r.Get(webRuntimeID)
	if err != nil {
		return nil, err
	}
	return runtime.Admit(magentoVersion, allowUnsupported)
}

// LoadFromPath always fails. MageLift registers web runtimes in-process and
// never executes an unsigned file from the working directory.
func (r *Registry) LoadFromPath(path string) error {
	return fmt.Errorf("refusing to execute unsigned web-runtime file %q from the working directory; register plugins in-process or load a signed HashiCorp go-plugin", path)
}

// RegisterDefaults compiles in the first-party HTTP plugins. frankenphp-worker
// stays unregistered until a Magento storefront worker contract exists.
func RegisterDefaults(r *Registry) error {
	if r == nil {
		return errors.New("web-runtime registry is required")
	}
	for _, runtime := range []sdk.WebRuntime{nginxFPM(), frankenPHPClassic(), phpApache()} {
		if err := r.Register(runtime); err != nil {
			return err
		}
	}
	return nil
}
