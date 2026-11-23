package auth_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
)

const (
	testPkg       = "com.example.bazaarapp"
	testAppSecret = "secret-app-token-9876"
	testPubSecret = "secret-pub-token-1234"
)

type failingStore struct {
	err error
}

func (f failingStore) Set(ctx context.Context, key, secret string) (string, error) {
	_ = ctx
	_ = key
	_ = secret
	return "", f.err
}

func (f failingStore) Get(ctx context.Context, key string) (string, string, error) {
	_ = ctx
	_ = key
	return "", "", f.err
}

func (f failingStore) Delete(ctx context.Context, key string) error {
	_ = ctx
	_ = key
	return f.err
}

func TestFileStoreLifecycleAndPermissions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	credPath := filepath.Join(t.TempDir(), "nested", "credentials.yaml")
	store := auth.NewFileStore(ctx, credPath)

	key := auth.AppKey(ctx, testPkg)
	backend, err := store.Set(ctx, key, testAppSecret)
	if err != nil {
		t.Fatalf("set secret: %v", err)
	}
	if backend != auth.BackendFile {
		t.Fatalf("got backend %q, want %q", backend, auth.BackendFile)
	}

	info, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("stat credentials file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("got file mode %o, want 600", perm)
	}

	got, gotBackend, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if got != testAppSecret || gotBackend != auth.BackendFile {
		t.Fatalf("got (%q, %q), want (%q, %q)", got, gotBackend, testAppSecret, auth.BackendFile)
	}

	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("delete secret: %v", err)
	}
	if _, _, err := store.Get(ctx, key); !errors.Is(err, auth.ErrSecretNotFound) {
		t.Fatalf("got err=%v after delete, want ErrSecretNotFound", err)
	}
}

func TestFallbackStoreUsesFileWhenKeyringFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	credPath := filepath.Join(t.TempDir(), "credentials.yaml")
	fileStore := auth.NewFileStore(ctx, credPath)
	keyringErr := errors.New("dbus unavailable")

	var fallbackCalled bool
	store := auth.NewFallbackStore(ctx, failingStore{err: keyringErr}, fileStore, false, func(cbCtx context.Context, reason error) {
		_ = cbCtx
		if errors.Is(reason, keyringErr) {
			fallbackCalled = true
		}
	})

	pubKey := auth.PublisherKey(ctx, "")
	backend, err := store.Set(ctx, pubKey, testPubSecret)
	if err != nil {
		t.Fatalf("fallback set: %v", err)
	}
	if !fallbackCalled || backend != auth.BackendFile {
		t.Fatalf("got fallbackCalled=%v backend=%q, want true and file", fallbackCalled, backend)
	}

	got, gotBackend, err := store.Get(ctx, pubKey)
	if err != nil || got != testPubSecret || gotBackend != auth.BackendFile {
		t.Fatalf("fallback get: got=(%q,%q,%v)", got, gotBackend, err)
	}
}

func TestMaskToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "short", input: "abcd", want: "****"},
		{name: "normal", input: "abcdefgh1234", want: "****1234"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := auth.MaskToken(ctx, tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
