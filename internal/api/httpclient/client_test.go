package httpclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
)

const (
	testToken      = "test-secret-token"
	testEndpoint   = "/v1/test/"
	testUploadPath = "/v1/upload/"
	testPersianMsg = "چیزهای باحال جدید اضافه شد"
)

func TestClientDoJSONAndErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		wantErr        bool
		wantForbidden  bool
		wantBadRequest bool
		wantRateLimit  bool
	}{
		{
			name:         "success with utf8 persian payload",
			statusCode:   http.StatusOK,
			responseBody: `{"type":"success","message":"` + testPersianMsg + `"}`,
		},
		{
			name:           "pishkhan bad request",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"type":"bad_request","message":"There is not an uncommitted release."}`,
			wantErr:        true,
			wantBadRequest: true,
		},
		{
			name:          "pishkhan forbidden",
			statusCode:    http.StatusForbidden,
			responseBody:  `{"type":"access_forbidden","message":"You do not have permission to perform this action."}`,
			wantErr:       true,
			wantForbidden: true,
		},
		{
			name:          "sandoghche persian detail error",
			statusCode:    http.StatusForbidden,
			responseBody:  `{"detail":"API Key نامعتبر است."}`,
			wantErr:       true,
			wantForbidden: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			var receivedBody string
			var receivedHeader string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeader = r.Header.Get(httpclient.HeaderPishkhanSecret)
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				receivedBody = string(data)
				w.WriteHeader(tc.statusCode)
				if _, err := w.Write([]byte(tc.responseBody)); err != nil {
					t.Errorf("write response: %v", err)
				}
			}))
			defer srv.Close()

			client, err := httpclient.New(ctx, httpclient.Config{
				BaseURL:    srv.URL,
				Token:      testToken,
				MaxRetries: 1,
				RetryWait:  time.Millisecond,
			})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}

			var out map[string]string
			_, err = client.DoJSON(ctx, http.MethodPost, testEndpoint, nil, map[string]string{"note": testPersianMsg}, &out)
			if receivedHeader != testToken {
				t.Fatalf("got auth header %q, want %q", receivedHeader, testToken)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if tc.wantForbidden && !httpclient.IsForbidden(ctx, err) {
				t.Fatalf("expected forbidden error, got %v", err)
			}
			if tc.wantBadRequest && !httpclient.IsBadRequest(ctx, err) {
				t.Fatalf("expected bad request error, got %v", err)
			}
			if !tc.wantErr {
				if out["message"] != testPersianMsg {
					t.Fatalf("got message %q, want %q", out["message"], testPersianMsg)
				}
				var sent map[string]string
				if err := json.Unmarshal([]byte(receivedBody), &sent); err != nil {
					t.Fatalf("unmarshal sent json: %v", err)
				}
				if sent["note"] != testPersianMsg {
					t.Fatalf("got sent note %q, want %q", sent["note"], testPersianMsg)
				}
			}
		})
	}
}

func TestClientRetriesRateLimit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"type":"too_many_requests","message":"Too many requests"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client, err := httpclient.New(ctx, httpclient.Config{
		BaseURL:    srv.URL,
		Token:      testToken,
		MaxRetries: 3,
		RetryWait:  5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	var out struct {
		OK bool `json:"ok"`
	}
	if _, err := client.DoJSON(ctx, http.MethodGet, testEndpoint, nil, nil, &out); err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if !out.OK || calls.Load() != 3 {
		t.Fatalf("got ok=%v calls=%d, want ok=true calls=3", out.OK, calls.Load())
	}
}

func TestClientDoMultipartWithProgress(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	apkPath := filepath.Join(dir, "app-release.apk")
	payload := []byte("fake-apk-binary-contents")
	if err := os.WriteFile(apkPath, payload, 0o600); err != nil {
		t.Fatalf("write temp apk: %v", err)
	}

	var gotArch string
	var gotFileBytes []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			return
		}
		gotArch = r.FormValue("architecture")
		file, _, err := r.FormFile("apk")
		if err != nil {
			t.Errorf("read apk form file: %v", err)
			return
		}
		defer func() {
			_ = file.Close()
		}()
		gotFileBytes, err = io.ReadAll(file)
		if err != nil {
			t.Errorf("read apk bytes: %v", err)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type":"success"}`))
	}))
	defer srv.Close()

	client, err := httpclient.New(ctx, httpclient.Config{
		BaseURL: srv.URL,
		Token:   testToken,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	var lastSent int64
	var lastTotal int64
	var resp map[string]string
	_, err = client.DoMultipart(
		ctx,
		testUploadPath,
		map[string]string{"architecture": "all"},
		"apk",
		apkPath,
		httpclient.ContentTypeAPK,
		func(progressCtx context.Context, sent, total int64) {
			_ = progressCtx
			lastSent = sent
			lastTotal = total
		},
		&resp,
	)
	if err != nil {
		t.Fatalf("multipart upload failed: %v", err)
	}
	if gotArch != "all" {
		t.Fatalf("got architecture %q, want all", gotArch)
	}
	if string(gotFileBytes) != string(payload) {
		t.Fatalf("got file %q, want %q", string(gotFileBytes), string(payload))
	}
	if lastSent != int64(len(payload)) || lastTotal != int64(len(payload)) {
		t.Fatalf("got progress %d/%d, want %d/%d", lastSent, lastTotal, len(payload), len(payload))
	}
}

func TestClientValidationErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	if _, err := httpclient.New(ctx, httpclient.Config{Token: testToken}); !errors.Is(err, httpclient.ErrEmptyBaseURL) {
		t.Fatalf("got %v, want ErrEmptyBaseURL", err)
	}
	if _, err := httpclient.New(ctx, httpclient.Config{BaseURL: "https://example.com"}); !errors.Is(err, httpclient.ErrEmptyToken) {
		t.Fatalf("got %v, want ErrEmptyToken", err)
	}
	if _, err := httpclient.New(ctx, httpclient.Config{BaseURL: "https://example.com", Token: testToken, ProxyURL: "ftp://bad-proxy:21"}); !errors.Is(err, httpclient.ErrInvalidProxyURL) {
		t.Fatalf("got %v, want ErrInvalidProxyURL", err)
	}
}

