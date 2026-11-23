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
	extAAB = ".aab"
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
	var raw map[string]any

	if strings.TrimSpace(req.ArtifactPath) != "" {
		fields := make(map[string]string, len(req.ExtraFields)+2)
		for k, v := range req.ExtraFields {
			fields[k] = v
		}
		if strings.TrimSpace(req.PackageName) != "" {
			fields[FieldPackageName] = strings.TrimSpace(req.PackageName)
		}
		fields[FieldIssueAPIAgent] = strconv.FormatBool(req.IssueAPIAgent)

		fileField := strings.TrimSpace(req.ArtifactField)
		contentType := httpclient.ContentTypeAPK
		if strings.EqualFold(filepath.Ext(req.ArtifactPath), extAAB) {
			if fileField == "" {
				fileField = FieldAAB
			}
			contentType = httpclient.ContentTypeOctetStream
		} else if fileField == "" {
			fileField = FieldAPK
		}

		if _, err := c.http.DoMultipart(ctx, EndpointApps, fields, fileField, req.ArtifactPath, contentType, onProgress, &raw); err != nil {
			return CreateAppResponse{}, fmt.Errorf("create app via publisher api: %w", err)
		}
	} else {
		payload := make(map[string]any, len(req.ExtraFields)+2)
		for k, v := range req.ExtraFields {
			payload[k] = v
		}
		if strings.TrimSpace(req.PackageName) != "" {
			payload[FieldPackageName] = strings.TrimSpace(req.PackageName)
		}
		payload[FieldIssueAPIAgent] = req.IssueAPIAgent

		if _, err := c.http.DoJSON(ctx, http.MethodPost, EndpointApps, nil, payload, &raw); err != nil {
			return CreateAppResponse{}, fmt.Errorf("create app via publisher api: %w", err)
		}
	}

	parsed := ParseCreateAppResponse(ctx, raw)
	if parsed.PackageName == "" {
		parsed.PackageName = strings.TrimSpace(req.PackageName)
	}
	return parsed, nil
}
