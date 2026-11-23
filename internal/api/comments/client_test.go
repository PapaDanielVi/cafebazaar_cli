package comments_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/comments"
)

const (
	testCommentsToken = "app-comments-token"
	testReplyText     = "ممنون از بازخوردتون، در نسخه جدید مشکل برطرف شده."
	testReplyEmail    = "dev@example.com"
)

func TestCommentsListFiltersAndPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var srvURL string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get(comments.QueryCursor) == "page2" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"next":null,"previous":"prev","results":[{"id":2,"rate":5,"text":"عالی بود"}]}`))
			return
		}
		gotQuery = r.URL.RawQuery
		nextURL := fmt.Sprintf("%s%s?cursor=page2", srvURL, comments.EndpointReviews)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"next":%q,"previous":null,"results":[{"id":1,"rate":4,"text":"اپ خوبیه"}]}`, nextURL)
	}))
	defer srv.Close()
	srvURL = srv.URL

	client, err := comments.New(ctx, comments.Config{
		BaseURL: srv.URL,
		Token:   testCommentsToken,
	})
	if err != nil {
		t.Fatalf("new comments client: %v", err)
	}

	isReplied := false
	all, err := client.ListAllReviews(ctx, comments.ListReviewsFilter{
		PublishDateAfter:  "2025-01-01T00:00:00",
		PublishDateBefore: "2025-12-31T23:59:59",
		RateMin:           3,
		RateMax:           5,
		AppVersionCode:    30200,
		SDKVersion:        33,
		IsReplied:         &isReplied,
	}, 0)
	if err != nil {
		t.Fatalf("list all reviews: %v", err)
	}
	if len(all) != 2 || all[0].ID != 1 || all[1].ID != 2 {
		t.Fatalf("unexpected reviews: %+v", all)
	}
	if gotQuery == "" {
		t.Fatal("expected query parameters on first page request")
	}
}

func TestCommentsReplyReview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var gotReq comments.ReplyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v0/reviews/122455833/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode reply body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"text":%q,"email":%q}`, gotReq.Text, gotReq.Email)
	}))
	defer srv.Close()

	client, err := comments.New(ctx, comments.Config{
		BaseURL: srv.URL,
		Token:   testCommentsToken,
	})
	if err != nil {
		t.Fatalf("new comments client: %v", err)
	}

	resp, err := client.ReplyReview(ctx, 122455833, comments.ReplyRequest{
		Text:  testReplyText,
		Email: testReplyEmail,
	})
	if err != nil {
		t.Fatalf("reply review: %v", err)
	}
	if resp.Text != testReplyText || resp.Email != testReplyEmail {
		t.Fatalf("unexpected reply response: %+v", resp)
	}
}

func TestCommentsValidation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		name    string
		filter  comments.ListReviewsFilter
		wantErr error
	}{
		{
			name:    "rate min too high",
			filter:  comments.ListReviewsFilter{RateMin: 6},
			wantErr: comments.ErrInvalidRating,
		},
		{
			name:    "rate min exceeds rate max",
			filter:  comments.ListReviewsFilter{RateMin: 4, RateMax: 2},
			wantErr: comments.ErrInvalidRatingRange,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tc.filter.ToQuery(ctx); !errors.Is(err, tc.wantErr) {
				t.Fatalf("got err=%v, want %v", err, tc.wantErr)
			}
		})
	}
}
