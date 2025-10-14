package encryption

import (
	"context"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"strings"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type KMSService struct {
	client     *kms.KeyManagementClient
	projectID  string
	locationID string
	keyRingID  string
}

func NewKMSService(ctx context.Context, projectID, locationID, keyRingID string) (*KMSService, error) {
	client, err := kms.NewKeyManagementClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create KMS client: %w", err)
	}

	return &KMSService{
		client:     client,
		projectID:  projectID,
		locationID: locationID,
		keyRingID:  keyRingID,
	}, nil
}

func (k *KMSService) Close() error {
	return k.client.Close()
}

// sanitizeKeyID converts a tenant ID to a valid KMS key ID
// KMS key IDs must match the pattern ([a-zA-Z0-9_-]{1,63})
func sanitizeKeyID(tenantID string) string {
	// Extract the org ID part (before the colon if present)
	// e.g., "org_3eca4414e36:2ed39364-da0b-46bc-9103-3d0be68dfe4d" -> "org_3eca4414e36"
	parts := strings.Split(tenantID, ":")
	sanitized := parts[0]

	// Ensure it's not too long (max 63 chars for the full key ID)
	if len(sanitized) > 40 {
		sanitized = sanitized[:40]
	}
	return sanitized
}

// EnsureTenantKey creates a KMS key for a tenant if it doesn't exist
func (k *KMSService) EnsureTenantKey(ctx context.Context, tenantID string) (string, error) {
	sanitizedID := sanitizeKeyID(tenantID)
	keyID := fmt.Sprintf("%s-encryption", sanitizedID)
	keyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		k.projectID, k.locationID, k.keyRingID, keyID)

	// Try to get existing key
	_, err := k.client.GetCryptoKey(ctx, &kmspb.GetCryptoKeyRequest{
		Name: keyName,
	})

	if err != nil {
		// Key doesn't exist, create it
		parent := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s",
			k.projectID, k.locationID, k.keyRingID)

		_, err = k.client.CreateCryptoKey(ctx, &kmspb.CreateCryptoKeyRequest{
			Parent:      parent,
			CryptoKeyId: keyID,
			CryptoKey: &kmspb.CryptoKey{
				Purpose: kmspb.CryptoKey_ENCRYPT_DECRYPT,
				VersionTemplate: &kmspb.CryptoKeyVersionTemplate{
					Algorithm: kmspb.CryptoKeyVersion_GOOGLE_SYMMETRIC_ENCRYPTION,
				},
			},
		})

		if err != nil {
			return "", fmt.Errorf("failed to create key: %w", err)
		}
	}

	return keyName, nil
}

// Encrypt encrypts plaintext using the tenant's KMS key
func (k *KMSService) Encrypt(ctx context.Context, tenantID, plaintext string) (string, error) {
	keyName, err := k.EnsureTenantKey(ctx, tenantID)
	if err != nil {
		return "", err
	}

	plaintextBytes := []byte(plaintext)
	crc32c := crc32.Checksum(plaintextBytes, crc32.MakeTable(crc32.Castagnoli))

	result, err := k.client.Encrypt(ctx, &kmspb.EncryptRequest{
		Name:            keyName,
		Plaintext:       plaintextBytes,
		PlaintextCrc32C: wrapperspb.Int64(int64(crc32c)),
	})

	if err != nil {
		return "", fmt.Errorf("failed to encrypt: %w", err)
	}

	if !result.VerifiedPlaintextCrc32C {
		return "", fmt.Errorf("encrypt request corrupted")
	}

	ciphertextCRC32C := crc32.Checksum(result.Ciphertext, crc32.MakeTable(crc32.Castagnoli))
	if int64(ciphertextCRC32C) != result.CiphertextCrc32C.Value {
		return "", fmt.Errorf("encrypt response corrupted")
	}

	return base64.StdEncoding.EncodeToString(result.Ciphertext), nil
}

// Decrypt decrypts ciphertext using the tenant's KMS key
func (k *KMSService) Decrypt(ctx context.Context, tenantID, ciphertext string) (string, error) {
	keyName, err := k.EnsureTenantKey(ctx, tenantID)
	if err != nil {
		return "", err
	}

	ciphertextBytes, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}

	crc32c := crc32.Checksum(ciphertextBytes, crc32.MakeTable(crc32.Castagnoli))

	result, err := k.client.Decrypt(ctx, &kmspb.DecryptRequest{
		Name:             keyName,
		Ciphertext:       ciphertextBytes,
		CiphertextCrc32C: wrapperspb.Int64(int64(crc32c)),
	})

	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	plaintextCRC32C := crc32.Checksum(result.Plaintext, crc32.MakeTable(crc32.Castagnoli))
	if int64(plaintextCRC32C) != result.PlaintextCrc32C.Value {
		return "", fmt.Errorf("decrypt response corrupted")
	}

	return string(result.Plaintext), nil
}
