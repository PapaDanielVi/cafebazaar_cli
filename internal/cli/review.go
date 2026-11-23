package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/comments"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

func newReviewCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	revCmd := &cobra.Command{
		Use:   "review",
		Short: "Read and reply to CafeBazaar user reviews (Sandoghche Comments API)",
	}
	revCmd.AddCommand(
		newReviewListCmd(ctx, rt),
		newReviewReplyCmd(ctx, rt),
	)
	return revCmd
}

func newReviewListCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var after string
	var before string
	var rateMin int
	var rateMax int
	var versionCode int
	var sdkVersion int
	var repliedOnly bool
	var unrepliedOnly bool
	var cursor string
	var fetchAll bool
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List user reviews with optional filters (GET /api/v0/reviews/)",
		RunE: func(c *cobra.Command, args []string) error {
			_ = args
			if repliedOnly && unrepliedOnly {
				return errors.New("cannot use both --replied and --unreplied together")
			}
			var isReplied *bool
			if repliedOnly {
				v := true
				isReplied = &v
			} else if unrepliedOnly {
				v := false
				isReplied = &v
			}

			filter := comments.ListReviewsFilter{
				PublishDateAfter:  after,
				PublishDateBefore: before,
				RateMin:           rateMin,
				RateMax:           rateMax,
				AppVersionCode:    versionCode,
				SDKVersion:        sdkVersion,
				IsReplied:         isReplied,
				Cursor:            cursor,
			}
			return runReviewList(c.Context(), rt, filter, fetchAll, limit)
		},
	}

	cmd.Flags().StringVar(&after, "after", "", "Filter reviews published after ISO 8601 timestamp (publish_date_after)")
	cmd.Flags().StringVar(&before, "before", "", "Filter reviews published before ISO 8601 timestamp (publish_date_before)")
	cmd.Flags().IntVar(&rateMin, "rate-min", 0, "Minimum star rating (1 to 5)")
	cmd.Flags().IntVar(&rateMax, "rate-max", 0, "Maximum star rating (1 to 5)")
	cmd.Flags().IntVar(&versionCode, "version-code", 0, "Filter by app_version_code")
	cmd.Flags().IntVar(&sdkVersion, "sdk-version", 0, "Filter by Android sdk_version")
	cmd.Flags().BoolVar(&repliedOnly, "replied", false, "Only show reviews that have a developer reply")
	cmd.Flags().BoolVar(&unrepliedOnly, "unreplied", false, "Only show reviews without a developer reply")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Pagination cursor")
	cmd.Flags().BoolVar(&fetchAll, "all", false, "Follow cursor pagination across multiple pages")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of reviews to return when using --all")
	return cmd
}

func runReviewList(ctx context.Context, rt *AppRuntime, filter comments.ListReviewsFilter, fetchAll bool, limit int) error {
	resolved, err := requireInitializedApp(ctx, rt, "")
	if err != nil {
		return err
	}
	client, err := rt.newCommentsClient(ctx, resolved.Token)
	if err != nil {
		return err
	}

	var reviews []comments.Review
	var nextCursor string
	if err := rt.Printer.WithSpinner(ctx, "Fetching user reviews from CafeBazaar", func(spinCtx context.Context) error {
		if fetchAll || limit > 0 {
			var listErr error
			reviews, listErr = client.ListAllReviews(spinCtx, filter, limit)
			return listErr
		}
		page, pageErr := client.ListReviews(spinCtx, filter)
		if pageErr != nil {
			return pageErr
		}
		reviews = page.Results
		if page.Next != nil {
			nextCursor = *page.Next
		}
		return nil
	}); err != nil {
		return err
	}

	payload := map[string]any{
		"package_name": resolved.PackageName,
		"count":        len(reviews),
		"next":         nextCursor,
		"results":      reviews,
	}
	if printed, pErr := rt.Printer.PrintStructured(ctx, payload); printed || pErr != nil {
		return pErr
	}

	if len(reviews) == 0 {
		return rt.Printer.PrintInfo(ctx, "No reviews matched your filters.")
	}

	for _, rev := range reviews {
		statusBadge := rt.Printer.Theme.RenderBadge(ctx, "UNREPLIED")
		if rev.DeveloperReply != nil {
			statusBadge = rt.Printer.Theme.RenderBadge(ctx, "REPLIED")
		}
		header := fmt.Sprintf("#%d  %s  %s  (vCode %d, SDK %d, %s)",
			rev.ID,
			rt.Printer.Theme.RenderStars(ctx, rev.Rate),
			statusBadge,
			rev.AppVersionCode,
			rev.SDKVersion,
			rev.PublishDate,
		)
		items := []output.KeyValue{
			{Key: "Comment", Value: rev.Text},
			{Key: "Votes", Value: fmt.Sprintf("+%d / -%d", rev.Likes, rev.Dislikes)},
		}
		if rev.DeveloperReply != nil {
			items = append(items, output.KeyValue{
				Key:   "Reply",
				Value: rev.DeveloperReply.Text,
			})
		}
		if err := rt.Printer.PrintCard(ctx, header, items); err != nil {
			return err
		}
	}

	if nextCursor != "" {
		return rt.Printer.PrintInfo(ctx, "More reviews available. Re-run with --all to fetch subsequent pages.")
	}
	return nil
}

