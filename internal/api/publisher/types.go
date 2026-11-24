package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultBaseURL is the default base URL for CafeBazaar's Nasher Publisher API.
	DefaultBaseURL = "https://api.pishkhan.cafebazaar.ir"
	// EndpointApps creates a new app and submits its initial version for review.
	EndpointApps = "/v1/apps/"

	// FieldPackageName is the request field carrying the Android package name.
	FieldPackageName = "package_name"
	// FieldCategoryID is the required CafeBazaar integer category identifier.
	FieldCategoryID = "category_id"
	// FieldNameFA is the required Persian app display name (max 36 characters).
	FieldNameFA = "name_fa"
	// FieldNameEN is the optional English app display name (max 36 characters).
	FieldNameEN = "name_en"
	// FieldShortDescriptionFA is the required Persian short description (max 126 characters).
	FieldShortDescriptionFA = "short_description_fa"
	// FieldShortDescriptionEN is the optional English short description (max 126 characters).
	FieldShortDescriptionEN = "short_description_en"
	// FieldDescriptionFA is the required Persian full app description.
	FieldDescriptionFA = "description_fa"
	// FieldDescriptionEN is the optional English full app description.
	FieldDescriptionEN = "description_en"
	// FieldEmail is the required developer support email address.
	FieldEmail = "email"
	// FieldPhone is the optional developer support phone number.
	FieldPhone = "phone"
	// FieldWebsite is the optional app or developer website URL.
	FieldWebsite = "website"
	// FieldIcon is the required square app icon multipart file field (min 512x512).
	FieldIcon = "icon"
	// FieldScreenshots is the required repeated multipart file field for app screenshots (1 to 12 files).
	FieldScreenshots = "screenshots"
	// FieldAutoPublish controls whether CafeBazaar publishes the app automatically upon approval.
	FieldAutoPublish = "auto_publish"
	// FieldIssueAPIAgent instructs CafeBazaar to issue a per-app Pishkhan token on creation.
	FieldIssueAPIAgent = "issue_api_agent"
	// FieldAPK is the multipart file field name used when uploading an initial APK.
	FieldAPK = "apk"
	// FieldAAB is the multipart file field name used when uploading an initial AAB.
	FieldAAB = "aab"

	// MaxAppNameRunes is the maximum character length for name_fa and name_en.
	MaxAppNameRunes = 36
	// MaxShortDescRunes is the maximum character length for short_description_fa and short_description_en.
	MaxShortDescRunes = 126
	// MinIconDimension is the minimum width and height (in pixels) for the square app icon.
	MinIconDimension = 512
	// MinScreenshotCount is the minimum number of screenshots required to submit a new app.
	MinScreenshotCount = 1
	// MaxScreenshotCount is the maximum number of screenshots allowed by CafeBazaar.
	MaxScreenshotCount = 12
)

var (
	// ErrInvalidCategoryID indicates that category_id is missing or not a positive integer.
	ErrInvalidCategoryID = errors.New("category_id must be a positive integer")
	// ErrInvalidAppName indicates that name_fa or name_en is missing or exceeds 36 characters.
	ErrInvalidAppName = errors.New("app name must be between 1 and 36 characters")
	// ErrInvalidShortDescription indicates that short_description_fa or short_description_en is missing or exceeds 126 characters.
	ErrInvalidShortDescription = errors.New("short description must be between 1 and 126 characters")
	// ErrMissingDescriptionFA indicates that description_fa was not provided.
	ErrMissingDescriptionFA = errors.New("description_fa must not be empty")
	// ErrInvalidEmail indicates that email is missing or not a valid email address.
	ErrInvalidEmail = errors.New("email must be a valid email address")
	// ErrMissingArtifact indicates that no initial APK or AAB file path was provided.
	ErrMissingArtifact = errors.New("initial APK or AAB file path is required")
	// ErrInvalidIcon indicates that the app icon is missing, unreadable, non-square, or smaller than 512x512.
	ErrInvalidIcon = errors.New("icon must be a square image of at least 512x512 pixels")
	// ErrInvalidScreenshots indicates that screenshots are missing or exceed 12 files.
	ErrInvalidScreenshots = errors.New("screenshots must include between 1 and 12 image files")
)

