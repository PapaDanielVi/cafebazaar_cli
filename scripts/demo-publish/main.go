// Package main provides a self-contained demo harness and local mock CafeBazaar API
// used by `make demo-publish` to record the end-to-end publish-from-scratch VHS demo
// with the real bazaar CLI without touching any live CafeBazaar developer account.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/auth"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/cli"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/config"
)

const (
	envDemoAPIURL   = "BAZAAR_DEMO_API_URL"
	flagPrepare     = "--prepare"
	flagServe       = "--serve"
	demoPackageName = "ir.example.notes"
	demoAppVersion  = "1.0.0"
	demoNextVersion = "1.0.1"
	demoCLIVersion  = "v1.3.0"
	demoStateReview = "in_review"
	demoAPKSize     = 2 * 1024 * 1024
	demoDirPerm     = 0o755
	demoFilePerm    = 0o600
	demoExecPerm    = 0o755
	uploadPaceDelay = 12 * time.Millisecond
	uploadChunkSize = 32 * 1024
	statusPause     = 220 * time.Millisecond
	processPause    = 650 * time.Millisecond
	commitPause     = 550 * time.Millisecond
	serverTimeout   = 5 * time.Second
	readHeaderLimit = 10 * time.Second
)

type demoPackageInfo struct {
	VersionCode  int    `json:"version_code"`
	VersionName  string `json:"version_name"`
	Architecture string `json:"architecture,omitempty"`
}

type demoReleaseInfo struct {
	Packages                []demoPackageInfo `json:"packages"`
	StagedRolloutPercentage int               `json:"staged_rollout_percentage,omitempty"`
	AutoPublish             bool              `json:"auto_publish,omitempty"`
	ReviewState             string            `json:"review_state,omitempty"`
}

type demoReleaseEnvelope struct {
	Type    string           `json:"type"`
	Release *demoReleaseInfo `json:"release,omitempty"`
	Package *demoPackageInfo `json:"package,omitempty"`
}

