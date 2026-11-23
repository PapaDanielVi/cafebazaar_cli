package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultRequestTimeout is the default timeout for standard API metadata requests.
	DefaultRequestTimeout = 2 * time.Minute
	// DefaultPublishTimeout is the 10-minute timeout for release commit and app submission calls.
	DefaultPublishTimeout = 10 * time.Minute
	// DefaultUploadTimeout is the 15-minute timeout for package uploads and server-side APK inspection.
	DefaultUploadTimeout = 15 * time.Minute

	defaultMaxRetries = 3
	defaultRetryWait  = 2 * time.Second
	maxErrorBodyBytes = 64 * 1024
	httpSuccessMin    = 200
	httpSuccessMax    = 299
)

// Config configures a shared CafeBazaar API HTTP client.
type Config struct {
	BaseURL        string
	Token          string
	AuthHeaderName string
	UserAgent      string
	ProxyURL       string
	Timeout        time.Duration
	UploadTimeout  time.Duration
	MaxRetries     int
	RetryWait      time.Duration
	HTTPClient     *http.Client
}

// Client handles authenticated JSON and multipart HTTP requests to CafeBazaar APIs.
type Client struct {
	baseURL        string
	token          string
	authHeaderName string
	userAgent      string
	timeout        time.Duration
	uploadTimeout  time.Duration
	maxRetries     int
	retryWait      time.Duration
	httpClient     *http.Client
}

// New constructs a validated HTTP client for CafeBazaar API calls.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, ErrEmptyBaseURL
	}
	token := strings.TrimSpace(cfg.Token)
	if token == "" {
		return nil, ErrEmptyToken
	}

	authHeader := strings.TrimSpace(cfg.AuthHeaderName)
	if authHeader == "" {
		authHeader = HeaderPishkhanSecret
	}
	userAgent := strings.TrimSpace(cfg.UserAgent)
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	uploadTimeout := cfg.UploadTimeout
	if uploadTimeout <= 0 {
		uploadTimeout = DefaultUploadTimeout
	}
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	if cfg.MaxRetries == 0 {
		maxRetries = defaultMaxRetries
	}
	retryWait := cfg.RetryWait
	if retryWait <= 0 {
		retryWait = defaultRetryWait
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		transport, err := buildTransport(ctx, cfg.ProxyURL)
		if err != nil {
			return nil, err
		}
		httpClient = &http.Client{Transport: transport}
	}

	return &Client{
		baseURL:        baseURL,
		token:          token,
		authHeaderName: authHeader,
		userAgent:      userAgent,
		timeout:        timeout,
		uploadTimeout:  uploadTimeout,
		maxRetries:     maxRetries,
		retryWait:      retryWait,
		httpClient:     httpClient,
	}, nil
}

// DoJSON sends an authenticated JSON request using the client's default timeout.
func (c *Client) DoJSON(ctx context.Context, method, endpoint string, query url.Values, body any, out any) (int, error) {
	return c.DoJSONWithTimeout(ctx, c.timeout, method, endpoint, query, body, out)
}

// DoJSONWithTimeout sends an authenticated JSON request bounded by timeout and decodes the response into out.
func (c *Client) DoJSONWithTimeout(ctx context.Context, timeout time.Duration, method, endpoint string, query url.Values, body any, out any) (int, error) {
	reqURL, err := c.buildURL(ctx, endpoint, query)
	if err != nil {
		return 0, err
	}

	var payload []byte
	if body != nil {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(body); err != nil {
			return 0, fmt.Errorf("encode request body: %w", err)
		}
		payload = bytes.TrimRight(buf.Bytes(), "\n")
	}

	effectiveTimeout := timeout
	if effectiveTimeout <= 0 {
		effectiveTimeout = c.timeout
	}
	reqCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
	defer cancel()

	return c.executeWithRetry(reqCtx, func(attemptCtx context.Context) (*http.Request, error) {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(attemptCtx, method, reqURL, reader)
		if err != nil {
			return nil, fmt.Errorf("build http request: %w", err)
		}
		req.Header.Set(c.authHeaderName, c.token)
		req.Header.Set(HeaderUserAgent, c.userAgent)
		req.Header.Set(HeaderContentType, ContentTypeJSON)
		return req, nil
	}, out)
}

