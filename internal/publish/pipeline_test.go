package publish_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func writeTestResponse(ctx context.Context, t *testing.T, w http.ResponseWriter, body string) {
	_ = ctx
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write test response: %v", err)
	}
}

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
			writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[{"version_code":8,"version_name":"1.2.0"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastCommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastUncommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointReleases:
			w.WriteHeader(http.StatusCreated)
			writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[]}}`)
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointUploadAPK:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","package":{"version_code":9,"version_name":"1.2.1"}}`)
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointCommit:
			committed = true
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","release":{"id":99,"committed":true,"staged_rollout_percentage":100}}`)
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

func TestPipelineStagedRolloutPrompt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name               string
		remotePublishedPct int
		rolloutOpt         int
		rolloutExplicit    bool
		stagedEnabled      bool
		stdin              string
		wantRollout        int
		wantErr            error
		wantPrompted       bool
	}{
		{
			name:               "active staged rollout user confirms 100 percent",
			remotePublishedPct: 20,
			stdin:              "y\ny\n",
			wantRollout:        100,
			wantPrompted:       true,
		},
		{
			name:               "active staged rollout user enters custom percentage",
			remotePublishedPct: 20,
			stdin:              "n\n35\ny\n",
			wantRollout:        35,
			wantPrompted:       true,
		},
		{
			name:               "active staged rollout user enters invalid percentage",
			remotePublishedPct: 20,
			stdin:              "n\n150\n",
			wantErr:            pishkhan.ErrInvalidRollout,
			wantPrompted:       true,
		},
		{
			name:               "explicit rollout flag bypasses prompt",
			remotePublishedPct: 20,
			rolloutOpt:         50,
			rolloutExplicit:    true,
			stdin:              "y\n",
			wantRollout:        50,
			wantPrompted:       false,
		},
		{
			name:               "non-staged app at 100 percent skips rollout prompt",
			remotePublishedPct: 100,
			stdin:              "y\n",
			wantRollout:        100,
			wantPrompted:       false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			apkPath := filepath.Join(t.TempDir(), "app-release.apk")
			if err := os.WriteFile(apkPath, []byte("apk-bytes"), 0o600); err != nil {
				t.Fatalf("write apk: %v", err)
			}

			var gotCommit pishkhan.CommitReleaseRequest
			var committed bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastPublished:
					w.WriteHeader(http.StatusOK)
					body := fmt.Sprintf(`{"type":"success","release":{"packages":[{"version_code":8,"version_name":"1.2.0"}],"staged_rollout_percentage":%d}}`, tc.remotePublishedPct)
					writeTestResponse(ctx, t, w, body)
				case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastCommitted:
					w.WriteHeader(http.StatusOK)
					writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
				case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastUncommitted:
					w.WriteHeader(http.StatusOK)
					writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
				case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointReleases:
					w.WriteHeader(http.StatusCreated)
					writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[]}}`)
				case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointUploadAPK:
					w.WriteHeader(http.StatusOK)
					writeTestResponse(ctx, t, w, `{"type":"success","package":{"version_code":9,"version_name":"1.2.1"}}`)
				case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointCommit:
					if err := json.NewDecoder(r.Body).Decode(&gotCommit); err != nil {
						t.Errorf("decode commit request: %v", err)
					}
					committed = true
					w.WriteHeader(http.StatusOK)
					body := fmt.Sprintf(`{"type":"success","release":{"id":99,"committed":true,"staged_rollout_percentage":%d}}`, gotCommit.StagedRolloutPercentage)
					writeTestResponse(ctx, t, w, body)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			client, err := pishkhan.New(ctx, pishkhan.Config{BaseURL: srv.URL, Token: "tok"})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}

			var outBuf, errBuf bytes.Buffer
			printer := output.NewPrinter(ctx, strings.NewReader(tc.stdin), &outBuf, &errBuf, output.RuntimeMode{
				Interactive: true,
				Yes:         false,
				Format:      output.FormatPlain,
			})

			res, err := publish.Run(ctx, publish.Options{
				PackageName:          "ir.example.app",
				PishkhanClient:       client,
				Printer:              printer,
				APKPath:              apkPath,
				Rollout:              tc.rolloutOpt,
				RolloutExplicit:      tc.rolloutExplicit,
				StagedRolloutEnabled: tc.stagedEnabled,
				AutoPublishExplicit:  true,
				ChangelogFA:          "بهبود عملکرد",
				ChangelogEN:          "Performance improvements",
				PollInterval:         time.Millisecond,
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("got err=%v, want %v", err, tc.wantErr)
				}
				if committed {
					t.Fatal("expected release not to be committed on invalid rollout")
				}
				return
			}
			if err != nil {
				t.Fatalf("publish run failed: %v", err)
			}
			if !res.Committed || !committed {
				t.Fatal("expected release to be committed")
			}
			if gotCommit.StagedRolloutPercentage != tc.wantRollout {
				t.Fatalf("got committed rollout %d, want %d", gotCommit.StagedRolloutPercentage, tc.wantRollout)
			}
			prompted := strings.Contains(errBuf.String(), "Roll out this release to 100% of users?")
			if prompted != tc.wantPrompted {
				t.Fatalf("got prompted=%v, want %v (stderr=%q)", prompted, tc.wantPrompted, errBuf.String())
			}
		})
	}
}

func TestPipelineAutoPublishPrompt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name                string
		autoPublishOpt      bool
		autoPublishExplicit bool
		stdin               string
		wantAutoPublish     bool
		wantPrompted        bool
	}{
		{
			name:            "interactive user enables auto publish",
			stdin:           "y\ny\n",
			wantAutoPublish: true,
			wantPrompted:    true,
		},
		{
			name:            "interactive user keeps auto publish disabled",
			stdin:           "n\ny\n",
			wantAutoPublish: false,
			wantPrompted:    true,
		},
		{
			name:                "explicit auto publish flag skips prompt",
			autoPublishOpt:      true,
			autoPublishExplicit: true,
			stdin:               "y\n",
			wantAutoPublish:     true,
			wantPrompted:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			apkPath := filepath.Join(t.TempDir(), "app-release.apk")
			if err := os.WriteFile(apkPath, []byte("apk-bytes"), 0o600); err != nil {
				t.Fatalf("write apk: %v", err)
			}

			var gotCommit pishkhan.CommitReleaseRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastPublished:
					w.WriteHeader(http.StatusOK)
					writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[{"version_code":8,"version_name":"1.2.0"}],"staged_rollout_percentage":100}}`)
				case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastCommitted:
					w.WriteHeader(http.StatusOK)
					writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
				case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastUncommitted:
					w.WriteHeader(http.StatusOK)
					writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
				case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointReleases:
					w.WriteHeader(http.StatusCreated)
					writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[]}}`)
				case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointUploadAPK:
					w.WriteHeader(http.StatusOK)
					writeTestResponse(ctx, t, w, `{"type":"success","package":{"version_code":9,"version_name":"1.2.1"}}`)
				case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointCommit:
					if err := json.NewDecoder(r.Body).Decode(&gotCommit); err != nil {
						t.Errorf("decode commit request: %v", err)
					}
					w.WriteHeader(http.StatusOK)
					body := fmt.Sprintf(`{"type":"success","release":{"id":99,"committed":true,"staged_rollout_percentage":100,"auto_publish":%v}}`, gotCommit.AutoPublish)
					writeTestResponse(ctx, t, w, body)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			client, err := pishkhan.New(ctx, pishkhan.Config{BaseURL: srv.URL, Token: "tok"})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}

			var outBuf, errBuf bytes.Buffer
			printer := output.NewPrinter(ctx, strings.NewReader(tc.stdin), &outBuf, &errBuf, output.RuntimeMode{
				Interactive: true,
				Yes:         false,
				Format:      output.FormatPlain,
			})

			res, err := publish.Run(ctx, publish.Options{
				PackageName:         "ir.example.app",
				PishkhanClient:      client,
				Printer:             printer,
				APKPath:             apkPath,
				AutoPublish:         tc.autoPublishOpt,
				AutoPublishExplicit: tc.autoPublishExplicit,
				ChangelogFA:         "بهبود عملکرد",
				ChangelogEN:         "Performance improvements",
				PollInterval:        time.Millisecond,
			})
			if err != nil {
				t.Fatalf("publish run failed: %v", err)
			}
			if !res.Committed {
				t.Fatal("expected release to be committed")
			}
			if gotCommit.AutoPublish != tc.wantAutoPublish || res.AutoPublish != tc.wantAutoPublish {
				t.Fatalf("got auto_publish commit=%v res=%v, want %v", gotCommit.AutoPublish, res.AutoPublish, tc.wantAutoPublish)
			}
			prompted := strings.Contains(errBuf.String(), "Enable auto-publish after CafeBazaar approval?")
			if prompted != tc.wantPrompted {
				t.Fatalf("got prompted=%v, want %v (stderr=%q)", prompted, tc.wantPrompted, errBuf.String())
			}
		})
	}
}
