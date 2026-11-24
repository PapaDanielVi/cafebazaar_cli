package pishkhan_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/pishkhan"
)

const (
	testAppToken   = "app-pishkhan-token"
	testChangeFA   = "چیزهای باحال جدید اضافه شد"
	testChangeEN   = "cool new things added"
	testDevNote    = "Fixes #1"
	testSampleFile = "sample-binary-content"
)

func writeTestResponse(ctx context.Context, t *testing.T, w http.ResponseWriter, body string) {
	_ = ctx
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write test response: %v", err)
	}
}

func TestPishkhanAllEndpoints(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	apkFile := filepath.Join(dir, "app.apk")
	aabFile := filepath.Join(dir, "app.aab")
	binFile := filepath.Join(dir, "signer.bin")
	for _, f := range []string{apkFile, aabFile, binFile} {
		if err := os.WriteFile(f, []byte(testSampleFile), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", f, err)
		}
	}

	var gotCommitReq pishkhan.CommitReleaseRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointReleases:
			w.WriteHeader(http.StatusCreated)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"New Release successfully created.","release":{"creation_date":"2026-01-01T10:00:00","packages":[],"auto_publish":false,"staged_rollout_percentage":100}}`)
		case r.Method == http.MethodDelete && r.URL.Path == pishkhan.EndpointReleases:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"Release successfully deleted."}`)
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointUploadAPK:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse apk form: %v", err)
			}
			if r.FormValue(pishkhan.FormFieldArchitecture) != string(pishkhan.ArchitectureARM64) {
				t.Errorf("unexpected arch: %q", r.FormValue(pishkhan.FormFieldArchitecture))
			}
			if _, _, err := r.FormFile(pishkhan.FormFieldAPK); err != nil {
				t.Errorf("missing apk part: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"Package successfully added to release.","package":{"version_code":12,"version_name":"1.2.0"}}`)
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointUploadAAB:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse aab form: %v", err)
			}
			if _, _, err := r.FormFile(pishkhan.FormFieldAAB); err != nil {
				t.Errorf("missing aab part: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"AAB file successfully added to release.","bundle":{"state":"U","errors":null},"warnings":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointUploadBin:
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("parse bin form: %v", err)
			}
			if _, _, err := r.FormFile(pishkhan.FormFieldBinFile); err != nil {
				t.Errorf("missing bin file part: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"Bin file successfully uploaded.","bundle":{"state":"S","errors":null}}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointBundleStatus:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"Bundle release status successfully retrieved.","bundle":{"state":"D","errors":null}}`)
		case r.Method == http.MethodPost && r.URL.Path == pishkhan.EndpointCommit:
			if err := json.NewDecoder(r.Body).Decode(&gotCommitReq); err != nil {
				t.Errorf("decode commit request: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"Release successfully committed.","release":{"id":159,"release_state":"Active","committed":true,"staged_rollout_percentage":10,"auto_publish":true}}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastCommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"ok","release":{"packages":[{"version_code":11,"version_name":"1.1.0"}]}}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastUncommitted:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"not-exists","message":"No uncommitted release."}`)
		case r.Method == http.MethodGet && r.URL.Path == pishkhan.EndpointLastPublished:
			w.WriteHeader(http.StatusOK)
			writeTestResponse(ctx, t, w, `{"type":"success","message":"ok","release":{"packages":[{"version_code":10,"version_name":"1.0.0"}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client, err := pishkhan.New(ctx, pishkhan.Config{
		BaseURL: srv.URL,
		Token:   testAppToken,
	})
	if err != nil {
		t.Fatalf("new pishkhan client: %v", err)
	}

	created, err := client.CreateRelease(ctx)
	if err != nil || !created.Exists {
		t.Fatalf("create release: resp=%+v err=%v", created, err)
	}

	apkResp, err := client.UploadAPK(ctx, pishkhan.UploadAPKRequest{
		FilePath:     apkFile,
		Architecture: pishkhan.ArchitectureARM64,
	}, nil)
	if err != nil || apkResp.Package.VersionCode != 12 {
		t.Fatalf("upload apk: resp=%+v err=%v", apkResp, err)
	}

	aabResp, err := client.UploadAAB(ctx, aabFile, nil)
	if err != nil || aabResp.Bundle.State != pishkhan.BundleStateUploaded {
		t.Fatalf("upload aab: resp=%+v err=%v", aabResp, err)
	}

	binResp, err := client.UploadBin(ctx, binFile, nil)
	if err != nil || binResp.Bundle.State != pishkhan.BundleStateSigned {
		t.Fatalf("upload bin: resp=%+v err=%v", binResp, err)
	}

	bundleResp, err := client.BundleStatus(ctx)
	if err != nil || bundleResp.Bundle.State != pishkhan.BundleStateDone {
		t.Fatalf("bundle status: resp=%+v err=%v", bundleResp, err)
	}

	commitResp, err := client.CommitRelease(ctx, pishkhan.CommitReleaseRequest{
		ChangelogEN:             testChangeEN,
		ChangelogFA:             testChangeFA,
		DeveloperNote:           testDevNote,
		StagedRolloutPercentage: 10,
		AutoPublish:             true,
	})
	if err != nil || !commitResp.Exists || commitResp.Release.ID != 159 {
		t.Fatalf("commit release: resp=%+v err=%v", commitResp, err)
	}
	if gotCommitReq.ChangelogFA != testChangeFA || !gotCommitReq.AutoPublish {
		t.Fatalf("unexpected commit request decoded: %+v", gotCommitReq)
	}

	committed, err := client.LastCommittedRelease(ctx)
	if err != nil || !committed.Exists || committed.Release.Packages[0].VersionCode != 11 {
		t.Fatalf("last committed: resp=%+v err=%v", committed, err)
	}

	uncommitted, err := client.LastUncommittedRelease(ctx)
	if err != nil || uncommitted.Exists {
		t.Fatalf("last uncommitted should be not-exists: resp=%+v err=%v", uncommitted, err)
	}

	published, err := client.LastPublishedRelease(ctx)
	if err != nil || !published.Exists || published.Release.Packages[0].VersionCode != 10 {
		t.Fatalf("last published: resp=%+v err=%v", published, err)
	}

	deleted, err := client.DeleteRelease(ctx)
	if err != nil || deleted.Type != pishkhan.ResponseTypeSuccess {
		t.Fatalf("delete release: resp=%+v err=%v", deleted, err)
	}
}

func TestWaitForBundle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		states    []string
		wantState pishkhan.BundleState
		wantErr   bool
	}{
		{
			name:      "transitions to done",
			states:    []string{`{"type":"success","bundle":{"state":"S"}}`, `{"type":"success","bundle":{"state":"P"}}`, `{"type":"success","bundle":{"state":"D"}}`},
			wantState: pishkhan.BundleStateDone,
		},
		{
			name:      "transitions to error",
			states:    []string{`{"type":"success","bundle":{"state":"P"}}`, `{"type":"success","bundle":{"state":"E","errors":["invalid signature"]}}`},
			wantState: pishkhan.BundleStateError,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			var idx atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := int(idx.Add(1)) - 1
				if i >= len(tc.states) {
					i = len(tc.states) - 1
				}
				w.WriteHeader(http.StatusOK)
				writeTestResponse(ctx, t, w, tc.states[i])
			}))
			defer srv.Close()

			client, err := pishkhan.New(ctx, pishkhan.Config{
				BaseURL: srv.URL,
				Token:   testAppToken,
			})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}

			info, err := client.WaitForBundle(ctx, 5*time.Millisecond, nil)
			if tc.wantErr && !errors.Is(err, pishkhan.ErrBundleFailed) {
				t.Fatalf("got err=%v, want ErrBundleFailed", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if info.State != tc.wantState {
				t.Fatalf("got state %q, want %q", info.State, tc.wantState)
			}
		})
	}
}
