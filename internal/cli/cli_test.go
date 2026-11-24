package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	"github.com/PapaDanielVi/cafebazaar_cli/internal/gitflow"
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

	writePNG := func(path string, w, h int) string {
		t.Helper()
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		img.Set(0, 0, color.RGBA{R: 0, G: 177, B: 79, A: 255})
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("encode png: %v", err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
			t.Fatalf("write png: %v", err)
		}
		return path
	}
	assetsDir := t.TempDir()
	iconPath := writePNG(filepath.Join(assetsDir, "icon.png"), 512, 512)
	shotsDir := filepath.Join(assetsDir, "shots")
	if err := os.MkdirAll(shotsDir, 0o755); err != nil {
		t.Fatalf("mkdir shots: %v", err)
	}
	writePNG(filepath.Join(shotsDir, "01.png"), 1080, 1920)
	writePNG(filepath.Join(shotsDir, "02.png"), 1080, 1920)

	var gotShotCount int
	var gotNameFA string
	pubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == publisher.EndpointApps {
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Errorf("parse multipart: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			gotNameFA = r.FormValue(publisher.FieldNameFA)
			gotShotCount = len(r.MultipartForm.File[publisher.FieldScreenshots])
			w.WriteHeader(http.StatusCreated)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"New app successfully created and submitted for review.","package_name":"ir.demo.shop","version_code":1,"version_name":"1.0","app_url":"https://cafebazaar.ir/app/ir.demo.shop/","review_state":"in_review","auto_publish":true,"api_agent":{"token":"issued-app-token-777"},"warnings":[]}`)
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

	if err := runCLI(
		"init",
		"--mode", "new",
		"--package", "ir.demo.shop",
		"--category-id", "10",
		"--name-fa", "فروشگاه نمونه",
		"--name-en", "Demo Shop",
		"--short-description-fa", "توضیح کوتاه فروشگاه",
		"--description-fa", "توضیحات کامل اپلیکیشن فروشگاه نمونه",
		"--email", "dev@example.com",
		"--icon", iconPath,
		"--screenshot", shotsDir,
		"--yes", "--plain",
	); err != nil {
		t.Fatalf("bazaar init --mode new failed: %v", err)
	}
	if gotNameFA != "فروشگاه نمونه" || gotShotCount != 2 {
		t.Fatalf("unexpected multipart submission: name_fa=%q shots=%d", gotNameFA, gotShotCount)
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

func TestResolveTaggedRepoVariant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	initGitRepo := func(t *testing.T, withTag bool) string {
		t.Helper()
		dir := t.TempDir()
		runGit := func(args ...string) {
			t.Helper()
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v (%s)", args, err, string(out))
			}
		}
		runGit("init")
		runGit("config", "user.email", "dev@example.com")
		runGit("config", "user.name", "Dev")
		runGit("config", "commit.gpgsign", "false")
		runGit("config", "tag.gpgsign", "false")
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("demo"), 0o644); err != nil {
			t.Fatalf("write readme: %v", err)
		}
		if err := gitflow.CommitFiles(ctx, dir, []string{"README.md"}, "chore: init"); err != nil {
			t.Fatalf("commit readme: %v", err)
		}
		if withTag {
			if err := gitflow.CreateTag(ctx, dir, "v1.2.0", "Release 1.2.0"); err != nil {
				t.Fatalf("create tag: %v", err)
			}
		}
		return dir
	}

	baseMod := android.AppModule{
		Name: ":app",
		Variants: []android.Variant{
			{Name: "debug", BuildType: android.BuildTypeDebug, PackageName: "ir.demo.shop.debug"},
			{Name: "release", BuildType: android.BuildTypeRelease, PackageName: "ir.demo.shop"},
		},
	}
	flavoredMod := android.AppModule{
		Name: ":app",
		Variants: []android.Variant{
			{Name: "proDebug", BuildType: android.BuildTypeDebug, Flavors: []string{"pro"}, PackageName: "ir.demo.pro.debug"},
			{Name: "proRelease", BuildType: android.BuildTypeRelease, Flavors: []string{"pro"}, PackageName: "ir.demo.pro"},
			{Name: "freeDebug", BuildType: android.BuildTypeDebug, Flavors: []string{"free"}, PackageName: "ir.demo.free.debug"},
			{Name: "freeRelease", BuildType: android.BuildTypeRelease, Flavors: []string{"free"}, PackageName: "ir.demo.free"},
		},
	}

	tests := []struct {
		name            string
		withTag         bool
		mod             android.AppModule
		current         android.Variant
		explicitVariant string
		input           string
		wantVariant     string
		wantPkg         string
		wantPrompt      bool
	}{
		{
			name:        "tagged repo accepts switching debug to release",
			withTag:     true,
			mod:         baseMod,
			current:     baseMod.Variants[0],
			input:       "y\n",
			wantVariant: "release",
			wantPkg:     "ir.demo.shop",
			wantPrompt:  true,
		},
		{
			name:        "tagged repo declines switching keeps debug",
			withTag:     true,
			mod:         baseMod,
			current:     baseMod.Variants[0],
			input:       "n\n",
			wantVariant: "debug",
			wantPkg:     "ir.demo.shop.debug",
			wantPrompt:  true,
		},
		{
			name:        "tagged repo matches flavored release variant",
			withTag:     true,
			mod:         flavoredMod,
			current:     flavoredMod.Variants[2],
			input:       "1\n",
			wantVariant: "freeRelease",
			wantPkg:     "ir.demo.free",
			wantPrompt:  true,
		},
		{
			name:        "tagged repo with multiple non-debug variants lets user pick another variant",
			withTag:     true,
			mod:         flavoredMod,
			current:     flavoredMod.Variants[2],
			input:       "proRelease\n",
			wantVariant: "proRelease",
			wantPkg:     "ir.demo.pro",
			wantPrompt:  true,
		},
		{
			name:        "untagged repo skips prompt",
			withTag:     false,
			mod:         baseMod,
			current:     baseMod.Variants[0],
			input:       "y\n",
			wantVariant: "debug",
			wantPkg:     "ir.demo.shop.debug",
			wantPrompt:  false,
		},
		{
			name:            "explicit variant flag skips prompt",
			withTag:         true,
			mod:             baseMod,
			current:         baseMod.Variants[0],
			explicitVariant: "debug",
			input:           "y\n",
			wantVariant:     "debug",
			wantPkg:         "ir.demo.shop.debug",
			wantPrompt:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repoDir := initGitRepo(t, tc.withTag)
			var outBuf, errBuf bytes.Buffer
			printer := output.NewPrinter(ctx, strings.NewReader(tc.input), &outBuf, &errBuf, output.RuntimeMode{
				Interactive: true,
				Format:      output.FormatPlain,
			})

			gotVariant, gotPkg, err := cli.ResolveTaggedRepoVariant(ctx, printer, repoDir, tc.mod, tc.current, tc.explicitVariant, tc.current.PackageName)
			if err != nil {
				t.Fatalf("resolve tagged repo variant: %v", err)
			}
			if gotVariant.Name != tc.wantVariant {
				t.Fatalf("got variant %q, want %q", gotVariant.Name, tc.wantVariant)
			}
			if gotPkg != tc.wantPkg {
				t.Fatalf("got package %q, want %q", gotPkg, tc.wantPkg)
			}
			prompted := strings.Contains(errBuf.String(), "variant instead of")
			if prompted != tc.wantPrompt {
				t.Fatalf("got prompted=%v, want %v (stderr=%q)", prompted, tc.wantPrompt, errBuf.String())
			}
		})
	}
}

func TestSelectInitVariantListsAllVariants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	multiMod := android.AppModule{
		Name: ":app",
		Variants: []android.Variant{
			{Name: "bazaarDebug", BuildType: android.BuildTypeDebug, Flavors: []string{"bazaar"}, PackageName: "ir.demo.app"},
			{Name: "bazaarRelease", BuildType: android.BuildTypeRelease, Flavors: []string{"bazaar"}, PackageName: "ir.demo.app"},
			{Name: "bazaarStaging", BuildType: "staging", Flavors: []string{"bazaar"}, PackageName: "ir.demo.app"},
			{Name: "playDebug", BuildType: android.BuildTypeDebug, Flavors: []string{"play"}, PackageName: "ir.demo.app"},
			{Name: "playRelease", BuildType: android.BuildTypeRelease, Flavors: []string{"play"}, PackageName: "ir.demo.app"},
			{Name: "beta", BuildType: "beta", PackageName: "ir.demo.app.beta"},
		},
	}

	tests := []struct {
		name        string
		filterPkg   string
		stdin       string
		wantVariant string
		wantPkg     string
		wantListed  []string
	}{
		{
			name:        "lists all variants even when sharing package name",
			stdin:       "bazaarStaging\n",
			wantVariant: "bazaarStaging",
			wantPkg:     "ir.demo.app",
			wantListed:  []string{"bazaarRelease", "playRelease", "bazaarStaging", "beta", "bazaarDebug", "playDebug"},
		},
		{
			name:        "selects custom build type by number",
			stdin:       "4\n",
			wantVariant: "beta",
			wantPkg:     "ir.demo.app.beta",
			wantListed:  []string{"bazaarRelease", "playRelease", "bazaarStaging", "beta"},
		},
		{
			name:        "filters variants when package flag matches multiple variants",
			filterPkg:   "ir.demo.app",
			stdin:       "playRelease\n",
			wantVariant: "playRelease",
			wantPkg:     "ir.demo.app",
			wantListed:  []string{"bazaarRelease", "playRelease", "bazaarStaging", "bazaarDebug", "playDebug"},
		},
		{
			name:        "allows custom package entry",
			stdin:       "__custom__\nir.custom.pkg\n",
			wantVariant: "",
			wantPkg:     "ir.custom.pkg",
			wantListed:  []string{"Enter custom package name..."},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var outBuf, errBuf bytes.Buffer
			printer := output.NewPrinter(ctx, strings.NewReader(tc.stdin), &outBuf, &errBuf, output.RuntimeMode{
				Interactive: true,
				Format:      output.FormatPlain,
			})

			gotVar, gotPkg, err := cli.SelectInitVariant(ctx, printer, multiMod, tc.filterPkg)
			if err != nil {
				t.Fatalf("select init variant: %v", err)
			}
			if gotVar.Name != tc.wantVariant {
				t.Fatalf("got variant %q, want %q", gotVar.Name, tc.wantVariant)
			}
			if gotPkg != tc.wantPkg {
				t.Fatalf("got package %q, want %q", gotPkg, tc.wantPkg)
			}
			promptOut := errBuf.String()
			for _, label := range tc.wantListed {
				if !strings.Contains(promptOut, label) {
					t.Fatalf("expected prompt to list %q, got:\n%s", label, promptOut)
				}
			}
		})
	}
}

func TestSelectCategoryID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name   string
		stdin  string
		wantID int
	}{
		{
			name:   "selects official category by id value",
			stdin:  "58\n",
			wantID: 58,
		},
		{
			name:   "selects first official category (Tools=11)",
			stdin:  "1\n",
			wantID: 11,
		},
		{
			name:   "allows custom category id entry",
			stdin:  "__custom_category__\n99\n",
			wantID: 99,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var outBuf, errBuf bytes.Buffer
			printer := output.NewPrinter(ctx, strings.NewReader(tc.stdin), &outBuf, &errBuf, output.RuntimeMode{
				Interactive: true,
				Format:      output.FormatPlain,
			})
			gotID, err := cli.SelectCategoryID(ctx, printer)
			if err != nil {
				t.Fatalf("select category id: %v", err)
			}
			if gotID != tc.wantID {
				t.Fatalf("got category id %d, want %d", gotID, tc.wantID)
			}
			if !strings.Contains(errBuf.String(), "Tools") || !strings.Contains(errBuf.String(), "Strategy") {
				t.Fatalf("expected official categories in prompt output, got:\n%s", errBuf.String())
			}
		})
	}
}

func TestStatusShowsCurrentRolloutPercentage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pishkhanSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastPublished:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","release":{"packages":[{"version_code":21,"version_name":"2.1.0"}],"staged_rollout_percentage":25,"auto_publish":true}}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastCommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastUncommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"not-exists"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer pishkhanSrv.Close()

	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	runStatus := func(args ...string) string {
		t.Helper()
		var outBuf, errBuf bytes.Buffer
		cmd := cli.NewRootCmd(ctx, cli.Dependencies{
			In:              strings.NewReader(""),
			Out:             &outBuf,
			Err:             &errBuf,
			WorkDir:         t.TempDir(),
			PishkhanBaseURL: pishkhanSrv.URL,
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
		cmd.SetArgs(args)
		if err := cmd.ExecuteContext(ctx); err != nil {
			t.Fatalf("status %v failed: %v", args, err)
		}
		return outBuf.String()
	}

	plainOut := runStatus("status", "--plain")
	if !strings.Contains(plainOut, "Rollout: [STAGED] 25% (published release)") {
		t.Fatalf("expected plain status output to include staged rollout row, got:\n%s", plainOut)
	}

	jsonOut := runStatus("status", "--output", "json")
	if !strings.Contains(jsonOut, `"percentage": 25`) || !strings.Contains(jsonOut, `"active": true`) {
		t.Fatalf("expected json status output to include staged_rollout summary, got:\n%s", jsonOut)
	}
}
