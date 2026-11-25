package pishkhan

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
)

const (
	// DefaultBundlePollInterval follows CafeBazaar's guideline of at most one request per minute.
	DefaultBundlePollInterval = 60 * time.Second
)

// Client interacts with CafeBazaar's Pishkhan Release API using an app-specific secret token.
type Client struct {
	http *httpclient.Client
}

// Config configures a Pishkhan Release API client.
type Config struct {
	BaseURL       string
	Token         string
	ClientAppName string
	CLIVersion    string
	ProxyURL      string
	HTTPClient    *http.Client
	MaxRetries    int
	RetryWait     time.Duration
	DebugLog      httpclient.DebugLogFunc
}

// New creates a new Pishkhan Release API client.
func New(ctx context.Context, cfg Config) (*Client, error) {
	baseURL := cfg.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	client, err := httpclient.New(ctx, httpclient.Config{
		BaseURL:        baseURL,
		Token:          cfg.Token,
		AuthHeaderName: httpclient.HeaderPishkhanSecret,
		ClientAppName:  cfg.ClientAppName,
		CLIVersion:     cfg.CLIVersion,
		ProxyURL:       cfg.ProxyURL,
		HTTPClient:     cfg.HTTPClient,
		MaxRetries:     cfg.MaxRetries,
		RetryWait:      cfg.RetryWait,
		DebugLog:       cfg.DebugLog,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize pishkhan client: %w", err)
	}
	return &Client{http: client}, nil
}

// CreateRelease creates a new uncommitted release on CafeBazaar.
func (c *Client) CreateRelease(ctx context.Context) (ReleaseResponse, error) {
	var resp ReleaseResponse
	if _, err := c.http.DoJSON(ctx, http.MethodPost, EndpointReleases, nil, nil, &resp); err != nil {
		return ReleaseResponse{}, fmt.Errorf("create release: %w", err)
	}
	resp.Exists = resp.Type == ResponseTypeSuccess
	return resp, nil
}

// DeleteRelease deletes the current uncommitted release on CafeBazaar.
func (c *Client) DeleteRelease(ctx context.Context) (ActionResponse, error) {
	var resp ActionResponse
	if _, err := c.http.DoJSON(ctx, http.MethodDelete, EndpointReleases, nil, nil, &resp); err != nil {
		return ActionResponse{}, fmt.Errorf("delete release: %w", err)
	}
	return resp, nil
}

// UploadAPK uploads an APK package with the given CPU architecture to the uncommitted release.
func (c *Client) UploadAPK(ctx context.Context, req UploadAPKRequest, onProgress httpclient.ProgressFunc) (UploadAPKResponse, error) {
	arch := req.Architecture
	if arch == "" {
		arch = ArchitectureAll
	}
	if err := ValidateArchitecture(ctx, arch); err != nil {
		return UploadAPKResponse{}, err
	}

	var resp UploadAPKResponse
	fields := map[string]string{
		FormFieldArchitecture: string(arch),
	}
	if _, err := c.http.DoMultipart(ctx, EndpointUploadAPK, fields, FormFieldAPK, req.FilePath, httpclient.ContentTypeAPK, onProgress, &resp); err != nil {
		return UploadAPKResponse{}, fmt.Errorf("upload apk: %w", err)
	}
	return resp, nil
}

// UploadAAB uploads an Android App Bundle file to the uncommitted release.
func (c *Client) UploadAAB(ctx context.Context, filePath string, onProgress httpclient.ProgressFunc) (UploadAABResponse, error) {
	var resp UploadAABResponse
	if _, err := c.http.DoMultipart(ctx, EndpointUploadAAB, nil, FormFieldAAB, filePath, httpclient.ContentTypeOctetStream, onProgress, &resp); err != nil {
		return UploadAABResponse{}, fmt.Errorf("upload aab: %w", err)
	}
	return resp, nil
}

// UploadBin uploads the Bundle Signer .bin file associated with an uploaded AAB.
func (c *Client) UploadBin(ctx context.Context, filePath string, onProgress httpclient.ProgressFunc) (BundleStatusResponse, error) {
	var resp BundleStatusResponse
	if _, err := c.http.DoMultipart(ctx, EndpointUploadBin, nil, FormFieldBinFile, filePath, httpclient.ContentTypeOctetStream, onProgress, &resp); err != nil {
		return BundleStatusResponse{}, fmt.Errorf("upload bin file: %w", err)
	}
	return resp, nil
}

// BundleStatus retrieves the current AAB processing state from CafeBazaar.
func (c *Client) BundleStatus(ctx context.Context) (BundleStatusResponse, error) {
	var resp BundleStatusResponse
	if _, err := c.http.DoJSON(ctx, http.MethodGet, EndpointBundleStatus, nil, nil, &resp); err != nil {
		return BundleStatusResponse{}, fmt.Errorf("fetch bundle status: %w", err)
	}
	return resp, nil
}

// WaitForBundle polls BundleStatus at interval until the bundle reaches Done or Error.
func (c *Client) WaitForBundle(
	ctx context.Context,
	interval time.Duration,
	onUpdate func(context.Context, BundleInfo) error,
) (BundleInfo, error) {
	if interval <= 0 {
		interval = DefaultBundlePollInterval
	}

	for {
		statusResp, err := c.BundleStatus(ctx)
		if err != nil {
			return BundleInfo{}, err
		}

		if onUpdate != nil {
			if err := onUpdate(ctx, statusResp.Bundle); err != nil {
				return statusResp.Bundle, err
			}
		}

		switch statusResp.Bundle.State {
		case BundleStateDone:
			return statusResp.Bundle, nil
		case BundleStateError:
			if len(statusResp.Bundle.Errors) > 0 {
				return statusResp.Bundle, fmt.Errorf("%w: %s", ErrBundleFailed, strings.Join(statusResp.Bundle.Errors, "; "))
			}
			return statusResp.Bundle, ErrBundleFailed
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return statusResp.Bundle, fmt.Errorf("wait for bundle processing: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// CommitRelease submits the uncommitted release to CafeBazaar for review with a 10-minute publish timeout.
func (c *Client) CommitRelease(ctx context.Context, req CommitReleaseRequest) (ReleaseResponse, error) {
	if req.StagedRolloutPercentage != 0 &&
		(req.StagedRolloutPercentage < MinRolloutPercentage || req.StagedRolloutPercentage > MaxRolloutPercentage) {
		return ReleaseResponse{}, ErrInvalidRollout
	}

	var resp ReleaseResponse
	if _, err := c.http.DoJSONWithTimeout(ctx, httpclient.DefaultPublishTimeout, http.MethodPost, EndpointCommit, nil, req, &resp); err != nil {
		return ReleaseResponse{}, fmt.Errorf("commit release: %w", err)
	}
	resp.Exists = resp.Type == ResponseTypeSuccess
	return resp, nil
}

// LastCommittedRelease fetches the most recent committed release for the app.
func (c *Client) LastCommittedRelease(ctx context.Context) (ReleaseResponse, error) {
	return c.fetchReleaseByEndpoint(ctx, EndpointLastCommitted, "last committed release")
}

// LastUncommittedRelease fetches the current uncommitted release for the app.
func (c *Client) LastUncommittedRelease(ctx context.Context) (ReleaseResponse, error) {
	return c.fetchReleaseByEndpoint(ctx, EndpointLastUncommitted, "last uncommitted release")
}

// LastPublishedRelease fetches the currently published release for the app.
func (c *Client) LastPublishedRelease(ctx context.Context) (ReleaseResponse, error) {
	return c.fetchReleaseByEndpoint(ctx, EndpointLastPublished, "last published release")
}

func (c *Client) fetchReleaseByEndpoint(ctx context.Context, endpoint, label string) (ReleaseResponse, error) {
	var resp ReleaseResponse
	if _, err := c.http.DoJSON(ctx, http.MethodGet, endpoint, nil, nil, &resp); err != nil {
		return ReleaseResponse{}, fmt.Errorf("fetch %s: %w", label, err)
	}
	resp.Exists = resp.Type == ResponseTypeSuccess && resp.Release != nil
	return resp, nil
}
