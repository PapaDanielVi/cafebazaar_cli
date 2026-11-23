package publish_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/publish"
)

func TestPipelineDryRunAndLiveAPKFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	apkDir := filepath.Join(appDir, "build", "outputs", "apk", "release")
	if err := os.MkdirAll(apkDir, 0o755); err != nil {
		t.Fatalf("mkdir apk dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, android.FileSettingsKTS), []byte(`include(":app")`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	buildFile := filepath.Join(appDir, android.FileBuildKTS)
	buildSrc := `plugins { id("com.android.application") }
android {
  defaultConfig {
    applicationId = "ir.example.app"
    versionCode = 5
    versionName = "1.2.0"
  }
}`
	if err := os.WriteFile(buildFile, []byte(buildSrc), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}
	apkPath := filepath.Join(apkDir, "app-release.apk")
	if err := os.WriteFile(apkPath, []byte("apk-data"), 0o644); err != nil {
		t.Fatalf("write apk: %v", err)
	}

	var committed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastPublished:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"success","release":{"packages":[{"version_code":8,"version_name":"1.2.0"}]}}`))
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastCommitted:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"not-exists"}`))
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastUncommitted:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"not-exists"}`))
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointReleases:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"type":"success","release":{"packages":[]}}`))
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointUploadAPK:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"success","package":{"version_code":9,"version_name":"1.2.1"}}`))
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointCommit:
			committed = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"type":"success","release":{"id":99,"committed":true,"staged_rollout_percentage":100}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	proj, err := android.DetectProject(ctx, root)
	if err != nil {
		t.Fatalf("detect project: %v", err)
	}
	mod, _ := proj.PrimaryModule(ctx, "")
	variant, err := mod.FindVariant(ctx, "release")
	if err != nil {
		t.Fatalf("find release variant: %v", err)
	}

	client, err := pishkhan.New(ctx, pishkhan.Config{BaseURL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	var outBuf, errBuf bytes.Buffer
	printer := output.NewPrinter(ctx, strings.NewReader(""), &outBuf, &errBuf, output.RuntimeMode{
		Interactive: false,
		Yes:         true,
		Format:      output.FormatPlain,
	})

	// Verify dry-run does not modify Gradle or commit remotely.
	dryRes, err := publish.Run(ctx, publish.Options{
		Project:        proj,
		Module:         mod,
		Variant:        variant,
		PackageName:    variant.PackageName,
		PishkhanClient: client,
		Printer:        printer,
		DryRun:         true,
		BumpKind:       android.BumpPatch,
		PollInterval:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if dryRes.BumpPlan == nil || dryRes.BumpPlan.NewVersionCode != 9 || dryRes.BumpPlan.NewVersionName != "1.2.1" {
		t.Fatalf("unexpected dry-run bump plan: %+v", dryRes.BumpPlan)
	}
	if committed {
		t.Fatal("dry-run must not commit remotely")
	}

	// Verify live run bumps versionCode above remote (8 -> 9) and commits release.
	liveRes, err := publish.Run(ctx, publish.Options{
		Project:        proj,
		Module:         mod,
		Variant:        variant,
		PackageName:    variant.PackageName,
		PishkhanClient: client,
		Printer:        printer,
		DryRun:         false,
		BumpKind:       android.BumpPatch,
		ChangelogFA:    "بهبود عملکرد",
		ChangelogEN:    "Performance improvements",
		PollInterval:   time.Millisecond,
	})
	if err != nil {
		t.Fatalf("live publish failed: %v", err)
	}
	if !liveRes.Committed || !committed {
		t.Fatal("expected release to be committed")
	}
}