// CreateAppRequest defines parameters for initial app submission via the Nasher Publisher API.
type CreateAppRequest struct {
	PackageName        string            `json:"package_name,omitempty"`
	CategoryID         int               `json:"category_id"`
	NameFA             string            `json:"name_fa"`
	NameEN             string            `json:"name_en,omitempty"`
	ShortDescriptionFA string            `json:"short_description_fa"`
	ShortDescriptionEN string            `json:"short_description_en,omitempty"`
	DescriptionFA      string            `json:"description_fa"`
	DescriptionEN      string            `json:"description_en,omitempty"`
	Email              string            `json:"email"`
	Phone              string            `json:"phone,omitempty"`
	Website            string            `json:"website,omitempty"`
	IconPath           string            `json:"-"`
	ScreenshotPaths    []string          `json:"-"`
	AutoPublish        *bool             `json:"auto_publish,omitempty"`
	IssueAPIAgent      bool              `json:"issue_api_agent"`
	ArtifactPath       string            `json:"-"`
	ArtifactField      string            `json:"-"`
	ExtraFields        map[string]string `json:"-"`
}

// Validate checks all required CafeBazaar Nasher app creation fields and local image constraints.
func (r CreateAppRequest) Validate(ctx context.Context) error {
	if r.CategoryID <= 0 {
		return ErrInvalidCategoryID
	}
	if err := ValidateAppName(ctx, r.NameFA, true); err != nil {
		return fmt.Errorf("name_fa: %w", err)
	}
	if err := ValidateAppName(ctx, r.NameEN, false); err != nil {
		return fmt.Errorf("name_en: %w", err)
	}
	if err := ValidateShortDescription(ctx, r.ShortDescriptionFA, true); err != nil {
		return fmt.Errorf("short_description_fa: %w", err)
	}
	if err := ValidateShortDescription(ctx, r.ShortDescriptionEN, false); err != nil {
		return fmt.Errorf("short_description_en: %w", err)
	}
	if strings.TrimSpace(r.DescriptionFA) == "" {
		return ErrMissingDescriptionFA
	}
	if err := ValidateEmail(ctx, r.Email); err != nil {
		return err
	}
	if strings.TrimSpace(r.ArtifactPath) == "" {
		return ErrMissingArtifact
	}
	if err := ValidateRegularFile(ctx, r.ArtifactPath); err != nil {
		return fmt.Errorf("artifact: %w", err)
	}
	if err := ValidateIconFile(ctx, r.IconPath); err != nil {
		return err
	}
	if err := ValidateScreenshotPaths(ctx, r.ScreenshotPaths); err != nil {
		return err
	}
	return nil
}

// ValidateAppName checks that name is present when required and at most 36 characters.
func ValidateAppName(ctx context.Context, name string, required bool) error {
	_ = ctx
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		if required {
			return ErrInvalidAppName
		}
		return nil
	}
	if utf8.RuneCountInString(trimmed) > MaxAppNameRunes {
		return fmt.Errorf("%w (got %d characters)", ErrInvalidAppName, utf8.RuneCountInString(trimmed))
	}
	return nil
}

// ValidateShortDescription checks that text is present when required and at most 126 characters.
func ValidateShortDescription(ctx context.Context, text string, required bool) error {
	_ = ctx
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		if required {
			return ErrInvalidShortDescription
		}
		return nil
	}
	if utf8.RuneCountInString(trimmed) > MaxShortDescRunes {
		return fmt.Errorf("%w (got %d characters)", ErrInvalidShortDescription, utf8.RuneCountInString(trimmed))
	}
	return nil
}

// ValidateEmail checks that raw is a syntactically valid email address.
func ValidateEmail(ctx context.Context, raw string) error {
	_ = ctx
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr.Address != trimmed {
		return ErrInvalidEmail
	}
	_, domain, ok := strings.Cut(trimmed, "@")
	if !ok || !strings.Contains(domain, ".") {
		return ErrInvalidEmail
	}
	return nil
}

// ValidateRegularFile checks that path exists and is not a directory.
func ValidateRegularFile(ctx context.Context, path string) error {
	_ = ctx
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return errors.New("file path must not be empty")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return fmt.Errorf("stat file %q: %w", trimmed, err)
	}
	if info.IsDir() {
		return fmt.Errorf("path %q is a directory, not a file", trimmed)
	}
	return nil
}

// ValidateIconFile verifies that iconPath exists and is a square image of at least 512x512 pixels.
func ValidateIconFile(ctx context.Context, iconPath string) error {
	trimmed := strings.TrimSpace(iconPath)
	if trimmed == "" {
		return ErrInvalidIcon
	}
	if err := ValidateRegularFile(ctx, trimmed); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidIcon, err)
	}
	file, err := os.Open(trimmed)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidIcon, err)
	}
	cfg, _, decodeErr := image.DecodeConfig(file)
	closeErr := file.Close()
	if closeErr != nil {
		return fmt.Errorf("close icon file %q: %w", trimmed, closeErr)
	}
	if decodeErr != nil {
		// Allow WebP icons whose header isn't decoded by Go's stdlib image decoders.
		if strings.EqualFold(filepath.Ext(trimmed), ".webp") {
			return nil
		}
		return fmt.Errorf("%w: %w", ErrInvalidIcon, decodeErr)
	}
	if cfg.Width != cfg.Height || cfg.Width < MinIconDimension || cfg.Height < MinIconDimension {
		return fmt.Errorf("%w (got %dx%d)", ErrInvalidIcon, cfg.Width, cfg.Height)
	}
	return nil
}

