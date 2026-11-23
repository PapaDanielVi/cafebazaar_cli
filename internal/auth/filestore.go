package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	credentialsDirPerm  = 0o700
	credentialsFilePerm = 0o600
	tempFileSuffix      = ".tmp"
)

type credentialsFile struct {
	Secrets map[string]string `yaml:"secrets"`
}

// FileStore persists tokens in a 0600 YAML file when the OS keychain is unavailable.
type FileStore struct {
	mu   sync.Mutex
	path string
}

// NewFileStore creates a file-backed SecretStore at filePath.
func NewFileStore(ctx context.Context, filePath string) *FileStore {
	_ = ctx
	return &FileStore{path: filePath}
}

// Set writes or updates key in the 0600 credentials file atomically.
func (f *FileStore) Set(ctx context.Context, key, secret string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("file store set cancelled: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := f.loadLocked(ctx)
	if err != nil {
		return "", err
	}
	data.Secrets[key] = secret
	if err := f.saveLocked(ctx, data); err != nil {
		return "", err
	}
	return BackendFile, nil
}

// Get retrieves key from the 0600 credentials file.
func (f *FileStore) Get(ctx context.Context, key string) (string, string, error) {
	if err := ctx.Err(); err != nil {
		return "", "", fmt.Errorf("file store get cancelled: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := f.loadLocked(ctx)
	if err != nil {
		return "", "", err
	}
	secret, ok := data.Secrets[key]
	if !ok || secret == "" {
		return "", "", ErrSecretNotFound
	}
	return secret, BackendFile, nil
}

// Delete removes key from the 0600 credentials file.
func (f *FileStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("file store delete cancelled: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := f.loadLocked(ctx)
	if err != nil {
		return err
	}
	if _, ok := data.Secrets[key]; !ok {
		return ErrSecretNotFound
	}
	delete(data.Secrets, key)
	return f.saveLocked(ctx, data)
}

func (f *FileStore) loadLocked(ctx context.Context) (credentialsFile, error) {
	_ = ctx
	raw, err := os.ReadFile(f.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return credentialsFile{Secrets: map[string]string{}}, nil
		}
		return credentialsFile{}, fmt.Errorf("read credentials file %q: %w", f.path, err)
	}

	var parsed credentialsFile
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &parsed); err != nil {
			return credentialsFile{}, fmt.Errorf("parse credentials file %q: %w", f.path, err)
		}
	}
	if parsed.Secrets == nil {
		parsed.Secrets = map[string]string{}
	}
	return parsed, nil
}

func (f *FileStore) saveLocked(ctx context.Context, data credentialsFile) error {
	_ = ctx
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, credentialsDirPerm); err != nil {
		return fmt.Errorf("create credentials directory %q: %w", dir, err)
	}

	encoded, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal credentials yaml: %w", err)
	}

	tmpPath := f.path + tempFileSuffix
	if err := os.WriteFile(tmpPath, encoded, credentialsFilePerm); err != nil {
		return fmt.Errorf("write temporary credentials file: %w", err)
	}
	if err := os.Chmod(tmpPath, credentialsFilePerm); err != nil {
		return fmt.Errorf("chmod temporary credentials file: %w", err)
	}
	if err := os.Rename(tmpPath, f.path); err != nil {
		return fmt.Errorf("replace credentials file: %w", err)
	}
	return nil
}
