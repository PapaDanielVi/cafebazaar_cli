package comments

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/httpclient"
)

const (
	defaultMaxPages = 50
)

// Client interacts with CafeBazaar's Sandoghche Comments API using an app Pishkhan token.
type Client struct {
	http *httpclient.Client
}

// Config configures a Comments API client.
type Config struct {
	BaseURL       string
	Token         string
	ClientAppName string
	CLIVersion    string
	ProxyURL      string
	HTTPClient    *http.Client
	MaxRetries    int
	RetryWait     time.Duration
}

// New creates a new Comments API client.
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
	})
	if err != nil {
		return nil, fmt.Errorf("initialize comments client: %w", err)
	}
	return &Client{http: client}, nil
}

// ListReviews fetches a single page of user reviews matching filter.
func (c *Client) ListReviews(ctx context.Context, filter ListReviewsFilter) (ReviewsPage, error) {
	query, err := filter.ToQuery(ctx)
	if err != nil {
		return ReviewsPage{}, err
	}
	var page ReviewsPage
	if _, err := c.http.DoJSON(ctx, http.MethodGet, EndpointReviews, query, nil, &page); err != nil {
		return ReviewsPage{}, fmt.Errorf("list reviews: %w", err)
	}
	return page, nil
}

// ListAllReviews follows cursor pagination and collects reviews up to limit (0 means all pages up to safety cap).
func (c *Client) ListAllReviews(ctx context.Context, filter ListReviewsFilter, limit int) ([]Review, error) {
	page, err := c.ListReviews(ctx, filter)
	if err != nil {
		return nil, err
	}

	results := make([]Review, 0, len(page.Results))
	results = append(results, page.Results...)

	for pages := 1; page.Next != nil && strings.TrimSpace(*page.Next) != "" && pages < defaultMaxPages; pages++ {
		if limit > 0 && len(results) >= limit {
			break
		}
		var nextPage ReviewsPage
		if _, err := c.http.DoJSON(ctx, http.MethodGet, strings.TrimSpace(*page.Next), nil, nil, &nextPage); err != nil {
			return nil, fmt.Errorf("fetch next reviews page: %w", err)
		}
		results = append(results, nextPage.Results...)
		page = nextPage
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// ReplyReview posts a developer reply to a user review by reviewID.
func (c *Client) ReplyReview(ctx context.Context, reviewID int64, req ReplyRequest) (ReplyResponse, error) {
	if err := req.Validate(ctx); err != nil {
		return ReplyResponse{}, err
	}
	endpoint, err := FormatReplyEndpoint(ctx, reviewID)
	if err != nil {
		return ReplyResponse{}, err
	}

	var resp ReplyResponse
	if _, err := c.http.DoJSON(ctx, http.MethodPost, endpoint, nil, req, &resp); err != nil {
		return ReplyResponse{}, fmt.Errorf("reply to review %d: %w", reviewID, err)
	}
	return resp, nil
}
