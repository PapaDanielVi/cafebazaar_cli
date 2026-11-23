package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

const (
	// HeaderPishkhanSecret is the default CafeBazaar API authentication header name.
	HeaderPishkhanSecret = "CAFEBAZAAR-PISHKHAN-API-SECRET"
	// HeaderContentType is the standard HTTP Content-Type header name.
	HeaderContentType = "Content-Type"
	// HeaderUserAgent is the standard HTTP User-Agent header name.
	HeaderUserAgent = "User-Agent"
	// HeaderRetryAfter is the standard HTTP Retry-After header name.
	HeaderRetryAfter = "Retry-After"
	// ContentTypeJSON is the UTF-8 JSON content type expected by CafeBazaar APIs.
	ContentTypeJSON = "application/json;charset=UTF-8"
	// ContentTypeAPK is the MIME type used when uploading Android APK packages.
	ContentTypeAPK = "application/vnd.android.package-archive"
	// ContentTypeOctetStream is the fallback binary MIME type for AAB and bin uploads.
	ContentTypeOctetStream = "application/octet-stream"
	// DefaultUserAgent identifies the bazaar CLI in outgoing API requests.
	DefaultUserAgent = "bazaar-cli/1.0"
	// ProxyDirect bypasses all environment proxy settings and connects directly.
	ProxyDirect = "direct"
	// ProxyNone bypasses all environment proxy settings and connects directly.
	ProxyNone = "none"
	// ProxySchemeHTTP is the HTTP proxy URL scheme.
	ProxySchemeHTTP = "http"
	// ProxySchemeHTTPS is the HTTPS proxy URL scheme.
	ProxySchemeHTTPS = "https"
	// ProxySchemeSOCKS5 is the SOCKS5 proxy URL scheme.
	ProxySchemeSOCKS5 = "socks5"
	// ProxySchemeSOCKS5H is curl's DNS-over-SOCKS5 proxy URL scheme alias.
	ProxySchemeSOCKS5H = "socks5h"
)

var (
	// ErrEmptyBaseURL indicates that no API base URL was configured.
	ErrEmptyBaseURL = errors.New("api base URL must not be empty")
	// ErrEmptyToken indicates that no API secret token was configured.
	ErrEmptyToken = errors.New("api token must not be empty")
	// ErrEmptyFilePath indicates that no upload file path was provided.
	ErrEmptyFilePath = errors.New("upload file path must not be empty")
	// ErrInvalidProxyURL indicates that a configured proxy URL could not be parsed or uses an unsupported scheme.
	ErrInvalidProxyURL = errors.New("invalid proxy URL")
)

// APIError represents a non-2xx HTTP response returned by a CafeBazaar API.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
	Detail     string
	RawBody    string
}

// Error formats the APIError as a human-readable string.
func (e *APIError) Error() string {
	switch {
	case e.Type != "" && e.Message != "":
		return fmt.Sprintf("api error (status %d, %s): %s", e.StatusCode, e.Type, e.Message)
	case e.Detail != "":
		return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.Detail)
	case e.Message != "":
		return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.Message)
	case e.RawBody != "":
		return fmt.Sprintf("api error (status %d): %s", e.StatusCode, e.RawBody)
	default:
		return fmt.Sprintf("api error (status %d)", e.StatusCode)
	}
}

// IsForbidden reports whether err represents an HTTP 401 or 403 authentication error.
func IsForbidden(ctx context.Context, err error) bool {
	_ = ctx
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden
}

// IsRateLimited reports whether err represents an HTTP 429 rate-limit error.
func IsRateLimited(ctx context.Context, err error) bool {
	_ = ctx
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusTooManyRequests
}

// IsBadRequest reports whether err represents an HTTP 400 validation error.
func IsBadRequest(ctx context.Context, err error) bool {
	_ = ctx
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.StatusCode == http.StatusBadRequest
}