func newReviewReplyCmd(ctx context.Context, rt *AppRuntime) *cobra.Command {
	_ = ctx
	var textFlag string
	var textFile string
	var emailFlag string

	cmd := &cobra.Command{
		Use:   "reply <review-id>",
		Short: "Reply to a user review (POST /api/v0/reviews/{id}/)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			reviewID, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
			if err != nil {
				return fmt.Errorf("invalid review id %q: %w", args[0], err)
			}
			return runReviewReply(c.Context(), rt, reviewID, textFlag, textFile, emailFlag)
		},
	}

	cmd.Flags().StringVar(&textFlag, "text", "", "Developer reply text (UTF-8 Persian or English)")
	cmd.Flags().StringVar(&textFile, "text-file", "", "Read UTF-8 developer reply text from a file")
	cmd.Flags().StringVar(&emailFlag, "email", "", "Optional developer display email (defaults to CafeBazaar account email)")
	return cmd
}

func runReviewReply(ctx context.Context, rt *AppRuntime, reviewID int64, textFlag, textFile, emailFlag string) error {
	resolved, err := requireInitializedApp(ctx, rt, "")
	if err != nil {
		return err
	}

	text, err := readUTF8TextFlagOrFile(ctx, textFlag, textFile, "")
	if err != nil {
		return err
	}
	if text == "" && rt.Printer.Mode.Interactive {
		text, err = rt.Printer.MultilineInput(ctx, fmt.Sprintf("Reply to Review #%d", reviewID), "UTF-8 Persian or English reply (published after CafeBazaar moderation).", "")
		if err != nil {
			return err
		}
	}
	req := comments.ReplyRequest{
		Text:  text,
		Email: strings.TrimSpace(emailFlag),
	}
	if err := req.Validate(ctx); err != nil {
		return err
	}

	if _, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
		Title:     fmt.Sprintf("Post Public Developer Reply to Review #%d", reviewID),
		TargetApp: resolved.PackageName,
		Warning:   "Developer replies cannot be deleted via the API once submitted and will be shown publicly after CafeBazaar approval.",
		Details: []output.KeyValue{
			{Key: "Review ID", Value: strconv.FormatInt(reviewID, 10)},
			{Key: "Reply", Value: output.TruncateRunes(ctx, req.Text, 80)},
			{Key: "Email", Value: fallbackDash(ctx, req.Email)},
		},
		PromptLabel: "Submit this reply to CafeBazaar?",
	}); err != nil {
		return err
	}

	client, err := rt.newCommentsClient(ctx, resolved.Token)
	if err != nil {
		return err
	}
	var resp comments.ReplyResponse
	if err := rt.Printer.WithSpinner(ctx, "Submitting developer reply", func(spinCtx context.Context) error {
		var callErr error
		resp, callErr = client.ReplyReview(spinCtx, reviewID, req)
		return callErr
	}); err != nil {
		return err
	}

	if printed, pErr := rt.Printer.PrintStructured(ctx, resp); printed || pErr != nil {
		return pErr
	}
	return rt.Printer.PrintSuccess(ctx, fmt.Sprintf("Reply submitted for review #%d (pending CafeBazaar approval).", reviewID))
}
