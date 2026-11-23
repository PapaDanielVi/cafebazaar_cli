package android

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// ArtifactTypeAPK identifies an Android APK output file.
	ArtifactTypeAPK = "apk"
	// ArtifactTypeAAB identifies an Android App Bundle output file.
	ArtifactTypeAAB = "aab"
	// ArtifactTypeBin identifies a CafeBazaar Bundle Signer .bin file.
	ArtifactTypeBin = "bin"

	extAPK = ".apk"
	extAAB = ".aab"
	extBin = ".bin"
)

// Artifact describes a discovered APK, AAB, or .bin file in a Gradle module output tree.
type Artifact struct {
	Path        string    `json:"path" yaml:"path"`
	RelPath     string    `json:"rel_path" yaml:"rel_path"`
	Type        string    `json:"type" yaml:"type"`
	VariantName string    `json:"variant_name,omitempty" yaml:"variant_name,omitempty"`
	SizeBytes   int64     `json:"size_bytes" yaml:"size_bytes"`
	ModTime     time.Time `json:"mod_time" yaml:"mod_time"`
}

// DiscoverArtifacts locates APK, AAB, and .bin build outputs under mod.Dir/build/outputs.
func DiscoverArtifacts(ctx context.Context, mod AppModule) []Artifact {
	outputsDir := filepath.Join(mod.Dir, "build", "outputs")
	if _, err := os.Stat(outputsDir); err != nil {
		return nil
	}

	var found []Artifact
	walkErr := filepath.WalkDir(outputsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ext := strings.ToLower(filepath.Ext(path))
		var artifactType string
		switch ext {
		case extAPK:
			artifactType = ArtifactTypeAPK
		case extAAB:
			artifactType = ArtifactTypeAAB
		case extBin:
			artifactType = ArtifactTypeBin
		default:
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(mod.Dir, path)
		if relErr != nil {
			rel = path
		}

		found = append(found, Artifact{
			Path:        path,
			RelPath:     rel,
			Type:        artifactType,
			VariantName: inferArtifactVariant(ctx, rel, mod.Variants),
			SizeBytes:   info.Size(),
			ModTime:     info.ModTime(),
		})
		return nil
	})
	if walkErr != nil {
		return found
	}

	sort.Slice(found, func(i, j int) bool {
		if !found[i].ModTime.Equal(found[j].ModTime) {
			return found[i].ModTime.After(found[j].ModTime)
		}
		return found[i].Path < found[j].Path
	})

	return found
}

// FindNearbyBin searches the same directory as aabPath for a CafeBazaar Bundle Signer .bin file.
func FindNearbyBin(ctx context.Context, aabPath string) string {
	_ = ctx
	dir := filepath.Dir(strings.TrimSpace(aabPath))
	if dir == "" || dir == "." {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(aabPath), filepath.Ext(aabPath))
	exact := filepath.Join(dir, base+extBin)
	if _, err := os.Stat(exact); err == nil {
		return exact
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var bestPath string
	var bestMod time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), extBin) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		candidate := filepath.Join(dir, entry.Name())
		if bestPath == "" || info.ModTime().After(bestMod) {
			bestPath = candidate
			bestMod = info.ModTime()
		}
	}
	return bestPath
}

func inferArtifactVariant(ctx context.Context, relPath string, variants []Variant) string {
	_ = ctx
	lowerPath := strings.ToLower(filepath.ToSlash(relPath))
	best := ""
	for _, v := range variants {
		lowerName := strings.ToLower(v.Name)
		if strings.Contains(lowerPath, "/"+lowerName+"/") ||
			strings.Contains(lowerPath, "-"+lowerName+".") ||
			strings.Contains(lowerPath, "-"+lowerName+"-") {
			if len(v.Name) > len(best) {
				best = v.Name
			}
			continue
		}
		if len(v.Flavors) > 0 {
			flavorPath := strings.ToLower(strings.Join(v.Flavors, "/")) + "/" + strings.ToLower(v.BuildType)
			if strings.Contains(lowerPath, flavorPath) && len(v.Name) > len(best) {
				best = v.Name
			}
		}
	}
	return best
}
