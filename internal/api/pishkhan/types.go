package pishkhan

import (
	"context"
	"errors"
	"fmt"
)

const (
	// DefaultBaseURL is the production base URL for CafeBazaar's Pishkhan Release API.
	DefaultBaseURL = "https://api.pishkhan.cafebazaar.ir"

	// EndpointReleases creates or deletes an uncommitted release.
	EndpointReleases = "/v1/apps/releases/"
	// EndpointUploadAPK uploads an APK package to the current uncommitted release.
	EndpointUploadAPK = "/v1/apps/releases/upload/"
	// EndpointUploadAAB uploads an Android App Bundle to the current uncommitted release.
	EndpointUploadAAB = "/v1/apps/releases/upload-aab/"
	// EndpointUploadBin uploads the Bundle Signer bin file for an uploaded AAB.
	EndpointUploadBin = "/v1/apps/releases/upload-bin/"
	// EndpointBundleStatus retrieves the processing status of the uploaded AAB bundle.
	EndpointBundleStatus = "/v1/apps/releases/bundle-status/"
	// EndpointCommit commits the uncommitted release and requests CafeBazaar review.
	EndpointCommit = "/v1/apps/releases/commit/"
	// EndpointLastCommitted retrieves the most recent committed release.
	EndpointLastCommitted = "/v1/apps/releases/last-committed/"
	// EndpointLastUncommitted retrieves the current uncommitted draft release.
	EndpointLastUncommitted = "/v1/apps/releases/last-uncommitted/"
	// EndpointLastPublished retrieves the currently published release.
	EndpointLastPublished = "/v1/apps/releases/last-published/"

	// ResponseTypeSuccess indicates a successful Pishkhan API operation.
	ResponseTypeSuccess = "success"
	// ResponseTypeNotExists indicates that no matching release exists yet.
	ResponseTypeNotExists = "not-exists"

	// FormFieldArchitecture is the multipart form field specifying APK CPU architecture.
	FormFieldArchitecture = "architecture"
	// FormFieldAPK is the multipart form field carrying the APK binary.
	FormFieldAPK = "apk"
	// FormFieldAAB is the multipart form field carrying the AAB binary.
	FormFieldAAB = "aab"
	// FormFieldBinFile is the multipart form field carrying the Bundle Signer bin binary.
	FormFieldBinFile = "file"

	// MinRolloutPercentage is the minimum valid staged rollout percentage.
	MinRolloutPercentage = 1
	// MaxRolloutPercentage is the maximum valid staged rollout percentage.
	MaxRolloutPercentage = 100
)

// Architecture represents a supported CPU architecture value for APK uploads.
type Architecture string

const (
	// ArchitectureAll targets all CPU architectures with a universal APK.
	ArchitectureAll Architecture = "all"
	// ArchitectureARMv7 targets 32-bit ARM devices.
	ArchitectureARMv7 Architecture = "armeabi-v7a"
	// ArchitectureARM64 targets 64-bit ARM devices.
	ArchitectureARM64 Architecture = "arm64-v8a"
)

// ValidateArchitecture checks whether arch is supported by the Pishkhan API.
func ValidateArchitecture(ctx context.Context, arch Architecture) error {
	_ = ctx
	switch arch {
	case ArchitectureAll, ArchitectureARMv7, ArchitectureARM64:
		return nil
	default:
		return fmt.Errorf("invalid architecture %q: must be one of %s, %s, %s", arch, ArchitectureAll, ArchitectureARMv7, ArchitectureARM64)
	}
}

// BundleState represents the server-side processing state of an uploaded AAB bundle.
type BundleState string

const (
	// BundleStateUploaded means the AAB was uploaded and is waiting for the bin file.
	BundleStateUploaded BundleState = "U"
	// BundleStateSigned means the bin file was uploaded and is waiting to be signed.
	BundleStateSigned BundleState = "S"
	// BundleStateProcessing means CafeBazaar is actively processing and signing the bundle.
	BundleStateProcessing BundleState = "P"
	// BundleStateDone means bundle processing completed successfully.
	BundleStateDone BundleState = "D"
	// BundleStateError means bundle processing failed with errors.
	BundleStateError BundleState = "E"
)