func TestClientRoutesRequestsThroughHTTPProxy(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var proxiedRequestURL string
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedRequestURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"proxied":true}`))
	}))
	defer proxySrv.Close()

	client, err := httpclient.New(ctx, httpclient.Config{
		BaseURL:  "http://api.pishkhan.cafebazaar.ir",
		Token:    testToken,
		ProxyURL: proxySrv.URL,
	})
	if err != nil {
		t.Fatalf("new proxied client: %v", err)
	}

	var out struct {
		Proxied bool `json:"proxied"`
	}
	if _, err := client.DoJSON(ctx, http.MethodGet, testEndpoint, nil, nil, &out); err != nil {
		t.Fatalf("proxied request failed: %v", err)
	}
	if !out.Proxied || proxiedRequestURL != "http://api.pishkhan.cafebazaar.ir/v1/test/" {
		t.Fatalf("expected request routed through proxy, got proxied=%v url=%q", out.Proxied, proxiedRequestURL)
	}
}

func TestParseAndMaskProxyURL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name       string
		raw        string
		wantScheme string
		wantDirect bool
		wantErr    bool
		wantMasked string
	}{
		{name: "empty uses env", raw: "", wantMasked: ""},
		{name: "direct bypasses env", raw: "direct", wantDirect: true, wantMasked: "direct"},
		{name: "none bypasses env", raw: "none", wantDirect: true, wantMasked: "none"},
		{name: "socks5h normalized to socks5", raw: "socks5h://user:topsecret@127.0.0.1:1080", wantScheme: "socks5", wantMasked: "socks5h://user:%2A%2A%2A@127.0.0.1:1080"},
		{name: "http with auth masked", raw: "http://dev:pass123@proxy.local:8080", wantScheme: "http", wantMasked: "http://dev:%2A%2A%2A@proxy.local:8080"},
		{name: "unsupported scheme rejected", raw: "ssh://proxy.local:22", wantErr: true},
		{name: "missing host rejected", raw: "http://", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, direct, err := httpclient.ParseProxyURL(ctx, tc.raw)
			if tc.wantErr {
				if !errors.Is(err, httpclient.ErrInvalidProxyURL) {
					t.Fatalf("got err=%v, want ErrInvalidProxyURL", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if direct != tc.wantDirect {
				t.Fatalf("got direct=%v, want %v", direct, tc.wantDirect)
			}
			if tc.wantScheme != "" && (parsed == nil || parsed.Scheme != tc.wantScheme) {
				t.Fatalf("got parsed=%v, want scheme %q", parsed, tc.wantScheme)
			}
			if gotMasked := httpclient.MaskProxyURL(ctx, tc.raw); gotMasked != tc.wantMasked {
				t.Fatalf("got masked %q, want %q", gotMasked, tc.wantMasked)
			}
		})
	}
}
