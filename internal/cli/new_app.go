package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/PapaDanielVi/cafebazaar_cli/internal/android"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/api/publisher"
	"github.com/PapaDanielVi/cafebazaar_cli/internal/output"
	"github.com/spf13/cobra"
)

const (
	flagCategoryID         = "category-id"
	flagNameFA             = "name-fa"
	flagNameEN             = "name-en"
	flagShortDescriptionFA = "short-description-fa"
	flagShortDescriptionEN = "short-description-en"
	flagDescriptionFA      = "description-fa"
	flagDescriptionFAFile  = "description-fa-file"
	flagDescriptionEN      = "description-en"
	flagDescriptionENFile  = "description-en-file"
	flagEmail              = "email"
	flagPhone              = "phone"
	flagWebsite            = "website"
	flagIcon               = "icon"
	flagScreenshot         = "screenshot"
	flagAutoPublish        = "auto-publish"
	customCategoryOption   = "__custom_category__"

	maxPromptRetries = 5
)

type newAppFlags struct {
	CategoryID         int
	NameFA             string
	NameEN             string
	ShortDescriptionFA string
	ShortDescriptionEN string
	DescriptionFA      string
	DescriptionFAFile  string
	DescriptionEN      string
	DescriptionENFile  string
	Email              string
	Phone              string
	Website            string
	IconPath           string
	Screenshots        []string
	Artifact           string
	AutoPublish        bool
	AutoPublishSet     bool
	ExtraFields        []string
}

func registerNewAppFlags(ctx context.Context, cmd *cobra.Command, rt *AppRuntime, f *newAppFlags) {
	cmd.Flags().StringVar(&f.Artifact, "artifact", "", "Initial APK file path for CafeBazaar review")
	cmd.Flags().IntVar(&f.CategoryID, flagCategoryID, 0, "CafeBazaar integer category ID (required for new apps)")
	cmd.Flags().StringVar(&f.NameFA, flagNameFA, "", fmt.Sprintf("Persian app name (required, max %d characters)", publisher.MaxAppNameRunes))
	cmd.Flags().StringVar(&f.NameEN, flagNameEN, "", fmt.Sprintf("English app name (optional, max %d characters)", publisher.MaxAppNameRunes))
	cmd.Flags().StringVar(&f.ShortDescriptionFA, flagShortDescriptionFA, "", fmt.Sprintf("Persian short description (required, max %d characters)", publisher.MaxShortDescRunes))
	cmd.Flags().StringVar(&f.ShortDescriptionEN, flagShortDescriptionEN, "", fmt.Sprintf("English short description (optional, max %d characters)", publisher.MaxShortDescRunes))
	cmd.Flags().StringVar(&f.DescriptionFA, flagDescriptionFA, "", "Persian full description (required)")
	cmd.Flags().StringVar(&f.DescriptionFAFile, flagDescriptionFAFile, "", "Read UTF-8 Persian full description from file")
	cmd.Flags().StringVar(&f.DescriptionEN, flagDescriptionEN, "", "English full description (optional)")
	cmd.Flags().StringVar(&f.DescriptionENFile, flagDescriptionENFile, "", "Read UTF-8 English full description from file")
	cmd.Flags().StringVar(&f.Email, flagEmail, "", "Support email address (required)")
	cmd.Flags().StringVar(&f.Phone, flagPhone, "", "Support phone number (optional)")
	cmd.Flags().StringVar(&f.Website, flagWebsite, "", "App or developer website URL (optional)")
	cmd.Flags().StringVar(&f.IconPath, flagIcon, "", fmt.Sprintf("Square app icon path (required, min %dx%d PNG/JPEG)", publisher.MinIconDimension, publisher.MinIconDimension))
	cmd.Flags().StringArrayVar(&f.Screenshots, flagScreenshot, nil, fmt.Sprintf("Screenshot image path or directory (%d to %d images, repeatable or comma-separated)", publisher.MinScreenshotCount, publisher.MaxScreenshotCount))
	cmd.Flags().BoolVar(&f.AutoPublish, flagAutoPublish, false, "Publish automatically after CafeBazaar approval")
	cmd.Flags().StringArrayVar(&f.ExtraFields, "field", nil, "Additional key=value field for Nasher POST /v1/apps/ (repeatable)")

	registerFlagCompletion(ctx, cmd, "artifact", completeArtifactPaths(rt, android.ArtifactTypeAPK, "apk"))
	registerFlagCompletion(ctx, cmd, flagCategoryID, completeCategoryIDs(ctx))
}

