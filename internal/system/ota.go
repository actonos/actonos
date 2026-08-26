package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	ErrChecksumMismatch = errors.New("ota update failed: sha256 checksum mismatch")
	ErrOTAJobActive     = errors.New("an OTA job is already active")
	ErrOTANoPrevious    = errors.New("no previous build available for rollback")
	ErrApplyUnsupported = errors.New("ota apply is not supported on this runtime")
)

const (
	JobQueued      = "queued"
	JobDownloading = "downloading"
	JobVerifying   = "verifying"
	JobSwapping    = "swapping"
	JobRestarting  = "restarting"
	JobSucceeded   = "succeeded"
	JobFailed      = "failed"
	JobInterrupted = "interrupted"

	ErrCodeRateLimit        = "GITHUB_RATE_LIMIT"
	ErrCodeCheckFailed      = "OTA_CHECK_FAILED"
	ErrCodeAssetMissing     = "ASSET_MISSING"
	ErrCodeInvalidVersion   = "INVALID_VERSION"
	ErrCodeJobActive        = "OTA_JOB_ACTIVE"
	ErrCodeNoPrevious       = "OTA_NO_PREVIOUS"
	ErrCodeApplyUnsupported = "OTA_APPLY_UNSUPPORTED"
)

// UpdateRelease is the legacy single-binary payload kept for tests of persist/rollback.
type UpdateRelease struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	ChecksumSHA string `json:"checksum_sha256"`
	ReleaseDate string `json:"release_date"`
}

// CheckResult is the JSON body of POST /api/system/ota/check and GET /ota/status.
type CheckResult struct {
	CurrentVersion         string     `json:"current_version"`
	LatestVersion          string     `json:"latest_version"`
	UpdateAvailable        bool       `json:"update_available"`
	ApplySupported         bool       `json:"apply_supported"`
	ApplyUnsupportedReason string     `json:"apply_unsupported_reason,omitempty"`
	CanInstall             bool       `json:"can_install"`
	ChecksumMissing        bool       `json:"checksum_missing"`
	AllowUnsigned          bool       `json:"allow_unsigned"`
	EmbeddingdRequired     bool       `json:"embeddingd_required"`
	ErrorCode              string     `json:"error_code,omitempty"`
	ErrorMessage           string     `json:"error_message,omitempty"`
	RetryAfter             int        `json:"retry_after,omitempty"`
	LastChecked            string     `json:"last_checked"`
	GitCommit              string     `json:"git_commit,omitempty"`
	BuildTime              string     `json:"build_time,omitempty"`
	ActiveBinary           string     `json:"active_binary,omitempty"`
	PreviousBinary         string     `json:"previous_binary,omitempty"`
	Assets                 []OTAAsset `json:"assets,omitempty"`
	Job                    *OTAJob    `json:"job,omitempty"`
}

// OTAAsset describes one host-arch binary from the GitHub release.
type OTAAsset struct {
	Name            string `json:"name"`
	Role            string `json:"role"`
	Required        bool   `json:"required"`
	Present         bool   `json:"present"`
	DownloadURL     string `json:"download_url,omitempty"`
	Checksum        string `json:"checksum,omitempty"`
	ChecksumMissing bool   `json:"checksum_missing"`
}

