package publisher_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
)

const (
	testPublisherToken = "nasher-dev-token"
	testIssuedToken    = "newly-issued-app-token"
	testPkgName        = "com.example.newapp"
)

func writeTestPNG(ctx context.Context, t *testing.T, path string, w, h int) string {
	_ = ctx
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 0, G: 177, B: 79, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write png %s: %v", path, err)
	}
	return path
}

func TestPublisherCreateAppMultipartFieldsFilesAndResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		response    string
		wantToken   string
		wantWarning int
		wantAppURL  string
	}{
		{
			name:        "returns nested api agent token and listing metadata",
			response:    `{"type":"success","message":"New app successfully created and submitted for review.","package_name":"com.example.newapp","version_code":1,"version_name":"1.0","app_url":"https://cafebazaar.ir/app/com.example.newapp/","review_state":"in_review","auto_publish":true,"api_agent":{"token":"newly-issued-app-token"},"warnings":[]}`,
			wantToken:   testIssuedToken,
			wantWarning: 0,
			wantAppURL:  "https://cafebazaar.ir/app/com.example.newapp/",
		},
		{
			name:        "returns warning without token",
			response:    `{"type":"success","message":"App created.","warnings":["Could not issue API agent token automatically."]}`,
			wantToken:   "",
			wantWarning: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			dir := t.TempDir()
			apkPath := filepath.Join(dir, "app-release.apk")
			if err := os.WriteFile(apkPath, []byte("apk-bytes"), 0o600); err != nil {
				t.Fatalf("write temp apk: %v", err)
			}
			iconPath := writeTestPNG(ctx, t, filepath.Join(dir, "icon.png"), 512, 512)
			shot1 := writeTestPNG(ctx, t, filepath.Join(dir, "shot1.png"), 1080, 1920)
			shot2 := writeTestPNG(ctx, t, filepath.Join(dir, "shot2.png"), 1080, 1920)

			var gotCategoryID string
			var gotNameFA string
			var gotNameEN string
			var gotEmail string
			var gotScreenshotCount int
			var gotIconPresent bool
			var gotAPKContentType string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != publisher.EndpointApps {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if err := r.ParseMultipartForm(4 << 20); err != nil {
					t.Errorf("parse multipart: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				gotCategoryID = r.FormValue(publisher.FieldCategoryID)
				gotNameFA = r.FormValue(publisher.FieldNameFA)
				gotNameEN = r.FormValue(publisher.FieldNameEN)
				gotEmail = r.FormValue(publisher.FieldEmail)
				if apkHeaders := r.MultipartForm.File[publisher.FieldAPK]; len(apkHeaders) == 1 {
					gotAPKContentType = apkHeaders[0].Header.Get("Content-Type")
				}
				gotIconPresent = len(r.MultipartForm.File[publisher.FieldIcon]) == 1
				gotScreenshotCount = len(r.MultipartForm.File[publisher.FieldScreenshots])

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
				PackageName:        testPkgName,
				CategoryID:         10,
				NameFA:             "اپ نمونه",
				NameEN:             "Sample App",
				ShortDescriptionFA: "توضیح کوتاه فارسی",
				ShortDescriptionEN: "Short English summary",
				DescriptionFA:      "توضیحات کامل اپلیکیشن نمونه",
				DescriptionEN:      "Full English description",
				Email:              "dev@example.com",
				IconPath:           iconPath,
				ScreenshotPaths:    []string{shot1, shot2},
				IssueAPIAgent:      true,
				ArtifactPath:       apkPath,
			}, nil)
			if err != nil {
				t.Fatalf("create app: %v", err)
			}
			if gotCategoryID != "10" || gotNameFA != "اپ نمونه" || gotNameEN != "Sample App" || gotEmail != "dev@example.com" {
				t.Fatalf("unexpected form fields: cat=%q name_fa=%q name_en=%q email=%q", gotCategoryID, gotNameFA, gotNameEN, gotEmail)
			}
			if gotAPKContentType != "application/vnd.android.package-archive" {
				t.Fatalf("got apk content-type %q, want application/vnd.android.package-archive", gotAPKContentType)
			}
			if !gotIconPresent || gotScreenshotCount != 2 {
				t.Fatalf("got icon=%v screenshots=%d, want true and 2", gotIconPresent, gotScreenshotCount)
			}
			if resp.AppToken != tc.wantToken {
				t.Fatalf("got token %q, want %q", resp.AppToken, tc.wantToken)
			}
			if len(resp.Warnings) != tc.wantWarning {
				t.Fatalf("got %d warnings, want %d", len(resp.Warnings), tc.wantWarning)
			}
			if tc.wantAppURL != "" && resp.AppURL != tc.wantAppURL {
				t.Fatalf("got app_url %q, want %q", resp.AppURL, tc.wantAppURL)
			}
		})
	}
}

func TestCreateAppRequestValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	apkPath := filepath.Join(dir, "app.apk")
	if err := os.WriteFile(apkPath, []byte("apk"), 0o600); err != nil {
		t.Fatalf("write apk: %v", err)
	}
	validIcon := writeTestPNG(ctx, t, filepath.Join(dir, "icon512.png"), 512, 512)
	smallIcon := writeTestPNG(ctx, t, filepath.Join(dir, "icon256.png"), 256, 256)
	rectIcon := writeTestPNG(ctx, t, filepath.Join(dir, "icon_rect.png"), 512, 600)
	shot := writeTestPNG(ctx, t, filepath.Join(dir, "shot.png"), 1080, 1920)

	validBase := func() publisher.CreateAppRequest {
		return publisher.CreateAppRequest{
			CategoryID:         10,
			NameFA:             "برنامه تست",
			NameEN:             "Test App",
			ShortDescriptionFA: "توضیح کوتاه",
			DescriptionFA:      "توضیح کامل",
			Email:              "dev@example.com",
			ArtifactPath:       apkPath,
			IconPath:           validIcon,
			ScreenshotPaths:    []string{shot},
		}
	}

	shots13 := make([]string, 13)
	for i := range shots13 {
		shots13[i] = shot
	}

	tests := []struct {
		name    string
		mutate  func(*publisher.CreateAppRequest)
		wantErr error
	}{
		{
			name:   "valid request passes",
			mutate: func(r *publisher.CreateAppRequest) {},
		},
		{
			name:    "missing category id",
			mutate:  func(r *publisher.CreateAppRequest) { r.CategoryID = 0 },
			wantErr: publisher.ErrInvalidCategoryID,
		},
		{
			name:    "name_fa over 36 runes",
			mutate:  func(r *publisher.CreateAppRequest) { r.NameFA = strings.Repeat("آ", 37) },
			wantErr: publisher.ErrInvalidAppName,
		},
		{
			name:    "name_en over 36 runes",
			mutate:  func(r *publisher.CreateAppRequest) { r.NameEN = strings.Repeat("a", 37) },
			wantErr: publisher.ErrInvalidAppName,
		},
		{
			name:    "short_description_fa over 126 runes",
			mutate:  func(r *publisher.CreateAppRequest) { r.ShortDescriptionFA = strings.Repeat("آ", 127) },
			wantErr: publisher.ErrInvalidShortDescription,
		},
		{
			name:    "short_description_en over 126 runes",
			mutate:  func(r *publisher.CreateAppRequest) { r.ShortDescriptionEN = strings.Repeat("a", 127) },
			wantErr: publisher.ErrInvalidShortDescription,
		},
		{
			name:    "invalid email",
			mutate:  func(r *publisher.CreateAppRequest) { r.Email = "not-an-email" },
			wantErr: publisher.ErrInvalidEmail,
		},
		{
			name:    "icon smaller than 512x512",
			mutate:  func(r *publisher.CreateAppRequest) { r.IconPath = smallIcon },
			wantErr: publisher.ErrInvalidIcon,
		},
		{
			name:    "non-square icon",
			mutate:  func(r *publisher.CreateAppRequest) { r.IconPath = rectIcon },
			wantErr: publisher.ErrInvalidIcon,
		},
		{
			name:    "no screenshots",
			mutate:  func(r *publisher.CreateAppRequest) { r.ScreenshotPaths = nil },
			wantErr: publisher.ErrInvalidScreenshots,
		},
		{
			name:    "more than 12 screenshots",
			mutate:  func(r *publisher.CreateAppRequest) { r.ScreenshotPaths = shots13 },
			wantErr: publisher.ErrInvalidScreenshots,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := validBase()
			tc.mutate(&req)
			err := req.Validate(ctx)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got err=%v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestOfficialCategoriesCatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cats := publisher.Categories(ctx)
	if len(cats) != 30 {
		t.Fatalf("got %d categories, want 30", len(cats))
	}
	seen := map[int]struct{}{}
	for _, c := range cats {
		if c.ID <= 0 || c.NameFA == "" || c.NameEN == "" {
			t.Fatalf("invalid category entry: %+v", c)
		}
		if _, dup := seen[c.ID]; dup {
			t.Fatalf("duplicate category id %d", c.ID)
		}
		seen[c.ID] = struct{}{}
	}
	tools, ok := publisher.FindCategoryByID(ctx, 11)
	if !ok || tools.NameEN != "Tools" || tools.Group != publisher.CategoryGroupApp {
		t.Fatalf("unexpected category 11: %+v ok=%v", tools, ok)
	}
	strategy, ok := publisher.FindCategoryByID(ctx, 58)
	if !ok || strategy.NameEN != "Strategy" || strategy.Group != publisher.CategoryGroupGame {
		t.Fatalf("unexpected category 58: %+v ok=%v", strategy, ok)
	}
}
