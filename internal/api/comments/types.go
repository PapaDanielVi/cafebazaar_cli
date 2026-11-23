package comments

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	// DefaultBaseURL is the production base URL for CafeBazaar's Comments API.
	DefaultBaseURL = "https://sandoghche.cafebazaar.ir"
	// EndpointReviews lists user reviews for an app.
	EndpointReviews = "/api/v0/reviews/"
	// EndpointReviewReplyFmt formats the endpoint for replying to a specific review.
	EndpointReviewReplyFmt = "/api/v0/reviews/%d/"

	// QueryPublishDateAfter filters reviews published after an ISO 8601 timestamp.
	QueryPublishDateAfter = "publish_date_after"
	// QueryPublishDateBefore filters reviews published before an ISO 8601 timestamp.
	QueryPublishDateBefore = "publish_date_before"
	// QueryRateMin filters reviews with rating greater than or equal to the value.
	QueryRateMin = "rate_min"
	// QueryRateMax filters reviews with rating less than or equal to the value.
	QueryRateMax = "rate_max"
	// QueryAppVersionCode filters reviews by application version code.
	QueryAppVersionCode = "app_version_code"
	// QuerySDKVersion filters reviews by Android SDK version.
	QuerySDKVersion = "sdk_version"
	// QueryIsReplied filters reviews by developer reply presence.
	QueryIsReplied = "is_replied"
	// QueryCursor selects a specific pagination cursor.
	QueryCursor = "cursor"

	// MinRating is the lowest possible star rating on CafeBazaar.
	MinRating = 1
	// MaxRating is the highest possible star rating on CafeBazaar.
	MaxRating = 5
)

var (
	// ErrInvalidRating indicates a rating filter outside 1 to 5.
	ErrInvalidRating = errors.New("rating must be between 1 and 5")
	// ErrInvalidRatingRange indicates that rate_min exceeds rate_max.
	ErrInvalidRatingRange = errors.New("rate_min must not be greater than rate_max")
	// ErrInvalidReviewID indicates a non-positive review identifier.
	ErrInvalidReviewID = errors.New("review id must be positive")
	// ErrEmptyReplyText indicates an empty developer reply message.
	ErrEmptyReplyText = errors.New("reply text must not be empty")
)

// ListReviewsFilter defines optional filters for querying CafeBazaar user reviews.
type ListReviewsFilter struct {
	PublishDateAfter  string
	PublishDateBefore string
	RateMin           int
	RateMax           int
	AppVersionCode    int
	SDKVersion        int
	IsReplied         *bool
	Cursor            string
}

// Validate checks filter constraints and encodes them into URL query parameters.
func (f ListReviewsFilter) ToQuery(ctx context.Context) (url.Values, error) {
	_ = ctx
	if f.RateMin != 0 && (f.RateMin < MinRating || f.RateMin > MaxRating) {
		return nil, ErrInvalidRating
	}
	if f.RateMax != 0 && (f.RateMax < MinRating || f.RateMax > MaxRating) {
		return nil, ErrInvalidRating
	}
	if f.RateMin != 0 && f.RateMax != 0 && f.RateMin > f.RateMax {
		return nil, ErrInvalidRatingRange
	}

	q := url.Values{}
	if v := strings.TrimSpace(f.PublishDateAfter); v != "" {
		q.Set(QueryPublishDateAfter, v)
	}
	if v := strings.TrimSpace(f.PublishDateBefore); v != "" {
		q.Set(QueryPublishDateBefore, v)
	}
	if f.RateMin != 0 {
		q.Set(QueryRateMin, strconv.Itoa(f.RateMin))
	}
	if f.RateMax != 0 {
		q.Set(QueryRateMax, strconv.Itoa(f.RateMax))
	}
	if f.AppVersionCode != 0 {
		q.Set(QueryAppVersionCode, strconv.Itoa(f.AppVersionCode))
	}
	if f.SDKVersion != 0 {
		q.Set(QuerySDKVersion, strconv.Itoa(f.SDKVersion))
	}
	if f.IsReplied != nil {
		q.Set(QueryIsReplied, strconv.FormatBool(*f.IsReplied))
	}
	if v := strings.TrimSpace(f.Cursor); v != "" {
		q.Set(QueryCursor, v)
	}
	return q, nil
}

// DeveloperReply describes a developer response attached to a user review.
type DeveloperReply struct {
	Text        string `json:"text"`
	Email       string `json:"email"`
	Likes       int    `json:"likes"`
	Dislikes    int    `json:"dislikes"`
	PublishDate string `json:"publish_date"`
}

// Review describes a single CafeBazaar user comment and rating.
type Review struct {
	ID             int64           `json:"id"`
	UserAccountID  string          `json:"user_account_id"`
	SDKVersion     int             `json:"sdk_version"`
	Likes          int             `json:"likes"`
	Dislikes       int             `json:"dislikes"`
	AppVersionCode int             `json:"app_version_code"`
	PublishDate    string          `json:"publish_date"`
	IsEdited       bool            `json:"is_edited"`
	Text           string          `json:"text"`
	Rate           int             `json:"rate"`
	DeveloperReply *DeveloperReply `json:"developer_reply"`
}

// ReviewsPage represents a paginated response from the Comments API.
type ReviewsPage struct {
	Next     *string  `json:"next"`
	Previous *string  `json:"previous"`
	Results  []Review `json:"results"`
}

// ReplyRequest defines the JSON payload for replying to a user review.
type ReplyRequest struct {
	Text  string `json:"text"`
	Email string `json:"email,omitempty"`
}

// Validate checks that a ReplyRequest has non-empty reply text.
func (r ReplyRequest) Validate(ctx context.Context) error {
	_ = ctx
	if strings.TrimSpace(r.Text) == "" {
		return ErrEmptyReplyText
	}
	return nil
}

// ReplyResponse represents the response returned after submitting a review reply.
type ReplyResponse struct {
	Text  string `json:"text"`
	Email string `json:"email"`
}

// FormatReplyEndpoint builds the path for replying to a specific review ID.
func FormatReplyEndpoint(ctx context.Context, reviewID int64) (string, error) {
	_ = ctx
	if reviewID <= 0 {
		return "", ErrInvalidReviewID
	}
	return fmt.Sprintf(EndpointReviewReplyFmt, reviewID), nil
}
