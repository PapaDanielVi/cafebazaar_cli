package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// AppDirName is the configuration directory name under ~/.config.
	AppDirName = "bazaar"
	// GlobalConfigFileName is the global CLI metadata file name.
	GlobalConfigFileName = "config.yaml"
	// CredentialsFileName is the fallback 0600 token file name.
	CredentialsFileName = "credentials.yaml"

	// TokenSourceManual indicates a per-app token pasted from the Pishkhan dashboard.
	TokenSourceManual = "manual"
	// TokenSourcePublisherAPI indicates a per-app token returned by POST /v1/apps/.
	TokenSourcePublisherAPI = "publisher_api"

	// DefaultRTLMode selects automatic terminal BiDi/shaping detection.
	DefaultRTLMode = "auto"

	configDirPerm  = 0o700
	configFilePerm = 0o600
	tempSuffix     = ".tmp"
)

// PublisherConfig stores non-secret metadata for the developer Nasher account token.
type PublisherConfig struct {
	Account    string    `yaml:"account,omitempty"`
	AuthHeader string    `yaml:"auth_header,omitempty"`
	BaseURL    string    `yaml:"base_url,omitempty"`
	Backend    string    `yaml:"backend,omitempty"`
	AddedAt    time.Time `yaml:"added_at,omitempty"`
}

// AppEntry stores non-secret metadata and local directory bindings for a registered app.
type AppEntry struct {
	PackageName    string    `yaml:"package_name"`
	Alias          string    `yaml:"alias,omitempty"`
	DisplayName    string    `yaml:"display_name,omitempty"`
	TokenSource    string    `yaml:"token_source,omitempty"`
	Backend        string    `yaml:"backend,omitempty"`
	DefaultVariant string    `yaml:"default_variant,omitempty"`
	Paths          []string  `yaml:"paths,omitempty"`
	AddedAt        time.Time `yaml:"added_at,omitempty"`
}

// GlobalConfig represents ~/.config/bazaar/config.yaml.
type GlobalConfig struct {
	DefaultApp string              `yaml:"default_app,omitempty"`
	RTLMode    string              `yaml:"rtl_mode,omitempty"`
	Publisher  *PublisherConfig    `yaml:"publisher,omitempty"`
	Apps       map[string]AppEntry `yaml:"apps,omitempty"`
}

// DefaultConfigDir returns the resolved configuration directory for bazaar.
func DefaultConfigDir(ctx context.Context) (string, error) {
	_ = ctx
	if xdg := strings.TrimSpace(os.Getenv(EnvXDGConfigHome)); xdg != "" {
		return filepath.Join(xdg, AppDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".config", AppDirName), nil
}

// DefaultConfigPath returns the default global config.yaml path.
func DefaultConfigPath(ctx context.Context) (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvConfig)); override != "" {
		return override, nil
	}
	dir, err := DefaultConfigDir(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, GlobalConfigFileName), nil
}

// CredentialsPathForConfig derives the sibling credentials.yaml path from a config file path.
func CredentialsPathForConfig(ctx context.Context, configPath string) string {
	_ = ctx
	return filepath.Join(filepath.Dir(configPath), CredentialsFileName)
}

// LoadGlobal reads and parses GlobalConfig from path, returning an initialized empty config if missing.
func LoadGlobal(ctx context.Context, path string) (*GlobalConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("load global config cancelled: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &GlobalConfig{
				RTLMode: DefaultRTLMode,
				Apps:    map[string]AppEntry{},
			}, nil
		}
		return nil, fmt.Errorf("read global config %q: %w", path, err)
	}

	var cfg GlobalConfig
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("parse global config %q: %w", path, err)
		}
	}
	if cfg.Apps == nil {
		cfg.Apps = map[string]AppEntry{}
	}
	if cfg.RTLMode == "" {
		cfg.RTLMode = DefaultRTLMode
	}
	return &cfg, nil
}

