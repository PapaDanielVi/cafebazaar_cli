package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	// ServiceName is the OS keychain namespace used by the bazaar CLI.
	ServiceName = "bazaar-cli"
	// KeyPrefixPublisher prefixes developer Nasher publisher token entries.
	KeyPrefixPublisher = "publisher:"
	// KeyPrefixApp prefixes per-app Pishkhan token entries.
	KeyPrefixApp = "app:"
	// DefaultAccount is the default developer account key for Nasher tokens.
	DefaultAccount = "default"
	// BackendKeyring identifies the native OS keychain backend.
	BackendKeyring = "keyring"
	// BackendFile identifies the 0600 local credentials file backend.
	BackendFile = "file"
	// BackendEnv identifies a secret supplied via environment variable.
	BackendEnv = "env"

	minMaskVisibleLen = 4
	maskPrefix        = "****"
)

var (
	// ErrSecretNotFound indicates that no secret exists for the requested key.
	ErrSecretNotFound = errors.New("secret not found")
	// ErrEmptyKey indicates that an empty secret lookup key was provided.
	ErrEmptyKey = errors.New("secret key must not be empty")
	// ErrEmptySecret indicates that an empty token value was provided.
	ErrEmptySecret = errors.New("secret value must not be empty")
)

// SecretStore persists and retrieves sensitive CafeBazaar API tokens.
type SecretStore interface {
	Set(ctx context.Context, key, secret string) (string, error)
	Get(ctx context.Context, key string) (string, string, error)
	Delete(ctx context.Context, key string) error
}

// PublisherKey builds the secret store key for a developer account publisher token.
func PublisherKey(ctx context.Context, account string) string {
	_ = ctx
	trimmed := strings.TrimSpace(account)
	if trimmed == "" {
		trimmed = DefaultAccount
	}
	return KeyPrefixPublisher + trimmed
}

// AppKey builds the secret store key for an individual Android app package name.
func AppKey(ctx context.Context, packageName string) string {
	_ = ctx
	return KeyPrefixApp + strings.TrimSpace(packageName)
}

// MaskToken obscures a token for safe terminal display while keeping its last four characters.
func MaskToken(ctx context.Context, token string) string {
	_ = ctx
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= minMaskVisibleLen {
		return maskPrefix
	}
	return maskPrefix + string(runes[len(runes)-minMaskVisibleLen:])
}

// FallbackStore attempts the primary keyring store and transparently falls back to a file store.
type FallbackStore struct {
	primary        SecretStore
	fallback       SecretStore
	disablePrimary bool
	onFallback     func(ctx context.Context, reason error)
}

// NewFallbackStore builds a SecretStore that prefers primary and falls back to fallback on backend errors.
func NewFallbackStore(
	ctx context.Context,
	primary SecretStore,
	fallback SecretStore,
	disablePrimary bool,
	onFallback func(ctx context.Context, reason error),
) *FallbackStore {
	_ = ctx
	return &FallbackStore{
		primary:        primary,
		fallback:       fallback,
		disablePrimary: disablePrimary,
		onFallback:     onFallback,
	}
}

// Set stores secret in primary when available, falling back to the 0600 file store otherwise.
func (s *FallbackStore) Set(ctx context.Context, key, secret string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", ErrEmptyKey
	}
	if strings.TrimSpace(secret) == "" {
		return "", ErrEmptySecret
	}

	if !s.disablePrimary && s.primary != nil {
		backend, err := s.primary.Set(ctx, key, secret)
		if err == nil {
			if s.fallback != nil {
				if delErr := s.fallback.Delete(ctx, key); delErr != nil && !errors.Is(delErr, ErrSecretNotFound) {
					return backend, fmt.Errorf("clean stale fallback credential: %w", delErr)
				}
			}
			return backend, nil
		}
		if s.fallback == nil {
			return "", err
		}
		if s.onFallback != nil {
			s.onFallback(ctx, err)
		}
	}

	if s.fallback == nil {
		return "", errors.New("no credential storage backend available")
	}
	return s.fallback.Set(ctx, key, secret)
}

// Get retrieves a secret from primary first, then checks the fallback file store.
func (s *FallbackStore) Get(ctx context.Context, key string) (string, string, error) {
	if strings.TrimSpace(key) == "" {
		return "", "", ErrEmptyKey
	}

	if !s.disablePrimary && s.primary != nil {
		secret, backend, err := s.primary.Get(ctx, key)
		if err == nil {
			return secret, backend, nil
		}
		if !errors.Is(err, ErrSecretNotFound) && s.fallback == nil {
			return "", "", err
		}
	}

	if s.fallback != nil {
		return s.fallback.Get(ctx, key)
	}
	return "", "", ErrSecretNotFound
}

// Delete removes a secret from both primary and fallback stores.
func (s *FallbackStore) Delete(ctx context.Context, key string) error {
	if strings.TrimSpace(key) == "" {
		return ErrEmptyKey
	}

	foundAny := false
	var primaryErr error
	if !s.disablePrimary && s.primary != nil {
		err := s.primary.Delete(ctx, key)
		if err == nil {
			foundAny = true
		} else if !errors.Is(err, ErrSecretNotFound) {
			primaryErr = err
		}
	}

	var fallbackErr error
	if s.fallback != nil {
		err := s.fallback.Delete(ctx, key)
		if err == nil {
			foundAny = true
		} else if !errors.Is(err, ErrSecretNotFound) {
			fallbackErr = err
		}
	}

	if foundAny {
		return fallbackErr
	}
	if primaryErr != nil && s.fallback == nil {
		return primaryErr
	}
	if fallbackErr != nil {
		return fallbackErr
	}
	return ErrSecretNotFound
}