// OTAJob is the persisted apply/rollback progress record.
type OTAJob struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Version   string    `json:"version,omitempty"`
	Error     string    `json:"error,omitempty"`
	Progress  int       `json:"progress,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OTARestarter restarts helpers after a successful swap. Tests stub this.
type OTARestarter interface {
	RestartEmbeddingd(ctx context.Context) error
	RestartDaemon(ctx context.Context) error
}

// OTAEngine handles GitHub-backed check, two-binary apply, and rollback.
type OTAEngine struct {
	mu           sync.Mutex
	applyMu      sync.Mutex
	dataDir      string
	releasesDir  string
	binDir       string
	binSymlink   string
	version      string
	gitCommit    string
	buildTime    string
	apiURL       string
	downloadBase string
	token        string
	goos         string
	goarch       string

	checkClient    *http.Client
	downloadClient *http.Client
	allowLoopback  bool
	skipRestart    bool
	restarter      OTARestarter

	cache   *CheckResult
	cacheAt time.Time

	job                 *OTAJob
	activeBuild         string
	previousBuild       string
	embeddingActive     string
	embeddingPrevious   string
	lastNotifiedVersion string
	lastChecked         time.Time
}

// NewOTAEngine creates a new OTAEngine rooted at dataDir/releases.
func NewOTAEngine(dataDir string) *OTAEngine {
	releasesDir := filepath.Join(dataDir, "releases")
	binDir := filepath.Join(dataDir, "bin")
	_ = os.MkdirAll(releasesDir, 0755)
	_ = os.MkdirAll(binDir, 0755)

	apiURL := os.Getenv("ACTONOS_OTA_RELEASES_API")
	if apiURL == "" {
		apiURL = DefaultOTAReleasesAPI
	}
	dlBase := os.Getenv("ACTONOS_OTA_DOWNLOAD_BASE")
	if dlBase == "" {
		dlBase = DefaultOTADownloadBase
	}

	eng := &OTAEngine{
		dataDir:      dataDir,
		releasesDir:  releasesDir,
		binDir:       binDir,
		binSymlink:   filepath.Join(binDir, "actond"),
		apiURL:       apiURL,
		downloadBase: dlBase,
		token:        os.Getenv("ACTONOS_OTA_GITHUB_TOKEN"),
		goos:         runtime.GOOS,
		goarch:       runtime.GOARCH,
		checkClient:  &http.Client{Timeout: otaCheckTimeout},
		downloadClient: &http.Client{
			Timeout: otaDownloadTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 8 {
					return errors.New("too many redirects")
				}
				host := req.URL.Hostname()
				if otaHostAllowed(host) {
					return nil
				}
				return fmt.Errorf("redirect to unallowed host %s", host)
			},
		},
	}
	_ = eng.LoadState()
	return eng
}

// SetVersionMeta stamps check responses with the running daemon identity.
func (o *OTAEngine) SetVersionMeta(version, gitCommit, buildTime string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.version = version
	o.gitCommit = gitCommit
	o.buildTime = buildTime
}

// SetAPIURL overrides the GitHub REST endpoint (tests inject httptest.Server).
func (o *OTAEngine) SetAPIURL(raw string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.apiURL = raw
}

// SetAllowLoopback permits httptest (127.0.0.1) for unit tests.
func (o *OTAEngine) SetAllowLoopback(v bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.allowLoopback = v
	if v {
		o.downloadClient.CheckRedirect = nil
	}
}

// SetPlatform overrides GOOS/GOARCH for asset matching in tests.
func (o *OTAEngine) SetPlatform(goos, goarch string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.goos, o.goarch = goos, goarch
}

// SetSkipRestart disables HAL restart after apply (unit tests).
func (o *OTAEngine) SetSkipRestart(v bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.skipRestart = v
}

// SetRestarter installs the post-swap restart implementation.
func (o *OTAEngine) SetRestarter(r OTARestarter) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.restarter = r
}

// Check queries GitHub REST /releases/latest. HTML /releases is never fetched.
func (o *OTAEngine) Check(ctx context.Context, currentVersion string, force, embeddingdRequired bool) *CheckResult {
	o.mu.Lock()
	if !force && o.cache != nil && time.Since(o.cacheAt) < otaCheckCacheTTL {
		cached := *o.cache
		cached.Job = o.cloneJobLocked()
		cached.CanInstall = canInstall(&cached)
		o.mu.Unlock()
		return &cached
	}
	apiURL := o.apiURL
	goos, goarch := o.goos, o.goarch
	allowUnsigned := AllowUnsignedApply()
	o.mu.Unlock()

	result := o.baseCheckResult(currentVersion, embeddingdRequired, allowUnsigned)

	rel, status, header, err := o.fetchLatestRelease(ctx)
	if status == http.StatusForbidden || status == http.StatusTooManyRequests {
		result.ErrorCode = ErrCodeRateLimit
		result.ErrorMessage = "GitHub rate limit exceeded"
		result.RetryAfter = retryAfterSeconds(header)
		o.storeCache(result)
		return result
	}
	if err != nil {
		result.ErrorCode = ErrCodeCheckFailed
		result.ErrorMessage = err.Error()
		o.storeCache(result)
		return result
	}
	if rel.TagName == "" {
		result.ErrorCode = ErrCodeInvalidVersion
		result.ErrorMessage = "release tag is empty"
		o.storeCache(result)
		return result
	}

	canonLatest := CanonicalVersion(rel.TagName)
	canonCurrent := CanonicalVersion(currentVersion)
	if canonLatest == "" {
		result.ErrorCode = ErrCodeInvalidVersion
		result.ErrorMessage = "invalid latest version " + rel.TagName
		o.storeCache(result)
		return result
	}
	result.LatestVersion = strings.TrimPrefix(canonLatest, "v")
	if canonCurrent == "" && currentVersion != "" && currentVersion != "0.0.0-dev" {
		result.ErrorCode = ErrCodeInvalidVersion
		result.ErrorMessage = "invalid current version " + currentVersion
		o.storeCache(result)
		return result
	}

	sums := o.assetChecksums(ctx, rel)
	actondName := ReleaseAssetName("actond", result.LatestVersion, goos, goarch)
	embName := ReleaseAssetName("embeddingd", result.LatestVersion, goos, goarch)
	actond := findAsset(rel, actondName, o.downloadBase)
	emb := findAsset(rel, embName, o.downloadBase)
	if hex, ok := sums[actondName]; ok {
		actond.Checksum = hex
		actond.ChecksumMissing = hex == ""
	}
	if hex, ok := sums[embName]; ok {
		emb.Checksum = hex
		emb.ChecksumMissing = hex == ""
	}
	actond.ChecksumMissing = actond.Present && actond.Checksum == ""
	emb.ChecksumMissing = emb.Present && emb.Checksum == ""
	actond.Required = true
	emb.Required = embeddingdRequired
	result.Assets = []OTAAsset{actond, emb}

	newer := VersionNewer(rel.TagName, currentVersion) || (canonCurrent == "" && currentVersion == "0.0.0-dev" && canonLatest != "")
	if !actond.Present || (embeddingdRequired && !emb.Present) {
		if newer {
			result.ErrorCode = ErrCodeAssetMissing
			result.ErrorMessage = "host-arch release asset missing"
		}
		result.UpdateAvailable = false
	} else {
		result.UpdateAvailable = newer
	}
	result.ChecksumMissing = (actond.Present && actond.ChecksumMissing) ||
		(embeddingdRequired && emb.Present && emb.ChecksumMissing)
	result.CanInstall = canInstall(result)
	_ = apiURL
	o.storeCache(result)
	return result
}

func (o *OTAEngine) baseCheckResult(currentVersion string, embeddingdRequired, allowUnsigned bool) *CheckResult {
	supported, reason := OTAApplySupport()
	o.mu.Lock()
	defer o.mu.Unlock()
	now := time.Now().UTC()
	o.lastChecked = now
	_ = o.persistStateLocked()
	cur := strings.TrimPrefix(CanonicalVersion(currentVersion), "v")
	if cur == "" {
		cur = currentVersion
	}
	return &CheckResult{
		CurrentVersion:         cur,
		LatestVersion:          cur,
		ApplySupported:         supported,
		ApplyUnsupportedReason: reason,
		AllowUnsigned:          allowUnsigned,
		EmbeddingdRequired:     embeddingdRequired,
		LastChecked:            now.Format(time.RFC3339),
		GitCommit:              o.gitCommit,
		BuildTime:              o.buildTime,
		ActiveBinary:           o.activeBuild,
		PreviousBinary:         o.previousBuild,
		Job:                    o.cloneJobLocked(),
	}
}

func (o *OTAEngine) storeCache(result *CheckResult) {
	o.mu.Lock()
	defer o.mu.Unlock()
	result.Job = o.cloneJobLocked()
	result.CanInstall = canInstall(result)
	cp := *result
	o.cache = &cp
	o.cacheAt = time.Now()
}

func canInstall(r *CheckResult) bool {
	if r == nil || r.ErrorCode != "" || !r.UpdateAvailable || !r.ApplySupported {
		return false
	}
	if r.ChecksumMissing && !r.AllowUnsigned {
		return false
	}
	if r.Job != nil && JobIsActive(r.Job.Status) {
		return false
	}
	return true
}

func findAsset(rel *githubRelease, name, downloadBase string) OTAAsset {
	a := OTAAsset{Name: name, Role: strings.Split(name, "_")[0]}
	for _, asset := range rel.Assets {
		if asset.Name != name {
			continue
		}
		a.Present = true
		a.DownloadURL = asset.BrowserDownloadURL
		a.Checksum = NormalizeSHA256(asset.Digest)
		break
	}
	if !a.Present && downloadBase != "" {
		// Public-repo last-resort name; still require the asset to have been
		// listed in the JSON so we never scrape HTML /releases.
		return a
	}
	return a
}

// Status returns the last check (if any) plus the live job.
func (o *OTAEngine) Status() *CheckResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cache != nil {
		cp := *o.cache
		cp.Job = o.cloneJobLocked()
		cp.CanInstall = canInstall(&cp)
		return &cp
	}
	supported, reason := OTAApplySupport()
	return &CheckResult{
		CurrentVersion:         strings.TrimPrefix(CanonicalVersion(o.version), "v"),
		LatestVersion:          strings.TrimPrefix(CanonicalVersion(o.version), "v"),
		ApplySupported:         supported,
		ApplyUnsupportedReason: reason,
		LastChecked:            o.lastChecked.Format(time.RFC3339),
		GitCommit:              o.gitCommit,
		BuildTime:              o.buildTime,
		ActiveBinary:           o.activeBuild,
		PreviousBinary:         o.previousBuild,
		Job:                    o.cloneJobLocked(),
		AllowUnsigned:          AllowUnsignedApply(),
	}
}

// EnqueueApply starts download/activate in a goroutine and returns immediately.
func (o *OTAEngine) EnqueueApply(ctx context.Context, currentVersion string, embeddingdRequired bool) error {
	if supported, _ := OTAApplySupport(); !supported {
		return ErrApplyUnsupported
	}
	o.mu.Lock()
	if o.job != nil && JobIsActive(o.job.Status) {
		o.mu.Unlock()
		return ErrOTAJobActive
	}
	check := o.cache
	o.mu.Unlock()
	if check == nil || !check.UpdateAvailable {
		check = o.Check(ctx, currentVersion, true, embeddingdRequired)
	}
	if !check.UpdateAvailable {
		return fmt.Errorf("no update available")
	}
	if check.ChecksumMissing && !AllowUnsignedApply() {
		return fmt.Errorf("checksum missing")
	}
	o.mu.Lock()
	if o.job != nil && JobIsActive(o.job.Status) {
		o.mu.Unlock()
		return ErrOTAJobActive
	}
	o.setJobLocked(&OTAJob{
		ID:        fmt.Sprintf("ota_%d", time.Now().UnixNano()),
		Action:    "apply",
		Status:    JobQueued,
		Version:   check.LatestVersion,
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	assets := append([]OTAAsset(nil), check.Assets...)
	version := check.LatestVersion
	o.mu.Unlock()

	go func() {
		_ = o.runApply(context.Background(), version, assets, embeddingdRequired)
	}()
	return nil
}

func (o *OTAEngine) runApply(ctx context.Context, version string, assets []OTAAsset, embeddingdRequired bool) error {
	o.applyMu.Lock()
	defer o.applyMu.Unlock()
	if WritesFrozen(o.dataDir) {
		o.failJob("ota download frozen: disk exhausted")
		return errors.New("ota download frozen: disk exhausted")
	}
	o.setJobStatus(JobDownloading, 10, "")

	targetDir := filepath.Join(o.releasesDir, version)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		o.failJob(err.Error())
		return err
	}

	var actondSrc, embSrc string
	for _, a := range assets {
		if !a.Required && a.Role != "actond" {
			continue
		}
		if a.Role == "embeddingd" && !embeddingdRequired {
			continue
		}
		if a.DownloadURL == "" {
			o.failJob("missing download url for " + a.Name)
			return fmt.Errorf("missing download url for %s", a.Name)
		}
		destName := HostBinaryFileName(a.Role, o.goos)
		dest := filepath.Join(targetDir, destName)
		o.setJobStatus(JobDownloading, 30, "")
		if err := o.downloadFile(ctx, a.DownloadURL, dest); err != nil {
			o.failJob(err.Error())
			return err
		}
		o.setJobStatus(JobVerifying, 60, "")
		if a.Checksum != "" {
			if err := verifyFileSHA256(dest, a.Checksum); err != nil {
				_ = os.Remove(dest)
				o.failJob(err.Error())
				return err
			}
		} else if !AllowUnsignedApply() {
			_ = os.Remove(dest)
			o.failJob("checksum missing for " + a.Name)
			return fmt.Errorf("%w: missing checksum", ErrChecksumMismatch)
		}
		if a.Role == "actond" {
			actondSrc = dest
		} else if a.Role == "embeddingd" {
			embSrc = dest
		}
	}
	if actondSrc == "" {
		o.failJob("actond asset was not downloaded")
		return errors.New("actond asset was not downloaded")
	}

	o.setJobStatus(JobSwapping, 80, "")
	o.mu.Lock()
	prevActond := o.activeBuild
	prevEmb := o.embeddingActive
	o.mu.Unlock()

	if err := o.activateActond(actondSrc); err != nil {
		o.failJob(err.Error())
		return err
	}
	if embSrc != "" {
		if err := o.activateEmbeddingd(embSrc); err != nil {
			o.failJob(err.Error())
			return err
		}
	}

	o.mu.Lock()
	if prevActond != "" && prevActond != actondSrc {
		o.previousBuild = prevActond
	}
	o.activeBuild = actondSrc
	if embSrc != "" {
		if prevEmb != "" && prevEmb != embSrc {
			o.embeddingPrevious = prevEmb
		}
		o.embeddingActive = embSrc
	}
	_ = o.persistStateLocked()
	o.mu.Unlock()

	o.setJobStatus(JobRestarting, 90, "")
	o.deleteJobFile()
	if !o.skipRestart && o.restarter != nil {
		if embSrc != "" {
			_ = o.restarter.RestartEmbeddingd(ctx)
			time.Sleep(time.Second)
		}
		_ = o.restarter.RestartDaemon(ctx)
	}
	o.mu.Lock()
	o.job = nil
	o.mu.Unlock()
	slog.Info("ota apply completed", "version", version, "path", actondSrc)
	return nil
}

func (o *OTAEngine) downloadFile(ctx context.Context, rawURL, dest string) error {
	resp, err := o.doGitHub(ctx, o.downloadClient, http.MethodGet, rawURL)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: HTTP %d", rawURL, resp.StatusCode)
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, io.LimitReader(resp.Body, 512<<20)); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func verifyFileSHA256(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	sum := hex.EncodeToString(digest[:])
	if sum != NormalizeSHA256(expected) {
		return fmt.Errorf("%w (expected %s, got %s)", ErrChecksumMismatch, NormalizeSHA256(expected), sum)
	}
	return nil
}

// EnqueueRollback restores previous binaries and restarts.
func (o *OTAEngine) EnqueueRollback() error {
	o.mu.Lock()
	if o.job != nil && JobIsActive(o.job.Status) {
		o.mu.Unlock()
		return ErrOTAJobActive
	}
	if o.previousBuild == "" {
		o.mu.Unlock()
		return ErrOTANoPrevious
	}
	o.setJobLocked(&OTAJob{
		ID:        fmt.Sprintf("ota_rb_%d", time.Now().UnixNano()),
		Action:    "rollback",
		Status:    JobQueued,
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	o.mu.Unlock()
	go func() {
		_ = o.runRollback(context.Background())
	}()
	return nil
}

func (o *OTAEngine) runRollback(ctx context.Context) error {
	o.applyMu.Lock()
	defer o.applyMu.Unlock()
	if err := o.Rollback(); err != nil {
		o.failJob(err.Error())
		return err
	}
	o.setJobStatus(JobRestarting, 90, "")
	o.deleteJobFile()
	if !o.skipRestart && o.restarter != nil {
		_ = o.restarter.RestartEmbeddingd(ctx)
		time.Sleep(time.Second)
		_ = o.restarter.RestartDaemon(ctx)
	}
	o.mu.Lock()
	o.job = nil
	o.mu.Unlock()
	return nil
}

type otaState struct {
	Active              string `json:"active"`
	Previous            string `json:"previous"`
	EmbeddingActive     string `json:"embedding_active,omitempty"`
	EmbeddingPrevious   string `json:"embedding_previous,omitempty"`
	LastChecked         string `json:"last_checked,omitempty"`
	LastNotifiedVersion string `json:"last_notified_version,omitempty"`
}

func (o *OTAEngine) statePath() string {
	return filepath.Join(o.releasesDir, "state.json")
}

func (o *OTAEngine) jobPath() string {
	return filepath.Join(o.releasesDir, "job.json")
}

func (o *OTAEngine) persistState() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.persistStateLocked()
}

func (o *OTAEngine) persistStateLocked() error {
	state := otaState{
		Active:              o.activeBuild,
		Previous:            o.previousBuild,
		EmbeddingActive:     o.embeddingActive,
		EmbeddingPrevious:   o.embeddingPrevious,
		LastNotifiedVersion: o.lastNotifiedVersion,
	}
	if !o.lastChecked.IsZero() {
		state.LastChecked = o.lastChecked.UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(o.statePath(), data, 0644)
}

// LoadState restores previous/active paths so rollback survives process restart.
func (o *OTAEngine) LoadState() error {
	data, err := os.ReadFile(o.statePath())
	if err == nil {
		var state otaState
		if err := json.Unmarshal(data, &state); err != nil {
			return err
		}
		o.mu.Lock()
		o.activeBuild = state.Active
		o.previousBuild = state.Previous
		o.embeddingActive = state.EmbeddingActive
		o.embeddingPrevious = state.EmbeddingPrevious
		o.lastNotifiedVersion = state.LastNotifiedVersion
		if state.LastChecked != "" {
			if ts, err := time.Parse(time.RFC3339, state.LastChecked); err == nil {
				o.lastChecked = ts
			}
		}
		o.mu.Unlock()
	}
	o.reclaimJob()
	return nil
}

func (o *OTAEngine) reclaimJob() {
	data, err := os.ReadFile(o.jobPath())
	if err != nil {
		return
	}
	var job OTAJob
	if err := json.Unmarshal(data, &job); err != nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if JobIsActive(job.Status) {
		job.Status = JobInterrupted
		job.Error = "process restarted during OTA job"
		job.UpdatedAt = time.Now().UTC()
		o.job = &job
		_ = o.writeJobLocked()
		return
	}
	o.job = &job
}

func (o *OTAEngine) setJobLocked(job *OTAJob) {
	o.job = job
	_ = o.writeJobLocked()
}

func (o *OTAEngine) writeJobLocked() error {
	if o.job == nil {
		return nil
	}
	data, err := json.Marshal(o.job)
	if err != nil {
		return err
	}
	return os.WriteFile(o.jobPath(), data, 0644)
}

func (o *OTAEngine) setJobStatus(status string, progress int, errMsg string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.job == nil {
		return
	}
	o.job.Status = status
	o.job.Progress = progress
	if errMsg != "" {
		o.job.Error = errMsg
	}
	o.job.UpdatedAt = time.Now().UTC()
	_ = o.writeJobLocked()
}

func (o *OTAEngine) failJob(msg string) {
	o.setJobStatus(JobFailed, 0, msg)
}

func (o *OTAEngine) deleteJobFile() {
	_ = os.Remove(o.jobPath())
}

func (o *OTAEngine) cloneJobLocked() *OTAJob {
	if o.job == nil {
		return nil
	}
	cp := *o.job
	return &cp
}

// Job returns a copy of the current job.
func (o *OTAEngine) Job() *OTAJob {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.cloneJobLocked()
}

// State returns persisted OTA pointers.
func (o *OTAEngine) State() (active, previous string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.activeBuild, o.previousBuild
}

// EmbeddingState returns persisted embeddingd binary pointers.
func (o *OTAEngine) EmbeddingState() (active, previous string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.embeddingActive, o.embeddingPrevious
}

// LastNotifiedVersion is the last GitHub tag we already notified about.
func (o *OTAEngine) LastNotifiedVersion() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastNotifiedVersion
}

// SetLastNotifiedVersion persists the dedupe key for 24h notifications.
func (o *OTAEngine) SetLastNotifiedVersion(v string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lastNotifiedVersion = v
	_ = o.persistStateLocked()
}

// Rollback restores the previous build pointers and activates those binaries.
func (o *OTAEngine) Rollback() error {
	o.mu.Lock()
	if o.previousBuild == "" {
		o.mu.Unlock()
		return ErrOTANoPrevious
	}
	prev := o.previousBuild
	prevEmb := o.embeddingPrevious
	cur := o.activeBuild
	curEmb := o.embeddingActive
	o.activeBuild = prev
	o.previousBuild = cur
	if prevEmb != "" {
		o.embeddingActive = prevEmb
		o.embeddingPrevious = curEmb
	}
	_ = o.persistStateLocked()
	o.mu.Unlock()

	if err := o.activateActond(prev); err != nil {
		slog.Warn("ota rollback activate actond", "error", err)
	}
	if prevEmb != "" {
		if err := o.activateEmbeddingd(prevEmb); err != nil {
			slog.Warn("ota rollback activate embeddingd", "error", err)
		}
	}
	slog.Warn("ota rollback performed", "restored_build", prev)
	return nil
}

// ApplyUpdate is the legacy single-binary download used by older tests; prefer EnqueueApply.
func (o *OTAEngine) ApplyUpdate(ctx context.Context, release UpdateRelease) error {
	if WritesFrozen(o.dataDir) {
		return errors.New("ota download frozen: disk exhausted")
	}
	targetDir := filepath.Join(o.releasesDir, release.Version)
	_ = os.MkdirAll(targetDir, 0755)
	targetBinary := filepath.Join(targetDir, HostBinaryFileName("actond", o.goos))
	if err := o.downloadFile(ctx, release.DownloadURL, targetBinary); err != nil {
		return err
	}
	if release.ChecksumSHA != "" {
		if err := verifyFileSHA256(targetBinary, release.ChecksumSHA); err != nil {
			_ = os.Remove(targetBinary)
			return err
		}
	}
	o.mu.Lock()
	o.previousBuild = o.activeBuild
	o.activeBuild = targetBinary
	_ = o.persistStateLocked()
	o.mu.Unlock()
	return o.activateActond(targetBinary)
}

func copyReplace(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	_ = os.MkdirAll(filepath.Dir(dest), 0755)
	tmp := dest + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Remove(dest)
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	return os.Chmod(dest, 0755)
}
