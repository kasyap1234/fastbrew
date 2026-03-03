package daemon

import (
	"encoding/json"
	"time"

	"fastbrew/internal/brew"
	"fastbrew/internal/services"
)

const (
	APIVersion = 1

	RequestHandshake   = "handshake"
	RequestPing        = "ping"
	RequestStatus      = "status"
	RequestStats       = "stats"
	RequestWarmup      = "warmup"
	RequestShutdown    = "shutdown"
	RequestInvalidate  = "invalidate"
	RequestSearch      = "search"
	RequestList        = "list"
	RequestOutdated    = "outdated"
	RequestInfo        = "info"
	RequestDeps        = "deps"
	RequestLeaves      = "leaves"
	RequestTapInfo     = "tap_info"
	RequestServices    = "services_list"
	RequestJobSubmit   = "job_submit"
	RequestJobStatus   = "job_status"
	RequestJobStream   = "job_stream"
	ResponseCodeBadReq = "bad_request"
	ResponseCodeErr    = "internal_error"
	ResponseCodeVer    = "version_mismatch"
)

const (
	EventInstalledChanged = "installed_changed"
	EventTapChanged       = "tap_changed"
	EventIndexRefreshed   = "index_refreshed"
	EventServiceChanged   = "service_changed"
)

const (
	JobOperationInstall   = "install"
	JobOperationUpgrade   = "upgrade"
	JobOperationUninstall = "uninstall"
	JobOperationReinstall = "reinstall"
)

const (
	JobStatusQueued    = "queued"
	JobStatusRunning   = "running"
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
)

const (
	JobEventKindJob     = "job"
	JobEventKindPackage = "package"
)

const (
	JobEventPhaseResolve   = "resolve"
	JobEventPhaseMetadata  = "metadata"
	JobEventPhaseDownload  = "download"
	JobEventPhaseExtract   = "extract"
	JobEventPhaseLink      = "link"
	JobEventPhaseInstall   = "install"
	JobEventPhaseUninstall = "uninstall"
	JobEventPhaseComplete  = "complete"
)

const (
	JobEventStatusQueued    = "queued"
	JobEventStatusRunning   = "running"
	JobEventStatusProgress  = "progress"
	JobEventStatusSucceeded = "succeeded"
	JobEventStatusFailed    = "failed"
	JobEventStatusSkipped   = "skipped"
)

type Request struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type Response struct {
	OK      bool            `json:"ok"`
	Code    string          `json:"code,omitempty"`
	Error   string          `json:"error,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type HandshakeRequest struct {
	APIVersion    int    `json:"api_version"`
	BinaryVersion string `json:"binary_version"`
}

type HandshakeResponse struct {
	APIVersion    int    `json:"api_version"`
	BinaryVersion string `json:"binary_version"`
}

type SearchRequest struct {
	Query string `json:"query"`
}

type InfoRequest struct {
	Packages []string `json:"packages"`
}

type DepsRequest struct {
	Packages []string `json:"packages"`
}

type TapInfoRequest struct {
	Repo          string `json:"repo"`
	InstalledOnly bool   `json:"installed_only"`
}

type ServicesListRequest struct {
	Scope string `json:"scope"`
}

type InvalidateRequest struct {
	Event string `json:"event"`
}

type StatusResponse struct {
	PID             int       `json:"pid"`
	SocketPath      string    `json:"socket_path"`
	StartedAt       time.Time `json:"started_at"`
	LastActivityAt  time.Time `json:"last_activity_at"`
	IdleTimeoutSecs int       `json:"idle_timeout_secs"`
}

type StatsResponse struct {
	UptimeSeconds int64      `json:"uptime_seconds"`
	RequestsTotal uint64     `json:"requests_total"`
	CacheHits     uint64     `json:"cache_hits"`
	CacheMisses   uint64     `json:"cache_misses"`
	LastWarmupAt  *time.Time `json:"last_warmup_at,omitempty"`
	JobsTotal     int        `json:"jobs_total"`
	JobsRunning   int        `json:"jobs_running"`
	JobsFailed    int        `json:"jobs_failed"`

	InstalledCached bool `json:"installed_cached"`
	OutdatedCached  bool `json:"outdated_cached"`
	LeavesCached    bool `json:"leaves_cached"`

	FormulaMetaEntries int `json:"formula_meta_entries"`
	CaskMetaEntries    int `json:"cask_meta_entries"`
	DepsCacheEntries   int `json:"deps_cache_entries"`
	TapCacheEntries    int `json:"tap_cache_entries"`
	ServicesEntries    int `json:"services_entries"`
	SearchEntries      int `json:"search_entries"`
}

type PackageInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Desc         string   `json:"desc"`
	Homepage     string   `json:"homepage"`
	Dependencies []string `json:"dependencies"`
	KegOnly      bool     `json:"keg_only"`
}

type InfoResponse struct {
	Packages []PackageInfo `json:"packages"`
}

type SearchResponse struct {
	Items []brew.SearchItem `json:"items"`
}

type ListResponse struct {
	Items []brew.PackageInfo `json:"items"`
}

type OutdatedResponse struct {
	Items []brew.OutdatedPackage `json:"items"`
}

type DepsResponse struct {
	Dependencies []string `json:"dependencies"`
}

type LeavesResponse struct {
	Items []string `json:"items"`
}

type TapInfoResponse struct {
	Info *brew.TapInfo `json:"info"`
}

type ServicesListResponse struct {
	Items []services.Service `json:"items"`
}

type JobSubmitOptions struct {
	Pinned       []string `json:"pinned,omitempty"`
	StrictNative bool     `json:"strict_native,omitempty"`
}

type JobSubmitRequest struct {
	Operation string           `json:"operation"`
	Packages  []string         `json:"packages"`
	Options   JobSubmitOptions `json:"options,omitempty"`
}

type JobSubmitResponse struct {
	JobID string `json:"job_id"`
}

type JobStatusRequest struct {
	JobID string `json:"job_id"`
}

type JobStreamRequest struct {
	JobID    string `json:"job_id"`
	FromSeq  int    `json:"from_seq"`
	Blocking bool   `json:"blocking,omitempty"`
}

type JobEvent struct {
	Seq       int       `json:"seq"`
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Kind      string    `json:"kind,omitempty"`
	Operation string    `json:"operation,omitempty"`
	Package   string    `json:"package,omitempty"`
	Phase     string    `json:"phase,omitempty"`
	Status    string    `json:"status,omitempty"`
	Current   *int64    `json:"current,omitempty"`
	Total     *int64    `json:"total,omitempty"`
	Unit      string    `json:"unit,omitempty"`
}

type JobView struct {
	ID         string     `json:"id"`
	Operation  string     `json:"operation"`
	Packages   []string   `json:"packages"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	Submitted  time.Time  `json:"submitted_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type JobStatusResponse struct {
	Job JobView `json:"job"`
}

type JobStreamResponse struct {
	Job    JobView    `json:"job"`
	Events []JobEvent `json:"events"`
}
