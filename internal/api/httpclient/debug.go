package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const (
	// RedactedValue replaces secret tokens, auth headers, and credentials in verbose diagnostics.
	RedactedValue = "[REDACTED]"

	maxLoggedBodyRunes = 16384
)

// DebugLogFunc receives sanitized diagnostic lines when verbose logging is enabled.
type DebugLogFunc func(ctx context.Context, message string)

var sensitiveKeyTokens = []string{
	"token",
	"secret",
	"password",
	"passwd",
	"authorization",
	"api_key",
	"api-key",
	"apikey",
	"private_key",
	"private-key",
	"access_key",
	"access-key",
}

var sensitiveHeaderNames = []string{
	HeaderPishkhanSecret,
	"Authorization",
	"Proxy-Authorization",
	"Cookie",
	"Set-Cookie",
}

// SanitizeBody redacts token and secret fields in JSON bodies and scrubs literal secret values.
func SanitizeBody(ctx context.Context, body []byte, secrets []string) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}

	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err == nil {
		cleaned := sanitizeJSONValue(ctx, decoded, secrets)
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if encErr := enc.Encode(cleaned); encErr == nil {
			out := strings.TrimRight(buf.String(), "\n")
			out = ScrubSecrets(ctx, out, secrets)
			return truncateDebugRunes(ctx, out, len(trimmed))
		}
	}

	out := ScrubSecrets(ctx, string(trimmed), secrets)
	return truncateDebugRunes(ctx, out, len(trimmed))
}

// SanitizeURL redacts embedded passwords, sensitive query parameters, and literal secret values from a URL.
func SanitizeURL(ctx context.Context, rawURL string, secrets []string) string {
	masked := MaskProxyURL(ctx, rawURL)
	parsed, err := url.Parse(masked)
	if err != nil {
		return ScrubSecrets(ctx, masked, secrets)
	}
	if parsed.RawQuery != "" {
		q := parsed.Query()
		changed := false
		for key, vals := range q {
			if isSensitiveName(ctx, key) {
				q.Set(key, RedactedValue)
				changed = true
				continue
			}
			for i, v := range vals {
				scrubbed := ScrubSecrets(ctx, v, secrets)
				if scrubbed != v {
					vals[i] = scrubbed
					changed = true
				}
			}
			q[key] = vals
		}
		if changed {
			parsed.RawQuery = q.Encode()
		}
	}
	return ScrubSecrets(ctx, parsed.String(), secrets)
}

// ScrubSecrets replaces any literal occurrence of non-empty secret strings with RedactedValue.
func ScrubSecrets(ctx context.Context, input string, secrets []string) string {
	_ = ctx
	out := input
	for _, secret := range secrets {
		s := strings.TrimSpace(secret)
		if s == "" {
			continue
		}
		out = strings.ReplaceAll(out, s, RedactedValue)
	}
	return out
}

func (c *Client) logDebug(ctx context.Context, format string, args ...any) {
	if c == nil || c.debugLog == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	msg = ScrubSecrets(ctx, msg, c.secretValues(ctx))
	c.debugLog(ctx, msg)
}

func (c *Client) secretValues(ctx context.Context) []string {
	_ = ctx
	if c == nil || strings.TrimSpace(c.token) == "" {
		return nil
	}
	return []string{c.token}
}

func (c *Client) formatHeadersForLog(ctx context.Context, h http.Header) string {
	if len(h) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	secrets := c.secretValues(ctx)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		vals := h.Values(k)
		if c.isSensitiveHeader(ctx, k) {
			parts = append(parts, fmt.Sprintf("%s=%s", k, RedactedValue))
			continue
		}
		cleanVals := make([]string, len(vals))
		for i, v := range vals {
			cleanVals[i] = ScrubSecrets(ctx, v, secrets)
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, strings.Join(cleanVals, ", ")))
	}
	return strings.Join(parts, "; ")
}

func (c *Client) formatFieldsForLog(ctx context.Context, fields map[string]string) string {
	if len(fields) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	secrets := c.secretValues(ctx)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if isSensitiveName(ctx, k) {
			parts = append(parts, fmt.Sprintf("%s=%s", k, RedactedValue))
			continue
		}
		val := ScrubSecrets(ctx, fields[k], secrets)
		parts = append(parts, fmt.Sprintf("%s=%q", k, val))
	}
	return strings.Join(parts, ", ")
}

func (c *Client) isSensitiveHeader(ctx context.Context, name string) bool {
	if c != nil && c.authHeaderName != "" && strings.EqualFold(name, c.authHeaderName) {
		return true
	}
	for _, known := range sensitiveHeaderNames {
		if strings.EqualFold(name, known) {
			return true
		}
	}
	return isSensitiveName(ctx, name)
}

func isSensitiveName(ctx context.Context, name string) bool {
	_ = ctx
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return false
	}
	for _, marker := range sensitiveKeyTokens {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sanitizeJSONValue(ctx context.Context, v any, secrets []string) any {
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, val := range typed {
			if isSensitiveName(ctx, k) {
				switch nested := val.(type) {
				case map[string]any, []any:
					out[k] = sanitizeJSONValue(ctx, nested, secrets)
				case nil:
					out[k] = nil
				default:
					out[k] = RedactedValue
				}
				continue
			}
			out[k] = sanitizeJSONValue(ctx, val, secrets)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sanitizeJSONValue(ctx, item, secrets)
		}
		return out
	case string:
		return ScrubSecrets(ctx, typed, secrets)
	default:
		return v
	}
}

func truncateDebugRunes(ctx context.Context, s string, origBytes int) string {
	_ = ctx
	runes := []rune(s)
	if len(runes) <= maxLoggedBodyRunes {
		return s
	}
	return fmt.Sprintf("%s... (truncated, %d bytes total)", string(runes[:maxLoggedBodyRunes]), origBytes)
}