// SaveGlobal atomically writes GlobalConfig to path with restricted permissions.
func SaveGlobal(ctx context.Context, path string, cfg *GlobalConfig) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("save global config cancelled: %w", err)
	}
	if cfg == nil {
		return errors.New("global config must not be nil")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, configDirPerm); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	encoded, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode global config yaml: %w", err)
	}

	tmpPath := path + tempSuffix
	if err := os.WriteFile(tmpPath, encoded, configFilePerm); err != nil {
		return fmt.Errorf("write temporary global config: %w", err)
	}
	if err := os.Chmod(tmpPath, configFilePerm); err != nil {
		return fmt.Errorf("chmod temporary global config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace global config: %w", err)
	}
	return nil
}

// FindApp looks up a registered app by exact package name or case-insensitive alias.
func (c *GlobalConfig) FindApp(ctx context.Context, pkgOrAlias string) (AppEntry, bool) {
	_ = ctx
	query := strings.TrimSpace(pkgOrAlias)
	if query == "" || c == nil {
		return AppEntry{}, false
	}
	if entry, ok := c.Apps[query]; ok {
		return entry, true
	}
	for _, entry := range c.Apps {
		if entry.Alias != "" && strings.EqualFold(entry.Alias, query) {
			return entry, true
		}
	}
	return AppEntry{}, false
}

// BindPath links dirPath to pkg so navigating inside dirPath resolves to pkg automatically.
func (c *GlobalConfig) BindPath(ctx context.Context, pkg, dirPath string) error {
	_ = ctx
	entry, ok := c.Apps[pkg]
	if !ok {
		return fmt.Errorf("app %q is not registered", pkg)
	}
	absDir, err := normalizeDirPath(ctx, dirPath)
	if err != nil {
		return err
	}

	for otherPkg, other := range c.Apps {
		if otherPkg == pkg {
			continue
		}
		filtered := make([]string, 0, len(other.Paths))
		for _, p := range other.Paths {
			if p != absDir {
				filtered = append(filtered, p)
			}
		}
		if len(filtered) != len(other.Paths) {
			other.Paths = filtered
			c.Apps[otherPkg] = other
		}
	}

	for _, existing := range entry.Paths {
		if existing == absDir {
			return nil
		}
	}
	entry.Paths = append(entry.Paths, absDir)
	sort.Strings(entry.Paths)
	c.Apps[pkg] = entry
	return nil
}

// MatchBoundPath returns the app bound to cwd or its nearest ancestor directory.
func (c *GlobalConfig) MatchBoundPath(ctx context.Context, cwd string) (AppEntry, bool, error) {
	if c == nil || strings.TrimSpace(cwd) == "" {
		return AppEntry{}, false, nil
	}
	absCwd, err := normalizeDirPath(ctx, cwd)
	if err != nil {
		return AppEntry{}, false, err
	}

	var bestEntry AppEntry
	bestLen := -1
	for _, entry := range c.Apps {
		for _, bound := range entry.Paths {
			cleanBound := filepath.Clean(bound)
			if isPathWithin(ctx, absCwd, cleanBound) && len(cleanBound) > bestLen {
				bestLen = len(cleanBound)
				bestEntry = entry
			}
		}
	}
	if bestLen < 0 {
		return AppEntry{}, false, nil
	}
	return bestEntry, true, nil
}

// SortedApps returns all registered app entries sorted alphabetically by package name.
func (c *GlobalConfig) SortedApps(ctx context.Context) []AppEntry {
	_ = ctx
	if c == nil || len(c.Apps) == 0 {
		return nil
	}
	keys := make([]string, 0, len(c.Apps))
	for k := range c.Apps {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]AppEntry, 0, len(keys))
	for _, k := range keys {
		result = append(result, c.Apps[k])
	}
	return result
}

func normalizeDirPath(ctx context.Context, dir string) (string, error) {
	_ = ctx
	abs, err := filepath.Abs(strings.TrimSpace(dir))
	if err != nil {
		return "", fmt.Errorf("resolve directory %q: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	return filepath.Clean(abs), nil
}

func isPathWithin(ctx context.Context, target, base string) bool {
	_ = ctx
	if target == base {
		return true
	}
	prefix := base + string(filepath.Separator)
	return strings.HasPrefix(target, prefix)
}