// DoMultipart streams a multipart/form-data file upload with optional progress callbacks.
func (c *Client) DoMultipart(
	ctx context.Context,
	endpoint string,
	fields map[string]string,
	fileField string,
	filePath string,
	fileContentType string,
	onProgress ProgressFunc,
	out any,
) (int, error) {
	if strings.TrimSpace(filePath) == "" {
		return 0, ErrEmptyFilePath
	}

	reqURL, err := c.buildURL(ctx, endpoint, nil)
	if err != nil {
		return 0, err
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return 0, fmt.Errorf("stat upload file %q: %w", filePath, err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("upload path %q is a directory", filePath)
	}

	if fileContentType == "" {
		fileContentType = ContentTypeOctetStream
	}

	reqCtx, cancel := context.WithTimeout(ctx, c.uploadTimeout)
	defer cancel()

	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	multipartContentType := mw.FormDataContentType()

	errCh := make(chan error, 1)
	go func() {
		writeErr := writeMultipartBody(reqCtx, mw, fields, fileField, filePath, fileContentType, info.Size(), onProgress)
		closeWriterErr := mw.Close()
		finalErr := errors.Join(writeErr, closeWriterErr)
		if closePipeErr := pw.CloseWithError(finalErr); closePipeErr != nil && finalErr == nil {
			finalErr = closePipeErr
		}
		errCh <- finalErr
	}()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, reqURL, pr)
	if err != nil {
		_ = pr.Close()
		<-errCh
		return 0, fmt.Errorf("build multipart request: %w", err)
	}
	req.Header.Set(c.authHeaderName, c.token)
	req.Header.Set(HeaderUserAgent, c.userAgent)
	req.Header.Set(HeaderContentType, multipartContentType)

	resp, doErr := c.httpClient.Do(req)
	pipeErr := <-errCh
	if doErr != nil {
		if pipeErr != nil {
			return 0, fmt.Errorf("upload multipart body: %w", errors.Join(doErr, pipeErr))
		}
		return 0, fmt.Errorf("send multipart request: %w", doErr)
	}

	status, decodeErr := decodeResponse(reqCtx, resp, out)
	if pipeErr != nil {
		return status, fmt.Errorf("stream multipart body: %w", pipeErr)
	}
	return status, decodeErr
}

func (c *Client) executeWithRetry(
	ctx context.Context,
	buildReq func(context.Context) (*http.Request, error),
	out any,
) (int, error) {
	var lastErr error
	var lastStatus int

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		req, err := buildReq(ctx)
		if err != nil {
			return 0, err
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, fmt.Errorf("execute http request: %w", err)
		}

		if resp.StatusCode == http.StatusTooManyRequests && attempt < c.maxRetries {
			wait := retryDelay(ctx, resp.Header.Get(HeaderRetryAfter), c.retryWait, attempt)
			_, drainErr := io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyBytes))
			closeErr := resp.Body.Close()
			if drainErr != nil {
				return resp.StatusCode, fmt.Errorf("drain rate-limited response: %w", drainErr)
			}
			if closeErr != nil {
				return resp.StatusCode, fmt.Errorf("close rate-limited response: %w", closeErr)
			}

			if err := sleepWithContext(ctx, wait); err != nil {
				return resp.StatusCode, err
			}
			continue
		}

		status, decodeErr := decodeResponse(ctx, resp, out)
		lastStatus = status
		lastErr = decodeErr
		return lastStatus, lastErr
	}

	return lastStatus, lastErr
}

func (c *Client) buildURL(ctx context.Context, endpoint string, query url.Values) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("build request url: %w", err)
	}

	rawURL := endpoint
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		if !strings.HasPrefix(endpoint, "/") {
			endpoint = "/" + endpoint
		}
		rawURL = c.baseURL + endpoint
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse endpoint url %q: %w", rawURL, err)
	}
	if len(query) > 0 {
		existing := parsed.Query()
		for k, vals := range query {
			existing.Del(k)
			for _, v := range vals {
				existing.Add(k, v)
			}
		}
		parsed.RawQuery = existing.Encode()
	}

	return parsed.String(), nil
}