// DescribeBundleState returns a human-readable explanation of a BundleState code.
func DescribeBundleState(ctx context.Context, state BundleState) string {
	_ = ctx
	switch state {
	case BundleStateUploaded:
		return "AAB uploaded, waiting for Bundle Signer .bin file"
	case BundleStateSigned:
		return "Bundle Signer .bin uploaded, queued for signing"
	case BundleStateProcessing:
		return "Processing and signing bundle"
	case BundleStateDone:
		return "Bundle processing completed"
	case BundleStateError:
		return "Bundle processing failed"
	default:
		return fmt.Sprintf("Unknown bundle state (%s)", string(state))
	}
}

var (
	// ErrBundleFailed indicates that CafeBazaar reported an error state while processing a bundle.
	ErrBundleFailed = errors.New("bundle processing failed on CafeBazaar")
	// ErrInvalidRollout indicates that staged_rollout_percentage is outside 1 to 100.
	ErrInvalidRollout = errors.New("staged rollout percentage must be between 1 and 100")
)

// PackageInfo describes a single APK package attached to a release.
type PackageInfo struct {
	APKSize          int64    `json:"apk_size"`
	SHA1Hash         string   `json:"sha1_hash"`
	VersionCode      int      `json:"version_code"`
	VersionName      string   `json:"version_name"`
	APILevel         int      `json:"api_level"`
	TargetAPILevel   int      `json:"target_api_level"`
	UploadDate       string   `json:"upload_date"`
	CPUArchitectures string   `json:"cpu_architectures"`
	DPI              int      `json:"dpi"`
	Locale           string   `json:"locale"`
	AdditionalFiles  []string `json:"additional_files"`
}

// ReleaseInfo describes a CafeBazaar release object returned by Pishkhan endpoints.
type ReleaseInfo struct {
	ID                      int64         `json:"id,omitempty"`
	ReleaseState            string        `json:"release_state,omitempty"`
	Committed               bool          `json:"committed,omitempty"`
	CreationDate            string        `json:"creation_date"`
	Packages                []PackageInfo `json:"packages"`
	AutoPublish             bool          `json:"auto_publish"`
	StagedRolloutPercentage int           `json:"staged_rollout_percentage"`
	ChangelogFA             *string       `json:"changelog_fa"`
	ChangelogEN             string        `json:"changelog_en"`
}

// ReleaseResponse is returned by release creation, commit, and status endpoints.
type ReleaseResponse struct {
	Type    string       `json:"type"`
	Message string       `json:"message"`
	Release *ReleaseInfo `json:"release,omitempty"`
	Exists  bool         `json:"exists"`
}

// ActionResponse is returned by endpoints such as release deletion.
type ActionResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// UploadAPKRequest defines parameters for uploading an APK file.
type UploadAPKRequest struct {
	FilePath     string
	Architecture Architecture
}

// UploadAPKResponse is returned after adding an APK package to a release.
type UploadAPKResponse struct {
	Type    string      `json:"type"`
	Message string      `json:"message"`
	Package PackageInfo `json:"package"`
}

// BundleInfo describes the current AAB bundle processing status and errors.
type BundleInfo struct {
	State  BundleState `json:"state"`
	Errors []string    `json:"errors"`
}

// UploadAABResponse is returned after uploading an AAB file to a release.
type UploadAABResponse struct {
	Type     string     `json:"type"`
	Message  string     `json:"message"`
	Bundle   BundleInfo `json:"bundle"`
	Warnings []string   `json:"warnings"`
}

// BundleStatusResponse is returned by bin upload and bundle status endpoints.
type BundleStatusResponse struct {
	Type    string     `json:"type"`
	Message string     `json:"message"`
	Bundle  BundleInfo `json:"bundle"`
}

// CommitReleaseRequest defines the JSON body for committing an uncommitted release.
type CommitReleaseRequest struct {
	ChangelogEN             string `json:"changelog_en"`
	ChangelogFA             string `json:"changelog_fa"`
	DeveloperNote           string `json:"developer_note,omitempty"`
	StagedRolloutPercentage int    `json:"staged_rollout_percentage,omitempty"`
	AutoPublish             bool   `json:"auto_publish"`
}
