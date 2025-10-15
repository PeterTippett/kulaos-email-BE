package datastore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
)

const DefaultNamespace = ""

type TenantStore struct {
	client *datastore.Client
}

func NewTenantStore(ctx context.Context, projectID string) (*TenantStore, error) {
	client, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create datastore client: %w", err)
	}
	return &TenantStore{client: client}, nil
}

type Tenant struct {
	OrgID           string    `datastore:"org_id"`
	Namespace       string    `datastore:"namespace"`
	BigQueryDataset string    `datastore:"bigquery_dataset"`
	CreatedAt       time.Time `datastore:"created_at"`
	UpdatedAt       time.Time `datastore:"updated_at"`
}

func (t *TenantStore) CreateTenant(ctx context.Context, orgID string) (*Tenant, error) {
	// Normalize key to the base org ID (before any colon suffixes)
	baseOrgID := strings.Split(orgID, ":")[0]

	// Check if tenant already exists
	existing, err := t.GetTenant(ctx, baseOrgID)
	if err == nil {
		return existing, nil
	}

	// Extract the org ID part (before the colon if present)
	namespace := baseOrgID

	tenant := &Tenant{
		OrgID:           orgID,
		Namespace:       namespace,
		BigQueryDataset: sanitizeDatasetName(orgID),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	key := datastore.NameKey("Tenant", baseOrgID, nil)
	key.Namespace = DefaultNamespace

	_, err = t.client.Put(ctx, key, tenant)
	if err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	return tenant, nil
}

func (t *TenantStore) GetTenant(ctx context.Context, orgID string) (*Tenant, error) {
	// Normalize key to the base org ID (before any colon suffixes)
	baseOrgID := strings.Split(orgID, ":")[0]
	key := datastore.NameKey("Tenant", baseOrgID, nil)
	key.Namespace = DefaultNamespace

	var tenant Tenant
	if err := t.client.Get(ctx, key, &tenant); err != nil {
		return nil, err
	}
	return &tenant, nil
}

func (t *TenantStore) UpdateTenant(ctx context.Context, tenant *Tenant) error {
	key := datastore.NameKey("Tenant", tenant.OrgID, nil)
	key.Namespace = DefaultNamespace

	tenant.UpdatedAt = time.Now()

	_, err := t.client.Put(ctx, key, tenant)
	return err
}

func (t *TenantStore) Close() error {
	return t.client.Close()
}

// sanitizeDatasetName converts org ID to a valid BigQuery dataset name
// Dataset names must contain only letters, numbers, and underscores
func sanitizeDatasetName(orgID string) string {
	// Extract the org ID part (before the colon if present)
	orgPart := strings.Split(orgID, ":")[0]

	// Replace hyphens and colons with underscores and make lowercase
	name := strings.ReplaceAll(orgPart, "-", "_")
	name = strings.ReplaceAll(name, ":", "_")
	name = strings.ToLower(name)
	return name
}
