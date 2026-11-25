package httpclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
)

const (
	testPishkhanSecretToken = "pishkhan-super-secret-token-9999"
	testNasherSecretToken   = "nasher-publisher-secret-token-8888"
	testIssuedAgentToken    = "newly-issued-pishkhan-token-7777"
	wantHTTPPostPrefix      = "HTTP -> POST"
	wantRedactedTokenJSON   = `"token":"[REDACTED]"`
)

func writeDebugTestBody(ctx context.Context, t *testing.T, w http.ResponseWriter, body string) {
	_ = ctx
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write mock response body: %v", err)
	}
}

func TestDebugLoggingRedactsTokensAcrossJSONAndMultipart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("json request and response redact pishkhan token and response tokens", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Request-Id", "req-123")
			w.Header().Set("Set-Cookie", "session=secret-cookie-value")
			w.WriteHeader(http.StatusOK)
			writeDebugTestBody(ctx, t, w, `{"type":"success","package_name":"com.example.demo","api_agent":{"token":"`+testIssuedAgentToken+`"},"note":"echo `+testPishkhanSecretToken+`"}`)
		}))
		defer srv.Close()

		var logs []string
		client, err := httpclient.New(ctx, httpclient.Config{
			BaseURL:        srv.URL,
			Token:          testPishkhanSecretToken,
			AuthHeaderName: httpclient.HeaderPishkhanSecret,
			DebugLog: func(_ context.Context, msg string) {
				logs = append(logs, msg)
			},
		})
		if err != nil {
			t.Fatalf("create client: %v", err)
		}

		var out map[string]any
		reqBody := map[string]any{
			"staged_rollout_percentage": 50,
			"auto_publish":              true,
		}
		if _, err := client.DoJSON(ctx, http.MethodPost, "/v1/apps/releases/last-uncommitted/commit/", nil, reqBody, &out); err != nil {
			t.Fatalf("do json: %v", err)
		}

		joined := strings.Join(logs, "\n")
		for _, secret := range []string{testPishkhanSecretToken, testIssuedAgentToken, "secret-cookie-value"} {
			if strings.Contains(joined, secret) {
				t.Fatalf("secret %q leaked into verbose logs:\n%s", secret, joined)
			}
		}
		for _, want := range []string{
			wantHTTPPostPrefix,
			"Cafebazaar-Pishkhan-Api-Secret=[REDACTED]",
			`"staged_rollout_percentage":50`,
			"HTTP <- 200 OK",
			"Set-Cookie=[REDACTED]",
			`"package_name":"com.example.demo"`,
			wantRedactedTokenJSON,
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("expected %q in verbose logs, got:\n%s", want, joined)
			}
		}
	})

	t.Run("multipart upload redacts nasher token while logging fields and files", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
			writeDebugTestBody(ctx, t, w, `{"package_name":"com.example.newapp","api_agent":{"token":"`+testIssuedAgentToken+`"}}`)
		}))
		defer srv.Close()

		tmpDir := t.TempDir()
		apkFile := filepath.Join(tmpDir, "app-release.apk")
		if err := os.WriteFile(apkFile, []byte("fake-apk-bytes"), 0o600); err != nil {
			t.Fatalf("write fake apk: %v", err)
		}

		var logs []string
		client, err := httpclient.New(ctx, httpclient.Config{
			BaseURL:        srv.URL,
			Token:          testNasherSecretToken,
			AuthHeaderName: "Authorization",
			DebugLog: func(_ context.Context, msg string) {
				logs = append(logs, msg)
			},
		})
		if err != nil {
			t.Fatalf("create client: %v", err)
		}

		var out map[string]any
		fields := map[string]string{
			"category_id":     "11",
			"name_fa":         "ابزار تست",
			"issue_api_agent": "true",
		}
		if _, err := client.DoMultipart(ctx, "/v1/apps/", fields, "apk", apkFile, httpclient.ContentTypeAPK, nil, &out); err != nil {
			t.Fatalf("do multipart: %v", err)
		}

		joined := strings.Join(logs, "\n")
		for _, secret := range []string{testNasherSecretToken, testIssuedAgentToken} {
			if strings.Contains(joined, secret) {
				t.Fatalf("secret %q leaked into multipart verbose logs:\n%s", secret, joined)
			}
		}
		for _, want := range []string{
			wantHTTPPostPrefix,
			"multipart/form-data",
			"Authorization=[REDACTED]",
			`category_id="11"`,
			`name_fa="ابزار تست"`,
			"app-release.apk",
			"HTTP <- 201 Created",
			wantRedactedTokenJSON,
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("expected %q in multipart verbose logs, got:\n%s", want, joined)
			}
		}
	})
}

func TestSanitizeURLRedactsCredentialsAndSensitiveParams(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	got := httpclient.SanitizeURL(
		ctx,
		"https://user:superpass@api.example.com/v1/test?token=abc123&limit=20&note="+testPishkhanSecretToken,
		[]string{testPishkhanSecretToken},
	)
	for _, secret := range []string{"superpass", "abc123", testPishkhanSecretToken} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q leaked in sanitized URL: %q", secret, got)
		}
	}
	if !strings.Contains(got, "limit=20") {
		t.Fatalf("expected non-secret query param limit=20 in %q", got)
	}
}