type demoCreateAppResponse struct {
	PackageName string            `json:"package_name"`
	VersionCode int               `json:"version_code"`
	VersionName string            `json:"version_name"`
	ReviewState string            `json:"review_state"`
	AutoPublish bool              `json:"auto_publish"`
	AppURL      string            `json:"app_url"`
	APIAgent    map[string]string `json:"api_agent"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string) int {

	if len(args) >= 3 && args[1] == flagPrepare {
		if err := prepareDemoWorkspace(ctx, args[2]); err != nil {
			writeErr(ctx, err)
			return 1
		}
		return 0
	}
	if len(args) >= 3 && args[1] == flagServe {
		if err := serveMockAPI(ctx, args[2]); err != nil {
			writeErr(ctx, err)
			return 1
		}
		return 0
	}

	apiURL := strings.TrimSpace(os.Getenv(envDemoAPIURL))
	cli.Version = demoCLIVersion
	cli.Commit = "demo"
	root := cli.NewRootCmd(ctx, cli.Dependencies{
		In:               os.Stdin,
		Out:              os.Stdout,
		Err:              os.Stderr,
		PishkhanBaseURL:  apiURL,
		CommentsBaseURL:  apiURL,
		PublisherBaseURL: apiURL,
	})
	if err := root.ExecuteContext(ctx); err != nil {
		writeErr(ctx, err)
		return cli.ExitCode(ctx, err)
	}
	return 0
}

func writeErr(ctx context.Context, err error) {
	_ = ctx
	if _, wErr := fmt.Fprintf(os.Stderr, "Error: %v\n", err); wErr != nil {
		return
	}
}

func prepareDemoWorkspace(ctx context.Context, rawRoot string) error {
	rootDir, err := filepath.Abs(filepath.Clean(rawRoot))
	if err != nil {
		return fmt.Errorf("resolve demo root: %w", err)
	}
	homeDir := filepath.Join(rootDir, "home")
	projectDir := filepath.Join(homeDir, "projects", "simple-notes")
	cfgDir := filepath.Join(homeDir, ".config", config.AppDirName)
	for _, dir := range []string{
		filepath.Join(projectDir, "app", "src", "main"),
		filepath.Join(projectDir, "app", "build", "outputs", "apk", "release"),
		filepath.Join(projectDir, "assets"),
		filepath.Join(projectDir, "notes"),
		cfgDir,
	} {
		if err := os.MkdirAll(dir, demoDirPerm); err != nil { //nolint:gosec // Local temporary demo workspace directory.
			return fmt.Errorf("create demo dir %q: %w", dir, err)
		}
	}

	settingsGradle := "rootProject.name = \"SimpleNotes\"\ninclude(\":app\")\n"
	if err := writeDemoFile(ctx, filepath.Join(projectDir, "settings.gradle.kts"), []byte(settingsGradle), demoFilePerm); err != nil {
		return err
	}

	buildGradle := `plugins {
    id("com.android.application")
}

android {
    namespace = "ir.example.notes"
    compileSdk = 35

    defaultConfig {
        applicationId = "ir.example.notes"
        minSdk = 24
        targetSdk = 35
        versionCode = 1
        versionName = "1.0.0"
    }

    buildTypes {
        release {
            isMinifyEnabled = true
        }
        debug {
            applicationIdSuffix = ".debug"
        }
    }
}
`
	if err := writeDemoFile(ctx, filepath.Join(projectDir, "app", "build.gradle.kts"), []byte(buildGradle), demoFilePerm); err != nil {
		return err
	}

	gradlewScript := `#!/usr/bin/env bash
set -euo pipefail
echo "> Task :app:compileReleaseKotlin"
sleep 0.25
echo "> Task :app:packageRelease"
sleep 0.25
echo "> Task $1"
mkdir -p app/build/outputs/apk/release
dd if=/dev/zero of=app/build/outputs/apk/release/app-release.apk bs=1024 count=2048 2>/dev/null
echo "BUILD SUCCESSFUL in 1s"
`
	if err := writeDemoFile(ctx, filepath.Join(projectDir, "gradlew"), []byte(gradlewScript), demoExecPerm); err != nil {
		return err
	}

	apkBytes := make([]byte, demoAPKSize)
	if err := writeDemoFile(ctx, filepath.Join(projectDir, "app", "build", "outputs", "apk", "release", "app-release.apk"), apkBytes, demoFilePerm); err != nil {
		return err
	}

	if err := writeDemoPNG(ctx, filepath.Join(projectDir, "assets", "icon.png"), 512, 512, color.RGBA{R: 0, G: 177, B: 79, A: 255}); err != nil {
		return err
	}
	if err := writeDemoPNG(ctx, filepath.Join(projectDir, "assets", "screen1.png"), 720, 1280, color.RGBA{R: 15, G: 23, B: 42, A: 255}); err != nil {
		return err
	}
	if err := writeDemoFile(ctx, filepath.Join(projectDir, "assets", "desc-fa.txt"), []byte("دفترچه یادداشت سریع و سبک با پشتیبانی کامل فارسی."), demoFilePerm); err != nil {
		return err
	}
	if err := writeDemoFile(ctx, filepath.Join(projectDir, "assets", "desc-en.txt"), []byte("Fast and lightweight notes app with full Persian support."), demoFilePerm); err != nil {
		return err
	}
	if err := writeDemoFile(ctx, filepath.Join(projectDir, "notes", "fa.txt"), []byte("بهبود سرعت و رفع خطاهای جزئی"), demoFilePerm); err != nil {
		return err
	}

	if err := initDemoGitRepo(ctx, projectDir); err != nil {
		return err
	}

	cfgPath := filepath.Join(cfgDir, config.GlobalConfigFileName)
	credPath := config.CredentialsPathForConfig(ctx, cfgPath)
	store := auth.NewFileStore(ctx, credPath)
	if _, err := store.Set(ctx, auth.PublisherKey(ctx, auth.DefaultAccount), demoPlaceholderSecret(ctx, "nasher")); err != nil {
		return fmt.Errorf("seed demo publisher token: %w", err)
	}
	globalCfg := &config.GlobalConfig{
		RTLMode: config.DefaultRTLMode,
		Publisher: &config.PublisherConfig{
			Account: auth.DefaultAccount,
			Backend: auth.BackendFile,
			AddedAt: time.Now().UTC(),
		},
		Apps: map[string]config.AppEntry{},
	}
	if err := config.SaveGlobal(ctx, cfgPath, globalCfg); err != nil {
		return fmt.Errorf("save demo global config: %w", err)
	}
	return nil
}

func demoPlaceholderSecret(ctx context.Context, kind string) string {
	_ = ctx
	return "demo-" + kind + "-placeholder-0000"
}

func writeDemoFile(ctx context.Context, path string, data []byte, perm os.FileMode) error {
	_ = ctx
	if err := os.WriteFile(path, data, perm); err != nil { //nolint:gosec // Controlled demo workspace path.
		return fmt.Errorf("write demo file %q: %w", path, err)
	}
	return nil
}

func writeDemoPNG(ctx context.Context, path string, width, height int, fill color.RGBA) error {
	_ = ctx
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, fill)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, demoFilePerm) //nolint:gosec // Controlled demo workspace path.
	if err != nil {
		return fmt.Errorf("create png %q: %w", path, err)
	}
	encodeErr := png.Encode(f, img)
	closeErr := f.Close()
	if err := errors.Join(encodeErr, closeErr); err != nil {
		return fmt.Errorf("write png %q: %w", path, err)
	}
	return nil
}

func initDemoGitRepo(ctx context.Context, dir string) error {
	cmds := [][]string{
		{"init", "-b", "main"},
		{"config", "user.name", "Demo Developer"},
		{"config", "user.email", "dev@example.com"},
		{"add", "settings.gradle.kts", "app/build.gradle.kts", "gradlew", "assets", "notes"},
		{"commit", "-m", "chore: initial Android app scaffold"},
	}
	for _, args := range cmds {
		c := exec.CommandContext(ctx, "git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %s (%w)", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}

type mockState struct {
	mu               sync.Mutex
	hasUncommitted   bool
	publishedRollout int
	publishedVersion string
	publishedCode    int
	committedRollout int
	committedAutoPub bool
	committedVersion string
	committedCode    int
	hasCommitted     bool
	uploadedAPK      bool
}

func serveMockAPI(ctx context.Context, addrFile string) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen mock api: %w", err)
	}
	apiURL := "http://" + ln.Addr().String()
	if err := writeDemoFile(ctx, addrFile, []byte(apiURL), demoFilePerm); err != nil {
		closeErr := ln.Close()
		return fmt.Errorf("write addr file: %w", errors.Join(err, closeErr))
	}

	state := &mockState{
		publishedRollout: 25,
		publishedVersion: demoAppVersion,
		publishedCode:    1,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(publisher.EndpointApps, func(w http.ResponseWriter, r *http.Request) {
		drainPacedBody(r.Context(), r.Body)
		sleepCtx(r.Context(), processPause)
		writeJSON(r.Context(), w, http.StatusCreated, demoCreateAppResponse{
			PackageName: demoPackageName,
			VersionCode: 1,
			VersionName: demoAppVersion,
			ReviewState: demoStateReview,
			AutoPublish: false,
			AppURL:      "https://cafebazaar.ir/app/" + demoPackageName,
			APIAgent: map[string]string{
				"token": demoPlaceholderSecret(r.Context(), "pishkhan"),
			},
		})
	})

	mux.HandleFunc(pishkhan.EndpointLastPublished, func(w http.ResponseWriter, r *http.Request) {
		sleepCtx(r.Context(), statusPause)
		state.mu.Lock()
		defer state.mu.Unlock()
		writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{
			Type: pishkhan.ResponseTypeSuccess,
			Release: &demoReleaseInfo{
				Packages:                []demoPackageInfo{{VersionCode: state.publishedCode, VersionName: state.publishedVersion}},
				StagedRolloutPercentage: state.publishedRollout,
				AutoPublish:             false,
			},
		})
	})

	mux.HandleFunc(pishkhan.EndpointLastCommitted, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if !state.hasCommitted {
			writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{Type: pishkhan.ResponseTypeNotExists})
			return
		}
		writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{
			Type: pishkhan.ResponseTypeSuccess,
			Release: &demoReleaseInfo{
				Packages:                []demoPackageInfo{{VersionCode: state.committedCode, VersionName: state.committedVersion}},
				StagedRolloutPercentage: state.committedRollout,
				AutoPublish:             state.committedAutoPub,
				ReviewState:             demoStateReview,
			},
		})
	})

	mux.HandleFunc(pishkhan.EndpointLastUncommitted, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if !state.hasUncommitted {
			writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{Type: pishkhan.ResponseTypeNotExists})
			return
		}
		pkgs := []demoPackageInfo{}
		if state.uploadedAPK {
			pkgs = append(pkgs, demoPackageInfo{VersionCode: 2, VersionName: demoNextVersion})
		}
		writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{
			Type:    pishkhan.ResponseTypeSuccess,
			Release: &demoReleaseInfo{Packages: pkgs},
		})
	})

	mux.HandleFunc(pishkhan.EndpointReleases, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.Method {
		case http.MethodPost:
			state.hasUncommitted = true
			state.uploadedAPK = false
			writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{
				Type:    pishkhan.ResponseTypeSuccess,
				Release: &demoReleaseInfo{Packages: []demoPackageInfo{}},
			})
		case http.MethodDelete:
			state.hasUncommitted = false
			state.uploadedAPK = false
			writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{Type: pishkhan.ResponseTypeSuccess})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc(pishkhan.EndpointUploadAPK, func(w http.ResponseWriter, r *http.Request) {
		drainPacedBody(r.Context(), r.Body)
		sleepCtx(r.Context(), processPause)
		state.mu.Lock()
		state.uploadedAPK = true
		state.mu.Unlock()
		writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{
			Type: pishkhan.ResponseTypeSuccess,
			Package: &demoPackageInfo{
				VersionCode:  2,
				VersionName:  demoNextVersion,
				Architecture: string(pishkhan.ArchitectureAll),
			},
		})
	})

	mux.HandleFunc(pishkhan.EndpointCommit, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			StagedRolloutPercentage int  `json:"staged_rollout_percentage"`
			AutoPublish             bool `json:"auto_publish"`
		}
		if decErr := json.NewDecoder(r.Body).Decode(&body); decErr != nil {
			body.StagedRolloutPercentage = 100
		}
		sleepCtx(r.Context(), commitPause)
		if body.StagedRolloutPercentage <= 0 {
			body.StagedRolloutPercentage = 100
		}
		state.mu.Lock()
		state.hasUncommitted = false
		state.hasCommitted = true
		state.committedCode = 2
		state.committedVersion = demoNextVersion
		state.committedRollout = body.StagedRolloutPercentage
		state.committedAutoPub = body.AutoPublish
		state.mu.Unlock()
		writeJSON(r.Context(), w, http.StatusOK, demoReleaseEnvelope{
			Type: pishkhan.ResponseTypeSuccess,
			Release: &demoReleaseInfo{
				Packages:                []demoPackageInfo{{VersionCode: 2, VersionName: demoNextVersion}},
				StagedRolloutPercentage: body.StagedRolloutPercentage,
				AutoPublish:             body.AutoPublish,
				ReviewState:             demoStateReview,
			},
		})
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: readHeaderLimit,
	}

	go func(parentCtx context.Context) {
		<-parentCtx.Done()
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(parentCtx), serverTimeout)
		defer cancel()
		if shutErr := srv.Shutdown(shutCtx); shutErr != nil {
			return
		}
	}(ctx)

	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func drainPacedBody(ctx context.Context, body io.ReadCloser) {
	defer func() {
		if err := body.Close(); err != nil {
			return
		}
	}()
	buf := make([]byte, uploadChunkSize)
	for {
		_, err := body.Read(buf)
		if err != nil {
			return
		}
		sleepCtx(ctx, uploadPaceDelay)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func writeJSON(ctx context.Context, w http.ResponseWriter, status int, payload any) {
	_ = ctx
	raw, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	w.WriteHeader(status)
	if _, writeErr := w.Write(raw); writeErr != nil {
		return
	}
}
