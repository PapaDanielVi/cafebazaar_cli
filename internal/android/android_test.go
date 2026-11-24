package android_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
)

const (
	kotlinBuildFixture = `
plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "ir.example.market"
    compileSdk = 35

    defaultConfig {
        applicationId = "ir.example.market"
        minSdk = 24
        targetSdk = 35
        versionCode = 42
        versionName = "2.4.1"
    }

    buildTypes {
        getByName("debug") {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
        create("staging") {
            applicationIdSuffix = ".staging"
            versionNameSuffix = "-staging"
        }
        release {
            isMinifyEnabled = true
        }
    }

    flavorDimensions += "tier"
    productFlavors {
        create("free") {
            dimension = "tier"
            applicationIdSuffix = ".free"
        }
        create("pro") {
            dimension = "tier"
            applicationId = "ir.example.marketpro"
        }
    }
}
`

	groovyBuildFixture = `
plugins {
    id 'com.android.application'
}

android {
    defaultConfig {
        applicationId 'com.demo.notes'
        versionCode 7
        versionName '1.0.3'
    }
    buildTypes {
        debug {
            applicationIdSuffix '.dev'
        }
        beta {
            applicationIdSuffix '.beta'
        }
        release {
            minifyEnabled true
        }
    }
}
`
)

func TestDetectKotlinDSLVariantsAndArtifacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	apkDir := filepath.Join(appDir, "build", "outputs", "apk", "free", "release")
	aabDir := filepath.Join(appDir, "build", "outputs", "bundle", "proStaging")
	for _, d := range []string{apkDir, aabDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	if err := os.WriteFile(filepath.Join(root, android.FileSettingsKTS), []byte(`include(":app")`), 0o644); err != nil {
		t.Fatalf("write settings.gradle.kts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(appDir, android.FileBuildKTS), []byte(kotlinBuildFixture), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}
	apkPath := filepath.Join(apkDir, "app-free-release.apk")
	aabPath := filepath.Join(aabDir, "app-pro-staging.aab")
	binPath := filepath.Join(aabDir, "app-pro-staging.bin")
	for _, f := range []string{apkPath, aabPath, binPath} {
		if err := os.WriteFile(f, []byte("binary"), 0o644); err != nil {
			t.Fatalf("write artifact %s: %v", f, err)
		}
	}

	proj, err := android.DetectProject(ctx, apkDir)
	if err != nil {
		t.Fatalf("detect project from nested dir: %v", err)
	}
	mod, ok := proj.PrimaryModule(ctx, "")
	if !ok {
		t.Fatal("expected primary module")
	}
	if mod.ApplicationID != "ir.example.market" || mod.VersionCode != 42 || mod.VersionName != "2.4.1" {
		t.Fatalf("unexpected module metadata: %+v", mod)
	}

	// Expect 2 productFlavors x 3 buildTypes = 6 variants.
	if len(mod.Variants) != 6 {
		t.Fatalf("got %d variants, want 6: %+v", len(mod.Variants), mod.Variants)
	}

	freeStaging, err := mod.FindVariant(ctx, "freeStaging")
	if err != nil {
		t.Fatalf("find freeStaging: %v", err)
	}
	if freeStaging.PackageName != "ir.example.market.free.staging" {
		t.Fatalf("got package %q, want ir.example.market.free.staging", freeStaging.PackageName)
	}
	if freeStaging.BundleTask != ":app:bundleFreeStaging" {
		t.Fatalf("got bundle task %q, want :app:bundleFreeStaging", freeStaging.BundleTask)
	}

	proRelease, err := mod.FindVariant(ctx, "proRelease")
	if err != nil {
		t.Fatalf("find proRelease: %v", err)
	}
	if proRelease.PackageName != "ir.example.marketpro" {
		t.Fatalf("got package %q, want ir.example.marketpro", proRelease.PackageName)
	}

	if gotBin := android.FindNearbyBin(ctx, aabPath); gotBin != binPath {
		t.Fatalf("got nearby bin %q, want %q", gotBin, binPath)
	}
	if len(mod.Artifacts) != 3 {
		t.Fatalf("got %d artifacts, want 3", len(mod.Artifacts))
	}
}

func TestDetectGroovyAndApplyVersionBump(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, android.FileSettingsGroovy), []byte(`include ':app'`), 0o644); err != nil {
		t.Fatalf("write settings.gradle: %v", err)
	}
	buildPath := filepath.Join(appDir, android.FileBuildGroovy)
	if err := os.WriteFile(buildPath, []byte(groovyBuildFixture), 0o644); err != nil {
		t.Fatalf("write build.gradle: %v", err)
	}

	proj, err := android.DetectProject(ctx, root)
	if err != nil {
		t.Fatalf("detect groovy project: %v", err)
	}
	mod, ok := proj.PrimaryModule(ctx, "")
	if !ok {
		t.Fatal("expected primary module")
	}

	pkgs := proj.UniquePackageNames(ctx)
	if len(pkgs) != 3 {
		t.Fatalf("got packages %v, want 3 build types", pkgs)
	}

	plan, err := android.ComputeBump(ctx, mod, android.BumpOptions{
		Kind:           android.BumpMinor,
		MinVersionCode: 10,
	})
	if err != nil {
		t.Fatalf("compute bump: %v", err)
	}
	if plan.NewVersionCode != 11 || plan.NewVersionName != "1.1.0" {
		t.Fatalf("unexpected bump plan: %+v", plan)
	}

	if err := android.ApplyBump(ctx, plan, false); err != nil {
		t.Fatalf("apply bump: %v", err)
	}
	raw, err := os.ReadFile(buildPath)
	if err != nil {
		t.Fatalf("read bumped build.gradle: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "versionCode 11") || !strings.Contains(text, "versionName '1.1.0'") {
		t.Fatalf("unexpected updated build.gradle:\n%s", text)
	}
}

func TestDetectKotlinDelegatedAndGenericVariants(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	root := t.TempDir()
	appDir := filepath.Join(root, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, android.FileSettingsKTS), []byte(`include(":app")`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	kts := `
plugins {
    id("com.android.application")
}
android {
    defaultConfig {
        applicationId = "ir.demo.multi"
        versionCode = 1
        versionName = "1.0.0"
    }
    buildTypes {
        val debug by getting {}
        val release by getting {}
        val staging by creating {
            applicationIdSuffix = ".staging"
        }
        create<ApplicationBuildType>("beta") {
            applicationIdSuffix = ".beta"
        }
    }
    flavorDimensions += "store"
    productFlavors {
        val bazaar by creating {
            dimension = "store"
        }
        val play by creating {
            dimension = "store"
        }
    }
}
`
	if err := os.WriteFile(filepath.Join(appDir, android.FileBuildKTS), []byte(kts), 0o644); err != nil {
		t.Fatalf("write build.gradle.kts: %v", err)
	}

	proj, err := android.DetectProject(ctx, root)
	if err != nil {
		t.Fatalf("detect project: %v", err)
	}
	mod, ok := proj.PrimaryModule(ctx, "")
	if !ok {
		t.Fatal("expected primary module")
	}
	// 2 flavors (bazaar, play) x 4 build types (debug, release, staging, beta) = 8 variants.
	if len(mod.Variants) != 8 {
		t.Fatalf("got %d variants, want 8: %+v", len(mod.Variants), mod.Variants)
	}
	for _, want := range []string{"bazaarRelease", "bazaarStaging", "bazaarBeta", "playRelease", "playStaging", "playBeta"} {
		if _, err := mod.FindVariant(ctx, want); err != nil {
			t.Fatalf("expected variant %q to be detected: %v", want, err)
		}
	}
}