func collectCreateAppRequest(
	ctx context.Context,
	rt *AppRuntime,
	pkg string,
	mod android.AppModule,
	issueAgent bool,
	f newAppFlags,
) (publisher.CreateAppRequest, error) {
	extra, err := parseKeyValueFlags(ctx, f.ExtraFields)
	if err != nil {
		return publisher.CreateAppRequest{}, err
	}

	artifactPath := expandUserPath(ctx, strings.TrimSpace(f.Artifact))
	if artifactPath == "" && len(mod.Artifacts) > 0 {
		for _, a := range mod.Artifacts {
			if a.Type == android.ArtifactTypeAPK {
				artifactPath = a.Path
				break
			}
		}
	}
	if artifactPath == "" && rt.Printer.Mode.Interactive {
		artifactPath, err = promptValidatedInput(ctx, rt.Printer,
			"Initial APK Path",
			"Path to the signed release APK for CafeBazaar review.",
			"app/build/outputs/apk/release/app-release.apk",
			"",
			func(val string) (string, error) {
				expanded := expandUserPath(ctx, val)
				if err := publisher.ValidateRegularFile(ctx, expanded); err != nil {
					return "", err
				}
				return expanded, nil
			},
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	categoryID := f.CategoryID
	if categoryID <= 0 && rt.Printer.Mode.Interactive {
		var pErr error
		categoryID, pErr = SelectCategoryID(ctx, rt.Printer)
		if pErr != nil {
			return publisher.CreateAppRequest{}, pErr
		}
	}

	nameFA := strings.TrimSpace(f.NameFA)
	if nameFA == "" && rt.Printer.Mode.Interactive {
		nameFA, err = promptValidatedInput(ctx, rt.Printer,
			fmt.Sprintf("Persian App Name (name_fa, max %d chars)", publisher.MaxAppNameRunes),
			"Required display name shown to CafeBazaar users in Persian.",
			"نام فارسی برنامه",
			"",
			func(val string) (string, error) {
				if err := publisher.ValidateAppName(ctx, val, true); err != nil {
					return "", err
				}
				return strings.TrimSpace(val), nil
			},
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	nameEN := strings.TrimSpace(f.NameEN)
	if nameEN == "" && rt.Printer.Mode.Interactive {
		nameEN, err = promptValidatedInput(ctx, rt.Printer,
			fmt.Sprintf("English App Name (name_en, optional, max %d chars)", publisher.MaxAppNameRunes),
			"Optional English display name. Leave blank to skip.",
			"My Android App",
			"",
			func(val string) (string, error) {
				if err := publisher.ValidateAppName(ctx, val, false); err != nil {
					return "", err
				}
				return strings.TrimSpace(val), nil
			},
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	shortDescFA := strings.TrimSpace(f.ShortDescriptionFA)
	if shortDescFA == "" && rt.Printer.Mode.Interactive {
		shortDescFA, err = promptValidatedInput(ctx, rt.Printer,
			fmt.Sprintf("Persian Short Description (short_description_fa, max %d chars)", publisher.MaxShortDescRunes),
			"Required short summary shown on CafeBazaar app listing.",
			"توضیح کوتاه برنامه",
			"",
			func(val string) (string, error) {
				if err := publisher.ValidateShortDescription(ctx, val, true); err != nil {
					return "", err
				}
				return strings.TrimSpace(val), nil
			},
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	shortDescEN := strings.TrimSpace(f.ShortDescriptionEN)
	if shortDescEN == "" && rt.Printer.Mode.Interactive {
		shortDescEN, err = promptValidatedInput(ctx, rt.Printer,
			fmt.Sprintf("English Short Description (short_description_en, optional, max %d chars)", publisher.MaxShortDescRunes),
			"Optional English short summary. Leave blank to skip.",
			"Short summary of the app",
			"",
			func(val string) (string, error) {
				if err := publisher.ValidateShortDescription(ctx, val, false); err != nil {
					return "", err
				}
				return strings.TrimSpace(val), nil
			},
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	descFA, err := readUTF8TextFlagOrFile(ctx, f.DescriptionFA, f.DescriptionFAFile, "")
	if err != nil {
		return publisher.CreateAppRequest{}, err
	}
	if descFA == "" && rt.Printer.Mode.Interactive {
		descFA, err = promptRequiredMultiline(ctx, rt.Printer,
			"Persian Full Description (description_fa)",
			"Required detailed description shown on CafeBazaar (UTF-8).",
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	descEN, err := readUTF8TextFlagOrFile(ctx, f.DescriptionEN, f.DescriptionENFile, "")
	if err != nil {
		return publisher.CreateAppRequest{}, err
	}
	if descEN == "" && rt.Printer.Mode.Interactive {
		descEN, err = rt.Printer.MultilineInput(ctx,
			"English Full Description (description_en, optional)",
			"Optional English description. Leave blank to skip.",
			"",
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	email := strings.TrimSpace(f.Email)
	if email == "" && rt.Printer.Mode.Interactive {
		email, err = promptValidatedInput(ctx, rt.Printer,
			"Support Email (email)",
			"Required support email address displayed on CafeBazaar.",
			"support@example.com",
			"",
			func(val string) (string, error) {
				if err := publisher.ValidateEmail(ctx, val); err != nil {
					return "", err
				}
				return strings.TrimSpace(val), nil
			},
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	phone := strings.TrimSpace(f.Phone)
	if phone == "" && rt.Printer.Mode.Interactive {
		phone, err = rt.Printer.Input(ctx,
			"Support Phone (phone, optional)",
			"Optional support phone number. Leave blank to skip.",
			"",
			"",
			false,
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	website := strings.TrimSpace(f.Website)
	if website == "" && rt.Printer.Mode.Interactive {
		website, err = rt.Printer.Input(ctx,
			"Website URL (website, optional)",
			"Optional app or developer website URL. Leave blank to skip.",
			"https://example.com",
			"",
			false,
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	iconPath := expandUserPath(ctx, strings.TrimSpace(f.IconPath))
	if iconPath == "" && rt.Printer.Mode.Interactive {
		iconPath, err = promptValidatedInput(ctx, rt.Printer,
			fmt.Sprintf("App Icon Path (icon, square min %dx%d)", publisher.MinIconDimension, publisher.MinIconDimension),
			"Required square PNG/JPEG/WebP icon with minimum dimensions 512x512.",
			"assets/icon-512.png",
			"",
			func(val string) (string, error) {
				expanded := expandUserPath(ctx, val)
				if err := publisher.ValidateIconFile(ctx, expanded); err != nil {
					return "", err
				}
				return expanded, nil
			},
		)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	screenshots, err := expandScreenshotInputs(ctx, f.Screenshots)
	if err != nil {
		return publisher.CreateAppRequest{}, err
	}
	if len(screenshots) == 0 && rt.Printer.Mode.Interactive {
		screenshots, err = promptScreenshots(ctx, rt.Printer)
		if err != nil {
			return publisher.CreateAppRequest{}, err
		}
	}

	var autoPubPtr *bool
	if f.AutoPublishSet {
		val := f.AutoPublish
		autoPubPtr = &val
	}

	req := publisher.CreateAppRequest{
		PackageName:        pkg,
		CategoryID:         categoryID,
		NameFA:             nameFA,
		NameEN:             nameEN,
		ShortDescriptionFA: shortDescFA,
		ShortDescriptionEN: shortDescEN,
		DescriptionFA:      descFA,
		DescriptionEN:      descEN,
		Email:              email,
		Phone:              strings.TrimSpace(phone),
		Website:            strings.TrimSpace(website),
		IconPath:           iconPath,
		ScreenshotPaths:    screenshots,
		AutoPublish:        autoPubPtr,
		IssueAPIAgent:      issueAgent,
		ArtifactPath:       artifactPath,
		ExtraFields:        extra,
	}
	if err := req.Validate(ctx); err != nil {
		return publisher.CreateAppRequest{}, err
	}
	return req, nil
}

func confirmCreateAppRequest(ctx context.Context, rt *AppRuntime, req publisher.CreateAppRequest) error {
	categoryDisplay := strconv.Itoa(req.CategoryID)
	if cat, ok := publisher.FindCategoryByID(ctx, req.CategoryID); ok {
		categoryDisplay = fmt.Sprintf("%d (%s - %s)", cat.ID, cat.NameEN, cat.NameFA)
	}
	details := []output.KeyValue{
		{Key: "Endpoint", Value: "POST " + publisher.EndpointApps},
		{Key: "Category", Value: categoryDisplay},
		{Key: "Name (FA)", Value: req.NameFA},
		{Key: "Short Desc (FA)", Value: output.TruncateRunes(ctx, req.ShortDescriptionFA, 60)},
		{Key: "Email", Value: req.Email},
		{Key: labelArtifact, Value: shortenDisplayPath(ctx, req.ArtifactPath)},
		{Key: "Icon", Value: shortenDisplayPath(ctx, req.IconPath)},
		{Key: "Screenshots", Value: fmt.Sprintf("%d file(s)", len(req.ScreenshotPaths))},
		{Key: "Issue App Token", Value: fmt.Sprintf("%v", req.IssueAPIAgent)},
	}
	if req.NameEN != "" {
		details = append(details, output.KeyValue{Key: "Name (EN)", Value: req.NameEN})
	}
	_, err := rt.Printer.ConfirmDangerousAction(ctx, output.ConfirmRequest{
		Title:       "Create New App via Nasher Publisher API",
		TargetApp:   req.PackageName,
		Warning:     "Creates a brand-new app under your CafeBazaar developer account and submits its first version for review.",
		Details:     details,
		PromptLabel: fmt.Sprintf("Submit new app %s to CafeBazaar?", req.PackageName),
	})
	return err
}

// SelectCategoryID prompts the developer to pick one of CafeBazaar's official app or game categories.
func SelectCategoryID(ctx context.Context, printer *output.Printer) (int, error) {
	cats := publisher.Categories(ctx)
	opts := make([]output.SelectOption, 0, len(cats)+1)
	for _, c := range cats {
		opts = append(opts, output.SelectOption{
			Label: c.FormatLabel(ctx),
			Value: strconv.Itoa(c.ID),
		})
	}
	opts = append(opts, output.SelectOption{
		Label: "Enter custom category ID...",
		Value: customCategoryOption,
	})

	chosen, err := printer.Select(
		ctx,
		"Select CafeBazaar Category",
		"Choose your app or game category (Apps listed first, followed by Games).",
		opts,
	)
	if err != nil {
		return 0, err
	}
	if chosen == customCategoryOption {
		rawCat, inErr := promptValidatedInput(ctx, printer,
			"Custom Category ID (category_id)",
			"Enter a positive numeric CafeBazaar category ID.",
			"e.g. 11",
			"",
			func(val string) (string, error) {
				id, convErr := strconv.Atoi(strings.TrimSpace(val))
				if convErr != nil || id <= 0 {
					return "", publisher.ErrInvalidCategoryID
				}
				return strconv.Itoa(id), nil
			},
		)
		if inErr != nil {
			return 0, inErr
		}
		id, convErr := strconv.Atoi(rawCat)
		if convErr != nil {
			return 0, publisher.ErrInvalidCategoryID
		}
		return id, nil
	}

	id, err := strconv.Atoi(strings.TrimSpace(chosen))
	if err != nil || id <= 0 {
		return 0, publisher.ErrInvalidCategoryID
	}
	return id, nil
}

func completeCategoryIDs(ctx context.Context) func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
	cats := publisher.Categories(ctx)
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		_ = cmd
		_ = args
		prefix := strings.ToLower(strings.TrimSpace(toComplete))
		out := make([]cobra.Completion, 0, len(cats))
		for _, c := range cats {
			idStr := strconv.Itoa(c.ID)
			if prefix != "" && !strings.HasPrefix(idStr, prefix) && !strings.Contains(strings.ToLower(c.NameEN), prefix) {
				continue
			}
			out = append(out, cobra.CompletionWithDesc(idStr, fmt.Sprintf("%s - %s (%s)", c.NameEN, c.NameFA, c.Group)))
		}
		return out, cobra.ShellCompDirectiveNoFileComp
	}
}

func promptValidatedInput(
	ctx context.Context,
	printer *output.Printer,
	title string,
	description string,
	placeholder string,
	defaultValue string,
	validate func(string) (string, error),
) (string, error) {
	for range maxPromptRetries {
		raw, err := printer.Input(ctx, title, description, placeholder, defaultValue, false)
		if err != nil {
			return "", err
		}
		validated, valErr := validate(raw)
		if valErr == nil {
			return validated, nil
		}
		if warnErr := printer.PrintWarning(ctx, valErr.Error()); warnErr != nil {
			return "", warnErr
		}
	}
	return "", fmt.Errorf("too many invalid inputs for %q", title)
}

func promptRequiredMultiline(ctx context.Context, printer *output.Printer, title, description string) (string, error) {
	for range maxPromptRetries {
		val, err := printer.MultilineInput(ctx, title, description, "")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val), nil
		}
		if warnErr := printer.PrintWarning(ctx, "This field is required and cannot be empty."); warnErr != nil {
			return "", warnErr
		}
	}
	return "", fmt.Errorf("missing required value for %q", title)
}

func promptScreenshots(ctx context.Context, printer *output.Printer) ([]string, error) {
	var collected []string
	for len(collected) < publisher.MaxScreenshotCount {
		title := fmt.Sprintf("Screenshot Path(s) or Folder (%d/%d added, max %d)", len(collected), publisher.MinScreenshotCount, publisher.MaxScreenshotCount)
		desc := "Enter a screenshot image path, comma-separated paths, or a folder containing PNG/JPEG/WebP screenshots."
		if len(collected) > 0 {
			title = fmt.Sprintf("Add Another Screenshot (%d/%d, leave blank to finish)", len(collected), publisher.MaxScreenshotCount)
			desc = "Enter another image path/folder, or press Enter with an empty value to continue."
		}

		raw, err := printer.Input(ctx, title, desc, "assets/screenshots/screen1.png", "", false)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(raw) == "" {
			if len(collected) >= publisher.MinScreenshotCount {
				break
			}
			if warnErr := printer.PrintWarning(ctx, "At least 1 screenshot image is required."); warnErr != nil {
				return nil, warnErr
			}
			continue
		}

		expanded, expErr := expandScreenshotInputs(ctx, []string{raw})
		if expErr != nil {
			if warnErr := printer.PrintWarning(ctx, expErr.Error()); warnErr != nil {
				return nil, warnErr
			}
			continue
		}
		if len(collected)+len(expanded) > publisher.MaxScreenshotCount {
			if warnErr := printer.PrintWarning(ctx, fmt.Sprintf("Maximum %d screenshots allowed (you currently have %d and tried to add %d).", publisher.MaxScreenshotCount, len(collected), len(expanded))); warnErr != nil {
				return nil, warnErr
			}
			continue
		}
		collected = append(collected, expanded...)
	}
	return collected, nil
}

func expandScreenshotInputs(ctx context.Context, rawInputs []string) ([]string, error) {
	var paths []string
	seen := map[string]struct{}{}

	appendUnique := func(p string) {
		clean := filepath.Clean(p)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		paths = append(paths, clean)
	}

	for _, raw := range rawInputs {
		for token := range strings.SplitSeq(raw, ",") {
			trimmed := strings.TrimSpace(token)
			if trimmed == "" {
				continue
			}
			expanded := expandUserPath(ctx, trimmed)
			info, err := os.Stat(expanded)
			if err != nil {
				return nil, fmt.Errorf("screenshot path %q: %w", expanded, err)
			}
			if !info.IsDir() {
				appendUnique(expanded)
				continue
			}

			entries, err := os.ReadDir(expanded)
			if err != nil {
				return nil, fmt.Errorf("read screenshot directory %q: %w", expanded, err)
			}
			var dirImages []string
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				if isSupportedImageExt(ctx, entry.Name()) {
					dirImages = append(dirImages, filepath.Join(expanded, entry.Name()))
				}
			}
			sort.Strings(dirImages)
			if len(dirImages) == 0 {
				return nil, fmt.Errorf("no PNG/JPEG/WebP images found in directory %q", expanded)
			}
			for _, imgPath := range dirImages {
				appendUnique(imgPath)
			}
		}
	}

	if len(paths) > publisher.MaxScreenshotCount {
		return nil, fmt.Errorf("%w (got %d)", publisher.ErrInvalidScreenshots, len(paths))
	}
	return paths, nil
}

func isSupportedImageExt(ctx context.Context, filename string) bool {
	_ = ctx
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".png", ".jpg", ".jpeg", ".webp":
		return true
	default:
		return false
	}
}

func expandUserPath(ctx context.Context, p string) string {
	_ = ctx
	trimmed := strings.TrimSpace(p)
	if trimmed == "" {
		return ""
	}
	// Strip surrounding quotes if the user pasted a quoted terminal path.
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	if trimmed == "~" || strings.HasPrefix(trimmed, "~/") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			if trimmed == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(trimmed, "~/"))
		}
	}
	return trimmed
}
