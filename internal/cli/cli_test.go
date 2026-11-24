package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/comments"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/cli"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
)

func writeTestResponse(ctx context.Context, t *testing.T, w http.ResponseWriter, body string) {
	_ = ctx
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write test response: %v", err)
	}
}

func setupAndroidFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	apkDir := filepath.Join(appDir, "build", "outputs", "apk", "staging")
	if err := os.MkdirAll(apkDir, 0o755); err != nil {
		t.Fatalf("mkdir apk dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, android.FileSettingsKTS), []byte(`include(":app")`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	gradleSrc := `plugins { id("com.android.application") }
android {
  defaultConfig {
    applicationId = "ir.demo.shop"
    versionCode = 10
    versionName = "1.5.0"
  }
  buildTypes {
    getByName("debug") { applicationIdSuffix = ".debug" }
    create("staging") { applicationIdSuffix = ".staging" }
    release {}
  }
}`
	if err := os.WriteFile(filepath.Join(appDir, android.FileBuildKTS), []byte(gradleSrc), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(apkDir, "app-staging.apk"), []byte("apk-bytes"), 0o644); err != nil {
		t.Fatalf("write sample apk: %v", err)
	}
	return root
}

func TestDynamicCompletionSuggestsGradleBuildTypes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	projectDir := setupAndroidFixture(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	store := auth.NewFileStore(ctx, filepath.Join(t.TempDir(), "credentials.yaml"))

	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewRootCmd(ctx, cli.Dependencies{
		In:          strings.NewReader(""),
		Out:         &outBuf,
		Err:         &errBuf,
		WorkDir:     projectDir,
		SecretStore: store,
		LookupEnv: func(key string) (string, bool) {
			if key == config.EnvConfig {
				return cfgPath, true
			}
			return "", false
		},
	})
	cmd.SetArgs([]string{"__complete", "release", "status", "--app", ""})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("execute __complete: %v", err)
	}
	out := outBuf.String()
	for _, wantPkg := range []string{"ir.demo.shop", "ir.demo.shop.debug", "ir.demo.shop.staging"} {
		if !strings.Contains(out, wantPkg) {
			t.Fatalf("expected completion output to suggest %q, got:\n%s", wantPkg, out)
		}
	}
}

func TestInitExistingAndAutoFolderAuthAndDangerGuard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	projectDir := setupAndroidFixture(t)
	nestedDir := filepath.Join(projectDir, "app")
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	store := auth.NewFileStore(ctx, filepath.Join(cfgDir, "credentials.yaml"))

	var deleted bool
	pishkhanSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastPublished:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[{"version_code":10,"version_name":"1.5.0"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastCommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastUncommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
		case r.Method == http.MethodDelete && r.URL.Path == pishkhan.EndpointReleases:
			deleted = true
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"Release successfully deleted."}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer pishkhanSrv.Close()

	commentsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == comments.EndpointReviews {
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"next":null,"previous":null,"results":[{"id":77,"rate":5,"text":"اپلیکیشن بسیار عالی است"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer commentsSrv.Close()

	runCLI := func(workDir string, args ...string) (string, error) {
		var outBuf, errBuf bytes.Buffer
		cmd := cli.NewRootCmd(ctx, cli.Dependencies{
			In:              strings.NewReader(""),
			Out:             &outBuf,
			Err:             &errBuf,
			WorkDir:         workDir,
			SecretStore:     store,
			PishkhanBaseURL: pishkhanSrv.URL,
			CommentsBaseURL: commentsSrv.URL,
			LookupEnv: func(key string) (string, bool) {
				switch key {
				case config.EnvConfig:
					return cfgPath, true
				case config.EnvCI:
					return "1", true
				default:
					return "", false
				}
			},
		})
		cmd.SetArgs(args)
		err := cmd.ExecuteContext(ctx)
		return outBuf.String(), err
	}

	// 1. Initialize app in project folder with existing dashboard token.
	if _, err := runCLI(projectDir, "init", "--mode", "existing", "--package", "ir.demo.shop", "--token", "secret-shop-token", "--plain"); err != nil {
		t.Fatalf("bazaar init failed: %v", err)
	}

	// 2. Navigate to nested folder and fetch Persian reviews in JSON without specifying --app or token.
	revJSON, err := runCLI(nestedDir, "review", "list", "--output", "json")
	if err != nil {
		t.Fatalf("bazaar review list failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(revJSON), &parsed); err != nil {
		t.Fatalf("parse review json: %v (%s)", err, revJSON)
	}
	if !strings.Contains(revJSON, "اپلیکیشن بسیار عالی است") {
		t.Fatalf("expected raw UTF-8 Persian text in JSON, got %s", revJSON)
	}

	// 3. Dangerous command without --yes in CI mode must be refused with ExitConfirmation.
	_, err = runCLI(nestedDir, "release", "delete")
	if err == nil {
		t.Fatal("expected release delete without --yes in CI to fail")
	}
	if code := cli.ExitCode(ctx, err); code != output.ExitConfirmation {
		t.Fatalf("got exit code %d, want %d (%v)", code, output.ExitConfirmation, err)
	}
	if deleted {
		t.Fatal("release delete should not have executed without confirmation")
	}

	// 4. Dangerous command WITH --yes succeeds.
	if _, err := runCLI(nestedDir, "release", "delete", "--yes"); err != nil {
		t.Fatalf("release delete --yes failed: %v", err)
	}
	if !deleted {
		t.Fatal("expected release delete to execute with --yes")
	}
}

func TestInitNewAppCapturesReturnedAppToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	projectDir := setupAndroidFixture(t)
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.yaml")
	store := auth.NewFileStore(ctx, filepath.Join(cfgDir, "credentials.yaml"))

	pubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == publisher.EndpointApps {
			w.WriteHeader(http.StatusCreated)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"App created.","package_name":"ir.demo.shop","api_agent_token":"issued-app-token-777"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer pubSrv.Close()

	runCLI := func(args ...string) error {
		var outBuf, errBuf bytes.Buffer
		cmd := cli.NewRootCmd(ctx, cli.Dependencies{
			In:               strings.NewReader(""),
			Out:              &outBuf,
			Err:              &errBuf,
			WorkDir:          projectDir,
			SecretStore:      store,
			PublisherBaseURL: pubSrv.URL,
			LookupEnv: func(key string) (string, bool) {
				switch key {
				case config.EnvConfig:
					return cfgPath, true
				case config.EnvPublisherToken:
					return "dev-nasher-token", true
				case config.EnvCI:
					return "1", true
				default:
					return "", false
				}
			},
		})
		cmd.SetArgs(args)
		return cmd.ExecuteContext(ctx)
	}

	if err := runCLI("init", "--mode", "new", "--package", "ir.demo.shop", "--yes", "--plain"); err != nil {
		t.Fatalf("bazaar init --mode new failed: %v", err)
	}

	savedTok, _, err := store.Get(ctx, auth.AppKey(ctx, "ir.demo.shop"))
	if err != nil {
		t.Fatalf("expected issued app token to be saved in store: %v", err)
	}
	if savedTok != "issued-app-token-777" {
		t.Fatalf("got saved token %q, want issued-app-token-777", savedTok)
	}
}

func TestCLIProxyFlagRoutesRequestsThroughProxy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var hitProxy bool
	var gotAppName string
	var gotVersion string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitProxy = true
		gotAppName = r.Header.Get(httpclient.HeaderAppName)
		gotVersion = r.Header.Get(httpclient.HeaderVersion)
		w.WriteHeader(http.StatusOK)
		writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[{"version_code":21,"version_name":"2.1.0"}]}}`)
	}))
	defer proxySrv.Close()

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	var outBuf, errBuf bytes.Buffer
	cmd := cli.NewRootCmd(ctx, cli.Dependencies{
		In:              strings.NewReader(""),
		Out:             &outBuf,
		Err:             &errBuf,
		WorkDir:         t.TempDir(),
		PishkhanBaseURL: "http://api.pishkhan.cafebazaar.ir",
		LookupEnv: func(key string) (string, bool) {
			switch key {
			case config.EnvConfig:
				return cfgPath, true
			case config.EnvAppToken:
				return "ci-app-token", true
			case config.EnvCI:
				return "1", true
			default:
				return "", false
			}
		},
	})
	cmd.SetArgs([]string{"release", "status", "--only", "published", "--proxy", proxySrv.URL, "--output", "json"})

	if err := cmd.ExecuteContext(ctx); err != nil {
		t.Fatalf("release status via --proxy failed: %v", err)
	}
	if !hitProxy {
		t.Fatal("expected CLI command to route through proxy server")
	}
	if gotAppName != httpclient.DefaultClientAppName {
		t.Fatalf("got app_name header %q, want %q", gotAppName, httpclient.DefaultClientAppName)
	}
	if gotVersion != cli.Version {
		t.Fatalf("got version header %q, want %q", gotVersion, cli.Version)
	}
	if !strings.Contains(outBuf.String(), `"version_code": 21`) {
		t.Fatalf("unexpected proxied output: %s", outBuf.String())
	}
}
