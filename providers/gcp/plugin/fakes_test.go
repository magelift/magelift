package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/magelift/magelift/internal/cloud/kube"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	gcpops "github.com/magelift/magelift/providers/gcp/ops"
	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
	gcpsecrets "github.com/magelift/magelift/providers/gcp/secrets"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
	"github.com/magelift/magelift/sdk"
)

// memObjects is an in-memory GCS object store for state lock/archive fakes.
type memObjects struct {
	mu      sync.Mutex
	objects map[string]memObject
}

type memObject struct {
	body       []byte
	generation string
}

func newMemObjects() *memObjects {
	return &memObjects{objects: map[string]memObject{}}
}

func (m *memObjects) key(bucket, key string) string { return bucket + "/" + key }

func (m *memObjects) Get(_ context.Context, bucket, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[m.key(bucket, key)]
	if !ok {
		return nil, "", gcpstate.ErrNotLocked
	}
	return append([]byte(nil), object.body...), object.generation, nil
}

func (m *memObjects) PutIfAbsent(_ context.Context, bucket, key string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[m.key(bucket, key)]; ok {
		return "", errors.New("object already exists")
	}
	generation := fmt.Sprintf("%d", len(m.objects)+1)
	m.objects[m.key(bucket, key)] = memObject{body: append([]byte(nil), body...), generation: generation}
	return generation, nil
}

func (m *memObjects) Put(_ context.Context, bucket, key string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	generation := fmt.Sprintf("%d", len(m.objects)+1)
	m.objects[m.key(bucket, key)] = memObject{body: append([]byte(nil), body...), generation: generation}
	return generation, nil
}

func (m *memObjects) Delete(_ context.Context, bucket, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, m.key(bucket, key))
	return nil
}

func (m *memObjects) DeleteGeneration(_ context.Context, bucket, key, generation string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[m.key(bucket, key)]
	if !ok {
		return errors.New("object not found")
	}
	if object.generation != generation {
		return errors.New("generation mismatch")
	}
	delete(m.objects, m.key(bucket, key))
	return nil
}

func (m *memObjects) Copy(_ context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[m.key(srcBucket, srcKey)]
	if !ok {
		return errors.New("object not found")
	}
	m.objects[m.key(dstBucket, dstKey)] = memObject{body: append([]byte(nil), object.body...), generation: fmt.Sprintf("%d", len(m.objects)+1)}
	return nil
}

