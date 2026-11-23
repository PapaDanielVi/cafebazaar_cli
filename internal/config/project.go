package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// ProjectConfigFileName is the non-secret project config placed in an Android repo root.
	ProjectConfigFileName = ".bazaar.yaml"
	// DefaultTagPrefix is the default Git tag prefix for Android releases.
	DefaultTagPrefix = "v"
	// DefaultCommitTemplate formats automated Gradle version bump commit messages.
	DefaultCommitTemplate = "chore(release): bump version to v%s (%d)"

	projectFilePerm = 0o644
)

// ProjectConfig defines non-secret per-repository settings stored in .bazaar.yaml.
type ProjectConfig struct {
	App             string            `yaml:"app,omitempty"`
	Module          string            `yaml:"module,omitempty"`
	DefaultVariant  string            `yaml:"default_variant,omitempty"`
	Apps            map[string]string `yaml:"apps,omitempty"`
	TagPrefix       string            `yaml:"tag_prefix,omitempty"`
	CommitTemplate  string            `yaml:"commit_template,omitempty"`
	AutoPublish     *bool             `yaml:"auto_publish,omitempty"`
	StagedRollout   int               `yaml:"staged_rollout,omitempty"`
	BundleSignerCmd string            `yaml:"bundle_signer_cmd,omitempty"`
}

// FindProjectConfig walks upward from startDir looking for .bazaar.yaml.
func FindProjectConfig(ctx context.Context, startDir string) (*ProjectConfig, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("find project config cancelled: %w", err)
	}
	dir, err := filepath.Abs(strings.TrimSpace(startDir))
	if err != nil {
		return nil, "", fmt.Errorf("resolve directory %q: %w", startDir, err)
	}

	for {
		candidate := filepath.Join(dir, ProjectConfigFileName)
		raw, readErr := os.ReadFile(candidate)
		if readErr == nil {
			var cfg ProjectConfig
			if len(raw) > 0 {
				if err := yaml.Unmarshal(raw, &cfg); err != nil {
					return nil, "", fmt.Errorf("parse project config %q: %w", candidate, err)
				}
			}
			applyProjectDefaults(ctx, &cfg)
			return &cfg, candidate, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return nil, "", fmt.Errorf("read project config %q: %w", candidate, readErr)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, "", nil
		}
		dir = parent
	}
}

// SaveProjectConfig writes .bazaar.yaml inside dir.
func SaveProjectConfig(ctx context.Context, dir string, cfg *ProjectConfig) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("save project config cancelled: %w", err)
	}
	if cfg == nil {
		return "", errors.New("project config must not be nil")
	}
	applyProjectDefaults(ctx, cfg)
	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("encode project config yaml: %w", err)
	}
	target := filepath.Join(dir, ProjectConfigFileName)
	if err := os.WriteFile(target, encoded, projectFilePerm); err != nil {
		return "", fmt.Errorf("write project config %q: %w", target, err)
	}
	return target, nil
}

// ResolveVariantPackage returns the package mapped to variant or falls back to the project default app.
func (p *ProjectConfig) ResolveVariantPackage(ctx context.Context, variant string) string {
	_ = ctx
	if p == nil {
		return ""
	}
	v := strings.TrimSpace(variant)
	if v != "" && len(p.Apps) > 0 {
		if pkg, ok := p.Apps[v]; ok && strings.TrimSpace(pkg) != "" {
			return strings.TrimSpace(pkg)
		}
		for k, pkg := range p.Apps {
			if strings.EqualFold(k, v) && strings.TrimSpace(pkg) != "" {
				return strings.TrimSpace(pkg)
			}
		}
	}
	return strings.TrimSpace(p.App)
}

func applyProjectDefaults(ctx context.Context, cfg *ProjectConfig) {
	_ = ctx
	if cfg.TagPrefix == "" {
		cfg.TagPrefix = DefaultTagPrefix
	}
	if cfg.CommitTemplate == "" {
		cfg.CommitTemplate = DefaultCommitTemplate
	}
}
