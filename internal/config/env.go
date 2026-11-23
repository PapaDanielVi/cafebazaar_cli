package config

import (
	"context"
	"os"
	"strconv"
	"strings"
)

const (
	// EnvPublisherToken supplies the developer-level Nasher token for CI/CD.
	EnvPublisherToken = "CAFEBAZAAR_PUBLISHER_TOKEN"
	// EnvAppToken supplies the per-app Pishkhan token directly for CI/CD.
	EnvAppToken = "CAFEBAZAAR_APP_TOKEN"
	// EnvApp selects a registered package name or alias.
	EnvApp = "CAFEBAZAAR_APP"
	// EnvVariant selects a Gradle build variant.
	EnvVariant = "CAFEBAZAAR_VARIANT"
	// EnvArch selects the APK architecture enum for uploads.
	EnvArch = "CAFEBAZAAR_ARCH"
	// EnvAutoPublish controls auto_publish during commit/publish.
	EnvAutoPublish = "CAFEBAZAAR_AUTO_PUBLISH"
	// EnvRollout controls staged_rollout_percentage during commit/publish.
	EnvRollout = "CAFEBAZAAR_ROLLOUT"
	// EnvChangelogFA provides Persian release notes via environment variable.
	EnvChangelogFA = "CAFEBAZAAR_CHANGELOG_FA"
	// EnvChangelogEN provides English release notes via environment variable.
	EnvChangelogEN = "CAFEBAZAAR_CHANGELOG_EN"
	// EnvDevNote provides reviewer developer notes via environment variable.
	EnvDevNote = "CAFEBAZAAR_DEV_NOTE"
	// EnvYes bypasses dangerous-action confirmation prompts in CI/CD.
	EnvYes = "CAFEBAZAAR_YES"
	// EnvNoUI disables Charmbracelet interactive UI components.
	EnvNoUI = "BAZAAR_NO_UI"
	// EnvPlain enables headless plain CLI output.
	EnvPlain = "BAZAAR_PLAIN"
	// EnvCI detects standard CI environments.
	EnvCI = "CI"
	// EnvOutput sets the default output format (table, plain, json, yaml).
	EnvOutput = "BAZAAR_OUTPUT"
	// EnvRTL sets Persian terminal shaping mode (auto, on, off).
	EnvRTL = "BAZAAR_RTL"
	// EnvNoKeyring skips OS keyring access and uses the 0600 file store.
	EnvNoKeyring = "BAZAAR_NO_KEYRING"
	// EnvConfig overrides the global config file path.
	EnvConfig = "BAZAAR_CONFIG"
	// EnvProxy configures an HTTP, HTTPS, or SOCKS5 proxy exclusively for bazaar.
	EnvProxy = "BAZAAR_PROXY"
	// EnvCafeBazaarProxy is an alias for BAZAAR_PROXY for CI pipelines using CAFEBAZAAR_* naming.
	EnvCafeBazaarProxy = "CAFEBAZAAR_PROXY"
	// EnvNoColor disables ANSI color output.
	EnvNoColor = "NO_COLOR"
	// EnvXDGConfigHome overrides the base XDG configuration directory.
	EnvXDGConfigHome = "XDG_CONFIG_HOME"
)

// EnvSettings captures all bazaar and CafeBazaar environment variable overrides.
type EnvSettings struct {
	PublisherToken string
	AppToken       string
	App            string
	Variant        string
	Arch           string
	AutoPublish    *bool
	Rollout        int
	ChangelogFA    string
	ChangelogEN    string
	DevNote        string
	Yes            bool
	NoUI           bool
	Plain          bool
	CI             bool
	Output         string
	RTL            string
	NoKeyring      bool
	ConfigPath     string
	Proxy          string
	NoColor        bool
}

// ReadEnv reads all supported environment variables using lookup.
func ReadEnv(ctx context.Context, lookup func(string) (string, bool)) EnvSettings {
	_ = ctx
	if lookup == nil {
		lookup = os.LookupEnv
	}

	get := func(key string) string {
		val, _ := lookup(key)
		return strings.TrimSpace(val)
	}
	getBool := func(key string) bool {
		return parseTruthy(ctx, get(key))
	}

	settings := EnvSettings{
		PublisherToken: get(EnvPublisherToken),
		AppToken:       get(EnvAppToken),
		App:            get(EnvApp),
		Variant:        get(EnvVariant),
		Arch:           get(EnvArch),
		ChangelogFA:    get(EnvChangelogFA),
		ChangelogEN:    get(EnvChangelogEN),
		DevNote:        get(EnvDevNote),
		Yes:            getBool(EnvYes),
		NoUI:           getBool(EnvNoUI),
		Plain:          getBool(EnvPlain),
		CI:             getBool(EnvCI),
		Output:         strings.ToLower(get(EnvOutput)),
		RTL:            strings.ToLower(get(EnvRTL)),
		NoKeyring:      getBool(EnvNoKeyring),
		ConfigPath:     get(EnvConfig),
		Proxy:          firstNonEmpty(ctx, get(EnvProxy), get(EnvCafeBazaarProxy)),
		NoColor:        isSetNonEmpty(ctx, lookup, EnvNoColor),
	}

	if rawAuto, ok := lookup(EnvAutoPublish); ok && strings.TrimSpace(rawAuto) != "" {
		parsed := parseTruthy(ctx, rawAuto)
		settings.AutoPublish = &parsed
	}
	if rawRollout := get(EnvRollout); rawRollout != "" {
		if n, err := strconv.Atoi(rawRollout); err == nil {
			settings.Rollout = n
		}
	}

	return settings
}

func firstNonEmpty(ctx context.Context, values ...string) string {
	_ = ctx
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isSetNonEmpty(ctx context.Context, lookup func(string) (string, bool), key string) bool {
	_ = ctx
	v, ok := lookup(key)
	return ok && strings.TrimSpace(v) != ""
}

func parseTruthy(ctx context.Context, raw string) bool {
	_ = ctx
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