func (m *memObjects) List(_ context.Context, bucket, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var keys []string
	for key := range m.objects {
		name := strings.TrimPrefix(key, bucket+"/")
		if strings.HasPrefix(name, prefix) {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// memSecrets is an in-memory Secret Manager for secrets fakes.
type memSecrets struct {
	mu      sync.Mutex
	values  map[string][]byte
	deleted []string
}

func newMemSecrets() *memSecrets {
	return &memSecrets{values: map[string][]byte{}}
}

func (m *memSecrets) List(_ context.Context, project string) ([]gcpsecrets.Meta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []gcpsecrets.Meta
	for name := range m.values {
		if strings.HasPrefix(name, "projects/"+project+"/secrets/") {
			out = append(out, gcpsecrets.Meta{Name: name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *memSecrets) Set(_ context.Context, project, name string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values["projects/"+project+"/secrets/"+name] = append([]byte(nil), value...)
	return nil
}

func (m *memSecrets) Remove(_ context.Context, project, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := "projects/" + project + "/secrets/" + name
	m.deleted = append(m.deleted, key)
	delete(m.values, key)
	return nil
}

func (m *memSecrets) GetSecretValue(_ context.Context, name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range m.values {
		if key == name || strings.HasPrefix(name, key+"/versions/") {
			return append([]byte(nil), value...), nil
		}
	}
	return nil, errors.New("secret not found")
}

// memBuckets is an in-memory bucket API for bootstrap fakes.
type memBuckets struct {
	mu      sync.Mutex
	buckets map[string]bool
}

func newMemBuckets() *memBuckets {
	return &memBuckets{buckets: map[string]bool{}}
}

func (m *memBuckets) BucketExists(_ context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buckets[name], nil
}

func (m *memBuckets) CreateBucket(_ context.Context, name, _, _ string, _ map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets[name] = true
	return nil
}

func (m *memBuckets) EnsureVersioning(_ context.Context, _ string) error { return nil }

func (m *memBuckets) EnsureSoftDelete(_ context.Context, _ string, _ time.Duration) error {
	return nil
}

// stubSQL is a scripted CloudSQLAPI for cleanup and leftover tests.
type stubSQL struct {
	mu        sync.Mutex
	instances map[string]gcpresilience.CloudSQLInstance
	backups   []gcpresilience.CloudSQLBackup
	deleted   []string
}

func newStubSQL() *stubSQL {
	return &stubSQL{instances: map[string]gcpresilience.CloudSQLInstance{}}
}

func (s *stubSQL) CreateBackup(context.Context, string, string, string, int64) (gcpresilience.CloudSQLOperation, error) {
	return gcpresilience.CloudSQLOperation{Status: "done"}, nil
}

func (s *stubSQL) DeleteBackup(_ context.Context, name string) (gcpresilience.CloudSQLOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, name)
	kept := s.backups[:0]
	for _, backup := range s.backups {
		if backup.Name != name {
			kept = append(kept, backup)
		}
	}
	s.backups = kept
	return gcpresilience.CloudSQLOperation{Status: "done"}, nil
}

func (s *stubSQL) ListBackups(context.Context, string) ([]gcpresilience.CloudSQLBackup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]gcpresilience.CloudSQLBackup(nil), s.backups...), nil
}

func (s *stubSQL) GetBackup(_ context.Context, name string) (gcpresilience.CloudSQLBackup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, backup := range s.backups {
		if backup.Name == name {
			return backup, nil
		}
	}
	return gcpresilience.CloudSQLBackup{}, errors.New("backup not found")
}

func (s *stubSQL) RestoreBackup(context.Context, string, string, string, gcpresilience.CloudSQLRestoreSettings) (gcpresilience.CloudSQLOperation, error) {
	return gcpresilience.CloudSQLOperation{Status: "done"}, nil
}

func (s *stubSQL) GetInstance(_ context.Context, _, name string) (gcpresilience.CloudSQLInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	instance, ok := s.instances[name]
	if !ok {
		return gcpresilience.CloudSQLInstance{}, errors.New("instance not found")
	}
	return instance, nil
}

func (s *stubSQL) ListInstances(context.Context, string) ([]gcpresilience.CloudSQLInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []gcpresilience.CloudSQLInstance
	for _, instance := range s.instances {
		out = append(out, instance)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *stubSQL) DeleteInstance(_ context.Context, _, name string) (gcpresilience.CloudSQLOperation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, name)
	delete(s.instances, name)
	return gcpresilience.CloudSQLOperation{Status: "done"}, nil
}

func (s *stubSQL) GetOperation(context.Context, string, string) (gcpresilience.CloudSQLOperation, error) {
	return gcpresilience.CloudSQLOperation{Status: "done"}, nil
}

// stubEdgeAdapter scripts edge proxy responses.
type stubEdgeAdapter struct {
	plan   sdk.EdgePlan
	result sdk.EdgeExecutionResult
	err    error
}

func (s stubEdgeAdapter) EdgeDescriptor() sdk.EdgeAdapterDescriptor {
	return sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.edge", Provider: "gcp", Version: "1.0.0"}
}

func (s stubEdgeAdapter) PlanEdge(context.Context, sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	if s.err != nil {
		return sdk.EdgePlan{}, s.err
	}
	return s.plan, nil
}

func (s stubEdgeAdapter) ExecuteEdge(context.Context, sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	if s.err != nil {
		return sdk.EdgeExecutionResult{}, s.err
	}
	return s.result, nil
}

// stubResilienceAdapter scripts resilience proxy responses.
type stubResilienceAdapter struct {
	plan   sdk.ResiliencePlan
	result sdk.ResilienceExecutionResult
	err    error
}

func (s stubResilienceAdapter) ResilienceDescriptor() sdk.ResilienceAdapterDescriptor {
	return sdk.ResilienceAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.resilience", Provider: "gcp", Version: "1.0.0"}
}

func (s stubResilienceAdapter) PlanResilience(context.Context, sdk.ResiliencePlanRequest) (sdk.ResiliencePlan, error) {
	if s.err != nil {
		return sdk.ResiliencePlan{}, s.err
	}
	return s.plan, nil
}

func (s stubResilienceAdapter) ExecuteResilience(context.Context, sdk.ResilienceExecutionRequest) (sdk.ResilienceExecutionResult, error) {
	if s.err != nil {
		return sdk.ResilienceExecutionResult{}, s.err
	}
	return s.result, nil
}

// fakeKubeFactory returns a ClientFactory serving a fixed clientset.
func fakeKubeFactory(client kubernetes.Interface) kube.ClientFactory {
	return func(map[string]any) (kubernetes.Interface, error) {
		return client, nil
	}
}

// stateServer builds a Server with in-memory state backends.
func stateServer(t *testing.T, objects *memObjects) *Server {
	t.Helper()
	manager, err := gcpstate.NewManager(objects, "shop-staging-state", "shop", "staging")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := gcpstate.NewArchive(objects, "shop-staging-state", "shop", "staging")
	if err != nil {
		t.Fatal(err)
	}
	return &Server{State: gcpops.State{
		NewManager: func(context.Context, string, string, string) (*gcpstate.Manager, error) { return manager, nil },
		NewArchive: func(context.Context, string, string, string) (*gcpstate.Archive, error) { return archive, nil },
	}}
}

// secretsServer builds a Server with an in-memory secret store.
func secretsServer(t *testing.T, secrets *memSecrets) *Server {
	t.Helper()
	return &Server{Secrets: gcpops.Secrets{
		NewStore: func(context.Context) (*gcpsecrets.Store, error) { return gcpsecrets.NewStoreFromClient(secrets), nil },
	}}
}

// bootstrapServer builds a Server with in-memory bootstrap backends.
func bootstrapServer(t *testing.T, buckets *memBuckets) *Server {
	t.Helper()
	ensurer, err := gcpbootstrap.NewFromClient(buckets)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Bootstrap: gcpops.Bootstrap{
		NewEnsurer:      func(context.Context) (*gcpbootstrap.Bootstrapper, error) { return ensurer, nil },
		VerifyAccountFn: func(context.Context, string) error { return nil },
	}}
}

// costServer builds a Server with a scripted budget reader.
func costServer(reader gcpcost.BudgetReader) *Server {
	return &Server{Estimator: gcpcost.Estimator{
		NewBudgetReader: func(context.Context) (gcpcost.BudgetReader, error) { return reader, nil },
	}}
}
