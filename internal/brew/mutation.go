package brew

const (
	MutationOperationInstall   = "install"
	MutationOperationUpgrade   = "upgrade"
	MutationOperationUninstall = "uninstall"
	MutationOperationReinstall = "reinstall"
)

const (
	MutationPhaseResolve   = "resolve"
	MutationPhaseMetadata  = "metadata"
	MutationPhaseDownload  = "download"
	MutationPhaseExtract   = "extract"
	MutationPhaseLink      = "link"
	MutationPhaseInstall   = "install"
	MutationPhaseUninstall = "uninstall"
	MutationPhaseComplete  = "complete"
)

const (
	MutationStatusQueued    = "queued"
	MutationStatusRunning   = "running"
	MutationStatusProgress  = "progress"
	MutationStatusSucceeded = "succeeded"
	MutationStatusFailed    = "failed"
	MutationStatusSkipped   = "skipped"
)

// MutationEvent describes package-level mutation progress for install/upgrade flows.
type MutationEvent struct {
	Operation string
	Package   string
	Phase     string
	Status    string
	Message   string
	Current   int64
	Total     int64
	Unit      string
}

func (c *Client) emitMutation(operation, pkg, phase, status, message string, current, total int64, unit string) {
	c.notifyMutation(MutationEvent{
		Operation: operation,
		Package:   pkg,
		Phase:     phase,
		Status:    status,
		Message:   message,
		Current:   current,
		Total:     total,
		Unit:      unit,
	})
}
