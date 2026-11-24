package publisher_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
)

const (
	testPublisherToken = "nasher-dev-token"
	testIssuedToken    = "newly-issued-app-token"
	testPkgName        = "com.example.newapp"
)

func TestPublisherCreateAppExtractsTokenAndWarnings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		withFile    bool
		response    string
		wantToken   string
		wantWarning int
	}{
		{
			name:        "multipart upload returns nested api agent token",
			withFile:    true,
			response:    `{"type":"success","message":"App created.","package_name":"com.example.newapp","api_agent":{"token":"newly-issued-app-token"},"warnings":[]}`,
			wantToken:   testIssuedToken,
			wantWarning: 0,
		},
		{
			name:        "json request returns warning without token",
			withFile:    false,
			response:    `{"type":"success","message":"App created.","warnings":["Could not issue API agent token automatically."]}`,
			wantToken:   "",
			wantWarning: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			var artifactPath string
			if tc.withFile {
				artifactPath = filepath.Join(t.TempDir(), "app-release.apk")
				if err := os.WriteFile(artifactPath, []byte("apk-bytes"), 0o600); err != nil {
					t.Fatalf("write temp apk: %v", err)
				}
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != publisher.EndpointApps {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusCreated)
				if _, err := w.Write([]byte(tc.response)); err != nil {
					t.Errorf("write create app response: %v", err)
				}
			}))
			defer srv.Close()

			client, err := publisher.New(ctx, publisher.Config{
				BaseURL: srv.URL,
				Token:   testPublisherToken,
			})
			if err != nil {
				t.Fatalf("new publisher client: %v", err)
			}

			resp, err := client.CreateApp(ctx, publisher.CreateAppRequest{
				PackageName:   testPkgName,
				IssueAPIAgent: true,
				ArtifactPath:  artifactPath,
			}, nil)
			if err != nil {
				t.Fatalf("create app: %v", err)
			}
			if resp.AppToken != tc.wantToken {
				t.Fatalf("got token %q, want %q", resp.AppToken, tc.wantToken)
			}
			if len(resp.Warnings) != tc.wantWarning {
				t.Fatalf("got %d warnings, want %d", len(resp.Warnings), tc.wantWarning)
			}
		})
	}
}
