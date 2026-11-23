package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	// DefaultBaseURL is the default base URL for CafeBazaar's Nasher Publisher API.
	DefaultBaseURL = "https://api.pishkhan.cafebazaar.ir"
	// EndpointApps creates a new app and submits its initial version for review.
	EndpointApps = "/v1/apps/"

	// FieldPackageName is the request field carrying the Android package name.
	FieldPackageName = "package_name"
	// FieldIssueAPIAgent instructs CafeBazaar to issue a per-app Pishkhan token on creation.
	FieldIssueAPIAgent = "issue_api_agent"
	// FieldAPK is the multipart file field name used when uploading an initial APK.
	FieldAPK = "apk"
	// FieldAAB is the multipart file field name used when uploading an initial AAB.
	FieldAAB = "aab"
)

// CreateAppRequest defines parameters for initial app submission via the Nasher Publisher API.
type CreateAppRequest struct {
	PackageName   string            `json:"package_name,omitempty"`
	IssueAPIAgent bool              `json:"issue_api_agent"`
	ArtifactPath  string            `json:"-"`
	ArtifactField string            `json:"-"`
	ExtraFields   map[string]string `json:"-"`
}

// CreateAppResponse holds the decoded response from POST /v1/apps/ along with any issued app token.
type CreateAppResponse struct {
	Type        string         `json:"type,omitempty"`
	Message     string         `json:"message,omitempty"`
	PackageName string         `json:"package_name,omitempty"`
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