// ValidateScreenshotPaths verifies that between 1 and 12 readable screenshot image paths are supplied.
func ValidateScreenshotPaths(ctx context.Context, paths []string) error {
	if len(paths) < MinScreenshotCount || len(paths) > MaxScreenshotCount {
		return fmt.Errorf("%w (got %d)", ErrInvalidScreenshots, len(paths))
	}
	for _, p := range paths {
		if err := ValidateRegularFile(ctx, p); err != nil {
			return fmt.Errorf("screenshot %q: %w", p, err)
		}
	}
	return nil
}

// CreateAppResponse holds the decoded response from POST /v1/apps/ along with any issued app token.
type CreateAppResponse struct {
	Type        string         `json:"type,omitempty"`
	Message     string         `json:"message,omitempty"`
	PackageName string         `json:"package_name,omitempty"`
	VersionCode int            `json:"version_code,omitempty"`
	VersionName string         `json:"version_name,omitempty"`
	AppURL      string         `json:"app_url,omitempty"`
	ReviewState string         `json:"review_state,omitempty"`
	AutoPublish bool           `json:"auto_publish,omitempty"`
	AppToken    string         `json:"app_token,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
	Raw         map[string]any `json:"raw,omitempty"`
}

// ParseCreateAppResponse extracts known metadata, warnings, and any issued app token from raw JSON.
func ParseCreateAppResponse(ctx context.Context, raw map[string]any) CreateAppResponse {
	_ = ctx
	resp := CreateAppResponse{
		Raw: raw,
	}
	if raw == nil {
		return resp
	}

	resp.Type = stringField(ctx, raw, "type")
	resp.Message = stringField(ctx, raw, "message")
	resp.PackageName = stringField(ctx, raw, FieldPackageName)
	resp.VersionCode = intField(ctx, raw, "version_code")
	resp.VersionName = stringField(ctx, raw, "version_name")
	resp.AppURL = stringField(ctx, raw, "app_url")
	resp.ReviewState = stringField(ctx, raw, "review_state")
	resp.AutoPublish = boolField(ctx, raw, FieldAutoPublish)
	resp.Warnings = extractWarnings(ctx, raw)
	resp.AppToken = extractAppToken(ctx, raw)
	return resp
}

func extractAppToken(ctx context.Context, raw map[string]any) string {
	_ = ctx
	topLevelKeys := []string{
		"api_agent_token",
		"api_secret",
		"app_token",
		"api_token",
		"token",
		"secret",
	}
	for _, key := range topLevelKeys {
		if v := stringField(ctx, raw, key); v != "" {
			return v
		}
	}

	nestedContainers := []string{"api_agent", "agent", "app", "data"}
	for _, containerKey := range nestedContainers {
		nested, ok := raw[containerKey].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range topLevelKeys {
			if v := stringField(ctx, nested, key); v != "" {
				return v
			}
		}
	}
	return ""
}

func extractWarnings(ctx context.Context, raw map[string]any) []string {
	_ = ctx
	var warnings []string
	if single := stringField(ctx, raw, "warning"); single != "" {
		warnings = append(warnings, single)
	}
	if list, ok := raw["warnings"].([]any); ok {
		for _, item := range list {
			switch val := item.(type) {
			case string:
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					warnings = append(warnings, trimmed)
				}
			default:
				encoded, err := json.Marshal(val)
				if err == nil && len(encoded) > 0 {
					warnings = append(warnings, string(encoded))
				}
			}
		}
	}
	return warnings
}

func stringField(ctx context.Context, m map[string]any, key string) string {
	_ = ctx
	val, ok := m[key]
	if !ok || val == nil {
		return ""
	}
	switch typed := val.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func intField(ctx context.Context, m map[string]any, key string) int {
	_ = ctx
	val, ok := m[key]
	if !ok || val == nil {
		return 0
	}
	switch typed := val.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return 0
}

func boolField(ctx context.Context, m map[string]any, key string) bool {
	_ = ctx
	val, ok := m[key]
	if !ok || val == nil {
		return false
	}
	switch typed := val.(type) {
	case bool:
		return typed
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(typed))
		return err == nil && b
	}
	return false
}
