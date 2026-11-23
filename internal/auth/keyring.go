package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

// KeyringStore stores secrets in the native OS keychain via go-keyring.
type KeyringStore struct {
	service string
}

// NewKeyringStore creates a KeyringStore bound to serviceName.
func NewKeyringStore(ctx context.Context, serviceName string) *KeyringStore {
	_ = ctx
	service := strings.TrimSpace(serviceName)
	if service == "" {
		service = ServiceName
	}
	return &KeyringStore{service: service}
}

// Set saves secret in the OS keyring under key.
func (k *KeyringStore) Set(ctx context.Context, key, secret string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("keyring set cancelled: %w", err)
	}
	if err := keyring.Set(k.service, key, secret); err != nil {
		return "", fmt.Errorf("save secret in os keyring: %w", err)
	}
	return BackendKeyring, nil
}

// Get fetches the secret stored under key from the OS keyring.
func (k *KeyringStore) Get(ctx context.Context, key string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", fmt.Errorf("keyring get cancelled: %w", err)
	}
	secret, err := keyring.Get(k.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", "", ErrSecretNotFound
		}
		return "", "", fmt.Errorf("read secret from os keyring: %w", err)
	}
	return secret, BackendKeyring, nil
}

// Delete removes key from the OS keyring.
func (k *KeyringStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("keyring delete cancelled: %w", err)
	}
	if err := keyring.Delete(k.service, key); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrSecretNotFound
		}
		return fmt.Errorf("delete secret from os keyring: %w", err)
	}
	return nil
}