func writeMultipartBody(
	ctx context.Context,
	mw *multipart.Writer,
	fields map[string]string,
	fileField string,
	filePath string,
	fileContentType string,
	fileSize int64,
	onProgress ProgressFunc,
) error {
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			return fmt.Errorf("write multipart field %q: %w", k, err)
		}
	}

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open upload file %q: %w", filePath, err)
	}
	defer func() {
		_ = file.Close()
	}()

	header := make(textproto.MIMEHeader)
	escapedName := escapeMultipartQuotes(ctx, filepath.Base(filePath))
	escapedField := escapeMultipartQuotes(ctx, fileField)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapedField, escapedName))
	header.Set(HeaderContentType, fileContentType)

	part, err := mw.CreatePart(header)
	if err != nil {
		return fmt.Errorf("create multipart file part %q: %w", fileField, err)
	}

	reader := newProgressReader(ctx, file, fileSize, onProgress)
	if _, err := io.Copy(part, reader); err != nil {
		return fmt.Errorf("copy upload file %q: %w", filePath, err)
	}
	return nil
}

func escapeMultipartQuotes(ctx context.Context, s string) string {
	_ = ctx
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(s)
}

func decodeResponse(ctx context.Context, resp *http.Response, out any) (int, error) {
	_ = ctx
	bodyBytes, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return resp.StatusCode, fmt.Errorf("read response body: %w", readErr)
	}
	if closeErr != nil {
		return resp.StatusCode, fmt.Errorf("close response body: %w", closeErr)
	}

	if resp.StatusCode < httpSuccessMin || resp.StatusCode > httpSuccessMax {
		return resp.StatusCode, parseAPIError(ctx, resp.StatusCode, bodyBytes)
	}

	if out != nil && len(bytes.TrimSpace(bodyBytes)) > 0 {
		if err := json.Unmarshal(bodyBytes, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response json: %w", err)
		}
	}

	return resp.StatusCode, nil
}

func parseAPIError(ctx context.Context, statusCode int, body []byte) error {
	_ = ctx
	var envelope struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil {
		if envelope.Type != "" || envelope.Message != "" || envelope.Detail != "" {
			return &APIError{
				StatusCode: statusCode,
				Type:       envelope.Type,
				Message:    envelope.Message,
				Detail:     envelope.Detail,
				RawBody:    strings.TrimSpace(string(body)),
			}
		}
	}
	return &APIError{
		StatusCode: statusCode,
		RawBody:    strings.TrimSpace(string(body)),
	}
}

func retryDelay(ctx context.Context, retryAfter string, baseWait time.Duration, attempt int) time.Duration {
	_ = ctx
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	multiplier := 1 << attempt
	return baseWait * time.Duration(multiplier)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait cancelled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// ParseProxyURL validates and normalizes a proxy URL (supporting http, https, socks5, socks5h, direct, and none).
func ParseProxyURL(ctx context.Context, raw string) (*url.URL, bool, error) {
	_ = ctx
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false, nil
	}
	lower := strings.ToLower(trimmed)
	if lower == ProxyDirect || lower == ProxyNone {
		return nil, true, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, false, fmt.Errorf("%w %q: %w", ErrInvalidProxyURL, trimmed, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == ProxySchemeSOCKS5H {
		parsed.Scheme = ProxySchemeSOCKS5
		scheme = ProxySchemeSOCKS5
	}
	switch scheme {
	case ProxySchemeHTTP, ProxySchemeHTTPS, ProxySchemeSOCKS5:
	default:
		return nil, false, fmt.Errorf("%w %q: unsupported scheme %q (use http, https, socks5, socks5h, or direct)", ErrInvalidProxyURL, trimmed, parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return nil, false, fmt.Errorf("%w %q: missing host", ErrInvalidProxyURL, trimmed)
	}
	return parsed, false, nil
}

// MaskProxyURL redacts any embedded password in a proxy URL for safe terminal display.
func MaskProxyURL(ctx context.Context, raw string) string {
	_ = ctx
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.User == nil {
		return trimmed
	}
	if _, hasPass := parsed.User.Password(); !hasPass {
		return trimmed
	}
	clone := *parsed
	clone.User = url.UserPassword(parsed.User.Username(), "***")
	return clone.String()
}

func buildTransport(ctx context.Context, proxyURL string) (*http.Transport, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	var transport *http.Transport
	if ok {
		transport = base.Clone()
	} else {
		transport = &http.Transport{}
	}

	parsed, direct, err := ParseProxyURL(ctx, proxyURL)
	if err != nil {
		return nil, err
	}
	switch {
	case direct:
		transport.Proxy = nil
	case parsed != nil:
		transport.Proxy = http.ProxyURL(parsed)
	default:
		transport.Proxy = http.ProxyFromEnvironment
	}
	return transport, nil
}
