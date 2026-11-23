package config_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
)

const (
	testPkgRelease = "com.example.shop"
	testPkgStaging = "com.example.shop.staging"
	testTokRelease = "tok-release-1111"
	testTokStaging = "tok-staging-2222"
	testTokEnv     = "tok-ci-env-9999"
)

func TestResolveAppPrecedenceAndFolderBinding(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	root := t.TempDir()
	projectDir := filepath.Join(root, "android-shop")
	nestedDir := filepath.Join(projectDir, "app", "src", "main")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("create nested dir: %v", err)
	}

	store := auth.NewFileStore(ctx, filepath.Join(root, "credentials.yaml"))
	if _, err := store.Set(ctx, auth.AppKey(ctx, testPkgRelease), testTokRelease); err != nil {
		t.Fatalf("store release token: %v", err)
	}
	if _, err := store.Set(ctx, auth.AppKey(ctx, testPkgStaging), testTokStaging); err != nil {
		t.Fatalf("store staging token: %v", err)
	}

	cfg := &config.GlobalConfig{
		DefaultApp: testPkgRelease,
		Apps: map[string]config.AppEntry{
			testPkgRelease: {PackageName: testPkgRelease, Alias: "prod"},
			testPkgStaging: {PackageName: testPkgStaging, Alias: "stage"},
		},
	}
	if err := cfg.BindPath(ctx, testPkgStaging, projectDir); err != nil {
		t.Fatalf("bind project dir: %v", err)
	}

	tests := []struct {
		name           string
		input          config.ResolveInput
		wantPkg        string
		wantTok        string
		wantResolvedBy string
		wantErr        error
	}{
		{
			name: "env token overrides everything",
			input: config.ResolveInput{
				Cwd:    nestedDir,
				Global: cfg,
				Store:  store,
				Env:    config.EnvSettings{AppToken: testTokEnv, App: testPkgRelease},
			},
			wantPkg:        testPkgRelease,
			wantTok:        testTokEnv,
			wantResolvedBy: config.ResolutionEnvToken,
		},
		{
			name: "flag alias resolves registered app",
			input: config.ResolveInput{
				FlagApp: "prod",
				Cwd:     nestedDir,
				Global:  cfg,
				Store:   store,
			},
			wantPkg:        testPkgRelease,
			wantTok:        testTokRelease,
			wantResolvedBy: config.ResolutionFlag,
		},
		{
			name: "nested folder resolves bound app automatically",
			input: config.ResolveInput{
				Cwd:    nestedDir,
				Global: cfg,
				Store:  store,
			},
			wantPkg:        testPkgStaging,
			wantTok:        testTokStaging,
			wantResolvedBy: config.ResolutionBoundPath,
		},
		{
			name: "gradle match resolves single matching app",
			input: config.ResolveInput{
				Cwd:              root,
				Global:           cfg,
				Store:            store,
				DetectedPackages: []string{testPkgRelease, "com.example.unregistered"},
			},
			wantPkg:        testPkgRelease,
			wantTok:        testTokRelease,
			wantResolvedBy: config.ResolutionGradleMatch,
		},
		{
			name: "multiple gradle matches returns ambiguous error",
			input: config.ResolveInput{
				Cwd:              root,
				Global:           cfg,
				Store:            store,
				DetectedPackages: []string{testPkgRelease, testPkgStaging},
			},
			wantErr: config.ErrAmbiguousApp,
		},
		{
			name: "project config overrides bound path",
			input: config.ResolveInput{
				Cwd:     nestedDir,
				Global:  cfg,
				Store:   store,
				Project: &config.ProjectConfig{App: testPkgRelease},
			},
			wantPkg:        testPkgRelease,
			wantTok:        testTokRelease,
			wantResolvedBy: config.ResolutionProjectConfig,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := config.ResolveApp(ctx, tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got err=%v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.PackageName != tc.wantPkg || got.Token != tc.wantTok || got.ResolvedBy != tc.wantResolvedBy {
				t.Fatalf("got %+v, want pkg=%q tok=%q by=%q", got, tc.wantPkg, tc.wantTok, tc.wantResolvedBy)
			}
		})
	}
}

func TestReadEnvSettings(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	values := map[string]string{
		config.EnvPublisherToken: "pub-123",
		config.EnvAppToken:       "app-456",
		config.EnvAutoPublish:    "true",
		config.EnvRollout:        "25",
		config.EnvCI:             "1",
		config.EnvYes:            "yes",
		config.EnvRTL:            "OFF",
	}
	env := config.ReadEnv(ctx, func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	})

	if env.PublisherToken != "pub-123" || env.AppToken != "app-456" {
		t.Fatalf("unexpected tokens: %+v", env)
	}
	if env.AutoPublish == nil || !*env.AutoPublish {
		t.Fatalf("expected auto publish true, got %v", env.AutoPublish)
	}
	if env.Rollout != 25 || !env.CI || !env.Yes || env.RTL != "off" {
		t.Fatalf("unexpected env flags: %+v", env)
	}
}
