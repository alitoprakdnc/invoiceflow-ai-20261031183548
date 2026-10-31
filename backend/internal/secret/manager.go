package secret

import (
	"context"
	"fmt"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"google.golang.org/api/option"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

// Manager defines the contract for secret retrieval.
type Manager interface {
	// GetSecret returns the plain‑text value of the secret identified by `name`.
	// The secret is cached in memory for the duration of the manager's TTL.
	// If the secret cannot be retrieved, an error is returned.
	GetSecret(ctx context.Context, name string) (string, error)

	// Close releases any underlying resources.
	Close() error
}

// cachedSecret holds a secret value together with its expiration time.
type cachedSecret struct {
	value     string
	expiresAt time.Time
}

// secretManager implements the Manager interface using Google Cloud Secret Manager.
type secretManager struct {
	client    *secretmanager.Client
	projectID string
	ttl       time.Duration

	mu    sync.RWMutex
	cache map[string]cachedSecret
}

// New creates a new secret manager.
// `projectID` is the GCP project that holds the secrets.
// `ttl` defines how long a secret stays in the in‑memory cache; if zero, a default of 5 minutes is used.
// Additional client options (e.g., credentials) can be supplied via `opts`.
func New(ctx context.Context, projectID string, ttl time.Duration, opts ...option.ClientOption) (Manager, error) {
	if projectID == "" {
		return nil, fmt.Errorf("projectID must not be empty")
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}

	client, err := secretmanager.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret manager client: %w", err)
	}

	sm := &secretManager{
		client:    client,
		projectID: projectID,
		ttl:       ttl,
		cache:     make(map[string]cachedSecret),
	}
	return sm, nil
}

// GetSecret retrieves the secret value identified by `name`.
// It first checks the in‑memory cache; if the entry is missing or expired,
// it fetches the latest version from Secret Manager, caches it, and returns the value.
func (sm *secretManager) GetSecret(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("secret name must not be empty")
	}

	// Fast path: cached value.
	sm.mu.RLock()
	if cs, ok := sm.cache[name]; ok && time.Now().Before(cs.expiresAt) {
		sm.mu.RUnlock()
		return cs.value, nil
	}
	sm.mu.RUnlock()

	// Build the resource name for the latest version.
	secretPath := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", sm.projectID, name)

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: secretPath,
	}

	// Access the secret version.
	resp, err := sm.client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("unable to access secret %s: %w", name, err)
	}
	if resp == nil || resp.Payload == nil {
		return "", fmt.Errorf("secret %s returned empty payload", name)
	}

	secretValue := string(resp.Payload.Data)

	// Store in cache.
	sm.mu.Lock()
	sm.cache[name] = cachedSecret{
		value:     secretValue,
		expiresAt: time.Now().Add(sm.ttl),
	}
	sm.mu.Unlock()

	return secretValue, nil
}

// Close shuts down the underlying Secret Manager client.
func (sm *secretManager) Close() error {
	if sm.client == nil {
		return nil
	}
	return sm.client.Close()
}