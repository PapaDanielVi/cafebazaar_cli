package publisher

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
)

const (
	extAAB  = ".aab"
	extPNG  = ".png"
	extJPG  = ".jpg"
	extJPEG = ".jpeg"
	extWEBP = ".webp"

	mimePNG  = "image/png"
	mimeJPEG = "image/jpeg"
	mimeWEBP = "image/webp"
)

// Client interacts with CafeBazaar's developer-level Nasher Publisher API.
type Client struct {
	http *httpclient.Client
}

// Config configures a Nasher Publisher API client.
type Config struct {
	BaseURL        string
	Token          string
	AuthHeaderName string
	ClientAppName  string
	CLIVersion     string
	ProxyURL       string
	HTTPClient     *http.Client
	MaxRetries     int
	RetryWait      time.Duration
}

// New creates a new Nasher Publisher API client.
func New(ctx context.Context, cfg Config) (*Client, error) {
	baseURL := cfg.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	headerName := cfg.AuthHeaderName
	if strings.TrimSpace(headerName) == "" {
		headerName = httpclient.HeaderPishkhanSecret
	}
	client, err := httpclient.New(ctx, httpclient.Config{
		BaseURL:        baseURL,
		Token:          cfg.Token,
		AuthHeaderName: headerName,
		ClientAppName:  cfg.ClientAppName,
		CLIVersion:     cfg.CLIVersion,
		ProxyURL:       cfg.ProxyURL,
		HTTPClient:     cfg.HTTPClient,
		MaxRetries:     cfg.MaxRetries,
		RetryWait:      cfg.RetryWait,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize publisher client: %w", err)
	}
	return &Client{http: client}, nil
}

// CreateApp submits a new app and its initial package using the developer Nasher token.
func (c *Client) CreateApp(ctx context.Context, req CreateAppRequest, onProgress httpclient.ProgressFunc) (CreateAppResponse, error) {
	if err := req.Validate(ctx); err != nil {
		return CreateAppResponse{}, fmt.Errorf("validate create app request: %w", err)
	}

	fields := make(map[string]string, len(req.ExtraFields)+12)
	for k, v := range req.ExtraFields {
		fields[k] = v
	}
	if strings.TrimSpace(req.PackageName) != "" {
		fields[FieldPackageName] = strings.TrimSpace(req.PackageName)
	}
	fields[FieldCategoryID] = strconv.Itoa(req.CategoryID)
	fields[FieldNameFA] = strings.TrimSpace(req.NameFA)
	if strings.TrimSpace(req.NameEN) != "" {
		fields[FieldNameEN] = strings.TrimSpace(req.NameEN)
	}
	fields[FieldShortDescriptionFA] = strings.TrimSpace(req.ShortDescriptionFA)
	if strings.TrimSpace(req.ShortDescriptionEN) != "" {
		fields[FieldShortDescriptionEN] = strings.TrimSpace(req.ShortDescriptionEN)
	}
	fields[FieldDescriptionFA] = strings.TrimSpace(req.DescriptionFA)
	if strings.TrimSpace(req.DescriptionEN) != "" {
		fields[FieldDescriptionEN] = strings.TrimSpace(req.DescriptionEN)
	}
	fields[FieldEmail] = strings.TrimSpace(req.Email)
	if strings.TrimSpace(req.Phone) != "" {
		fields[FieldPhone] = strings.TrimSpace(req.Phone)
	}
	if strings.TrimSpace(req.Website) != "" {
		fields[FieldWebsite] = strings.TrimSpace(req.Website)
	}
	if req.AutoPublish != nil {
		fields[FieldAutoPublish] = strconv.FormatBool(*req.AutoPublish)
	}
	fields[FieldIssueAPIAgent] = strconv.FormatBool(req.IssueAPIAgent)

	artifactField := strings.TrimSpace(req.ArtifactField)
	artifactContentType := httpclient.ContentTypeAPK
	if strings.EqualFold(filepath.Ext(req.ArtifactPath), extAAB) {
		if artifactField == "" {
			artifactField = FieldAAB
		}
		artifactContentType = httpclient.ContentTypeOctetStream
	} else if artifactField == "" {
		artifactField = FieldAPK
	}

	files := make([]httpclient.MultipartFile, 0, 2+len(req.ScreenshotPaths))
	files = append(files,
		httpclient.MultipartFile{
			FieldName:   artifactField,
			FilePath:    strings.TrimSpace(req.ArtifactPath),
			ContentType: artifactContentType,
		},
		httpclient.MultipartFile{
			FieldName:   FieldIcon,
			FilePath:    strings.TrimSpace(req.IconPath),
			ContentType: detectImageContentType(ctx, req.IconPath),
		},
	)
	for _, shotPath := range req.ScreenshotPaths {
		files = append(files, httpclient.MultipartFile{
			FieldName:   FieldScreenshots,
			FilePath:    strings.TrimSpace(shotPath),
			ContentType: detectImageContentType(ctx, shotPath),
		})
	}

	var raw map[string]any
	if _, err := c.http.DoMultipartFiles(ctx, EndpointApps, fields, files, onProgress, &raw); err != nil {
		return CreateAppResponse{}, fmt.Errorf("create app via publisher api: %w", err)
	}

	parsed := ParseCreateAppResponse(ctx, raw)
	if parsed.PackageName == "" {
		parsed.PackageName = strings.TrimSpace(req.PackageName)
	}
	return parsed, nil
}

func detectImageContentType(ctx context.Context, path string) string {
	_ = ctx
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case extPNG:
		return mimePNG
	case extJPG, extJPEG:
		return mimeJPEG
	case extWEBP:
		return mimeWEBP
	default:
		return httpclient.ContentTypeOctetStream
	}
}
