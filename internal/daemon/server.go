package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fastbrew/internal/brew"
	"fastbrew/internal/services"
)

type ServerOptions struct {
	SocketPath    string
	IdleTimeout   time.Duration
	BinaryVersion string
	Prewarm       bool
}

type Server struct {
	socketPath    string
	idleTimeout   time.Duration
	binaryVersion string
	prewarm       bool

	startedAt    time.Time
	lastActivity atomic.Int64

	cache  *Cache
	client *brew.Client
	jobs   *JobManager

	mutationMu sync.Mutex

	listener net.Listener
	stopOnce sync.Once
}

func NewServer(opts ServerOptions) (*Server, error) {
	if opts.SocketPath == "" {
		return nil, fmt.Errorf("socket path is required")
	}

	client, err := brew.NewClient()
	if err != nil {
		return nil, err
	}

	s := &Server{
		socketPath:    opts.SocketPath,
		idleTimeout:   opts.IdleTimeout,
		binaryVersion: opts.BinaryVersion,
		prewarm:       opts.Prewarm,
		cache:         NewCache(),
		client:        client,
		jobs:          NewJobManager(),
		startedAt:     time.Now(),
	}
	s.client.SetInvalidationHook(func(event string) {
		s.cache.invalidate(event)
	})
	s.touch()
	return s, nil
}

func (s *Server) Serve(ctx context.Context) error {
	if err := s.prepareSocket(); err != nil {
		return err
	}

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.socketPath, err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("failed to chmod socket: %w", err)
	}
	s.listener = ln

	if s.prewarm {
		go func() {
			_ = s.Warmup()
		}()
	}

	idleCtx, cancelIdle := context.WithCancel(context.Background())
	defer cancelIdle()

	go s.watchIdleTimeout(idleCtx)
	go func() {
		<-ctx.Done()
		cancelIdle()
		_ = s.Close()
	}()

	for {
		conn, acceptErr := s.listener.Accept()
		if acceptErr != nil {
			if errors.Is(acceptErr, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept failed: %w", acceptErr)
		}
		s.touch()
		go s.handleConn(conn)
	}
}

func (s *Server) ServeUntilInterrupted() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return s.Serve(ctx)
}

func (s *Server) Close() error {
	var closeErr error
	s.stopOnce.Do(func() {
		if s.listener != nil {
			closeErr = s.listener.Close()
		}
		_ = os.Remove(s.socketPath)
	})
	return closeErr
}

func (s *Server) Warmup() error {
	if _, err := s.client.LoadIndex(); err != nil {
		return err
	}
	if _, err := s.client.GetPrefixIndex(); err != nil {
		return err
	}
	if _, err := s.cache.loadInstalled(s.client.ListInstalledNative); err != nil {
		return err
	}
	s.cache.markWarmup()
	s.touch()
	return nil
}

func (s *Server) prepareSocket() error {
	runDir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(runDir, 0700); err != nil {
		return fmt.Errorf("failed to create run dir: %w", err)
	}

	info, err := os.Lstat(s.socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to overwrite non-socket path: %s", s.socketPath)
	}

	if err := os.Remove(s.socketPath); err != nil {
		return fmt.Errorf("failed to remove stale socket: %w", err)
	}
	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	handshakeDone := false

	for {
		var req Request
		if err := decoder.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
			return
		}
		s.touch()
		s.cache.TrackRequest()

		if !handshakeDone && req.Type != RequestHandshake {
			_ = writeErrorResponse(encoder, ResponseCodeVer, fmt.Errorf("handshake required before request"))
			return
		}

		switch req.Type {
		case RequestHandshake:
			if err := s.handleHandshake(encoder, req.Payload); err != nil {
				return
			}
			handshakeDone = true
		case RequestPing:
			_ = writeOKResponse(encoder, map[string]string{"status": "ok"})
		case RequestStatus:
			resp := StatusResponse{
				PID:             os.Getpid(),
				SocketPath:      s.socketPath,
				StartedAt:       s.startedAt,
				LastActivityAt:  s.lastActivityTime(),
				IdleTimeoutSecs: int(s.idleTimeout.Seconds()),
			}
			_ = writeOKResponse(encoder, resp)
		case RequestStats:
			stats := s.cache.stats(s.startedAt)
			jobStats := s.jobs.Stats()
			stats.JobsTotal = jobStats.Total
			stats.JobsRunning = jobStats.Running
			stats.JobsFailed = jobStats.Failed
			_ = writeOKResponse(encoder, stats)
		case RequestWarmup:
			if err := s.Warmup(); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, map[string]string{"status": "warmed"})
		case RequestShutdown:
			_ = writeOKResponse(encoder, map[string]string{"status": "shutting_down"})
			go func() {
				_ = s.Close()
			}()
			return
		case RequestInvalidate:
			var payload InvalidateRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			s.cache.invalidate(payload.Event)
			_ = writeOKResponse(encoder, map[string]string{"status": "invalidated"})
		case RequestSearch:
			var payload SearchRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			items, err := s.cache.loadSearch(payload.Query, s.client.SearchFuzzyWithIndex)
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, SearchResponse{Items: items})
		case RequestList:
			items, err := s.cache.loadInstalled(s.client.ListInstalledNative)
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, ListResponse{Items: items})
		case RequestOutdated:
			items, err := s.cache.loadOutdated(s.client.GetOutdated)
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, OutdatedResponse{Items: items})
		case RequestInfo:
			var payload InfoRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			info, err := s.loadPackageInfo(payload.Packages)
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, InfoResponse{Packages: info})
		case RequestDeps:
			var payload DepsRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			deps, err := s.cache.loadDeps(payload.Packages, func() ([]string, error) {
				return s.client.ResolveDeps(payload.Packages)
			})
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, DepsResponse{Dependencies: deps})
		case RequestLeaves:
			leaves, err := s.cache.loadLeaves(s.computeLeaves)
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, LeavesResponse{Items: leaves})
		case RequestTapInfo:
			var payload TapInfoRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			info, err := s.cache.loadTapInfo(payload.Repo, payload.InstalledOnly, func() (*brew.TapInfo, error) {
				manager, managerErr := brew.NewTapManager()
				if managerErr != nil {
					return nil, managerErr
				}
				return manager.GetTapInfo(payload.Repo, payload.InstalledOnly)
			})
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, TapInfoResponse{Info: info})
		case RequestServices:
			var payload ServicesListRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			svcs, err := s.cache.loadServices(payload.Scope, func() ([]services.Service, error) {
				if payload.Scope == "" {
					return services.NewServiceManager().ListServices()
				}
				manager, managerErr := services.NewServiceManagerWithScope(services.ServiceScope(payload.Scope))
				if managerErr != nil {
					return nil, managerErr
				}
				return manager.ListServices()
			})
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, ServicesListResponse{Items: svcs})
		case RequestJobSubmit:
			var payload JobSubmitRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			jobID, err := s.submitJob(payload)
			if err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeErr, err)
				continue
			}
			_ = writeOKResponse(encoder, JobSubmitResponse{JobID: jobID})
		case RequestJobStatus:
			var payload JobStatusRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			job, ok := s.jobs.Status(payload.JobID)
			if !ok {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, fmt.Errorf("job %s not found", payload.JobID))
				continue
			}
			_ = writeOKResponse(encoder, JobStatusResponse{Job: job})
		case RequestJobStream:
			var payload JobStreamRequest
			if err := json.Unmarshal(req.Payload, &payload); err != nil {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
				continue
			}
			job, events, ok := s.jobs.Stream(payload.JobID, payload.FromSeq, payload.Blocking)
			if !ok {
				_ = writeErrorResponse(encoder, ResponseCodeBadReq, fmt.Errorf("job %s not found", payload.JobID))
				continue
			}
			_ = writeOKResponse(encoder, JobStreamResponse{Job: job, Events: events})
		default:
			_ = writeErrorResponse(encoder, ResponseCodeBadReq, fmt.Errorf("unknown request type %q", req.Type))
		}
	}
}

func (s *Server) handleHandshake(encoder *json.Encoder, payload json.RawMessage) error {
	var req HandshakeRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		_ = writeErrorResponse(encoder, ResponseCodeBadReq, err)
		return err
	}

	if req.APIVersion != APIVersion {
		_ = writeErrorResponse(encoder, ResponseCodeVer, fmt.Errorf("api mismatch: daemon=%d client=%d", APIVersion, req.APIVersion))
		return fmt.Errorf("api mismatch")
	}
	if s.binaryVersion != "" && req.BinaryVersion != "" && req.BinaryVersion != s.binaryVersion {
		_ = writeErrorResponse(encoder, ResponseCodeVer, fmt.Errorf("binary mismatch: daemon=%s client=%s", s.binaryVersion, req.BinaryVersion))
		return fmt.Errorf("binary mismatch")
	}

	return writeOKResponse(encoder, HandshakeResponse{
		APIVersion:    APIVersion,
		BinaryVersion: s.binaryVersion,
	})
}

func (s *Server) loadPackageInfo(packages []string) ([]PackageInfo, error) {
	info := make([]PackageInfo, 0, len(packages))
	for _, name := range packages {
		formula, err := s.cache.loadFormula(name, s.client.FetchFormula)
		if err == nil {
			info = append(info, PackageInfo{
				Name:         formula.Name,
				Version:      formula.Versions.Stable,
				Desc:         formula.Desc,
				Homepage:     formula.Homepage,
				Dependencies: formula.Dependencies,
				KegOnly:      formula.KegOnly,
			})
			continue
		}

		caskMetadata, caskErr := s.cache.loadCask(name, s.client.FetchCaskMetadata)
		if caskErr != nil {
			return nil, fmt.Errorf("failed to fetch %s: %w", name, err)
		}
		info = append(info, PackageInfo{
			Name:    caskMetadata.Token,
			Version: caskMetadata.Version,
			Desc:    caskMetadata.Desc,
		})
	}
	return info, nil
}

func (s *Server) computeLeaves() ([]string, error) {
	installed, err := s.cache.loadInstalled(s.client.ListInstalledNative)
	if err != nil {
		return nil, err
	}
	if len(installed) == 0 {
		return nil, nil
	}

	idx, err := s.client.LoadIndex()
	if err != nil {
		return nil, err
	}
	formulaMap := make(map[string]brew.Formula, len(idx.Formulae))
	for _, formula := range idx.Formulae {
		formulaMap[formula.Name] = formula
	}

	isDependency := make(map[string]bool)
	for _, pkg := range installed {
		if pkg.IsCask {
			continue
		}
		if formula, ok := formulaMap[pkg.Name]; ok {
			for _, dep := range formula.Dependencies {
				isDependency[dep] = true
			}
		}
	}

	leaves := make([]string, 0, len(installed))
	for _, pkg := range installed {
		if pkg.IsCask {
			continue
		}
		if !isDependency[pkg.Name] {
			leaves = append(leaves, pkg.Name)
		}
	}
	return leaves, nil
}

func (s *Server) submitJob(req JobSubmitRequest) (string, error) {
	operation := strings.ToLower(strings.TrimSpace(req.Operation))
	switch operation {
	case JobOperationInstall, JobOperationUpgrade, JobOperationUninstall, JobOperationReinstall:
	default:
		return "", fmt.Errorf("unsupported job operation %q", req.Operation)
	}

	job := s.jobs.Submit(operation, req.Packages, func(job *Job) error {
		s.mutationMu.Lock()
		defer s.mutationMu.Unlock()

		cleanup := s.attachJobEventBridges(job)
		defer cleanup()

		switch operation {
		case JobOperationInstall:
			return s.executeInstallJob(job, req.Packages)
		case JobOperationUpgrade:
			return s.executeUpgradeJob(job, req.Packages, req.Options.Pinned)
		case JobOperationUninstall:
			return s.executeUninstallJob(job, req.Packages)
		case JobOperationReinstall:
			return s.executeReinstallJob(job, req.Packages)
		default:
			return fmt.Errorf("unknown job operation %q", operation)
		}
	})

	return job.id, nil
}

func (s *Server) executeInstallJob(job *Job, packages []string) error {
	if len(packages) == 0 {
		return fmt.Errorf("install requires at least one package")
	}
	job.addEvent("info", fmt.Sprintf("Installing packages: %s", strings.Join(packages, ", ")))
	for _, pkg := range packages {
		job.addPackageEvent("info", pkg, JobEventPhaseInstall, JobEventStatusQueued, "package queued", nil, nil, "")
	}
	if err := s.client.InstallNative(packages); err != nil {
		return err
	}
	job.addEvent("info", "Install completed")
	return nil
}

func (s *Server) executeUpgradeJob(job *Job, packages []string, pinned []string) error {
	job.addEvent("info", "Resolving outdated packages")
	outdated, err := s.cache.loadOutdated(s.client.GetOutdated)
	if err != nil {
		return err
	}

	if len(packages) > 0 {
		requested := make(map[string]struct{}, len(packages))
		for _, pkg := range packages {
			requested[pkg] = struct{}{}
		}
		filtered := make([]brew.OutdatedPackage, 0, len(outdated))
		for _, pkg := range outdated {
			if _, ok := requested[pkg.Name]; ok {
				filtered = append(filtered, pkg)
			}
		}
		outdated = filtered
	}

	if len(pinned) > 0 {
		pinnedSet := make(map[string]struct{}, len(pinned))
		for _, name := range pinned {
			pinnedSet[name] = struct{}{}
		}
		filtered := make([]brew.OutdatedPackage, 0, len(outdated))
		for _, pkg := range outdated {
			if _, ok := pinnedSet[pkg.Name]; ok {
				job.addEvent("info", fmt.Sprintf("Skipping pinned package: %s", pkg.Name))
				job.addPackageEvent("warn", pkg.Name, JobEventPhaseInstall, JobEventStatusSkipped, "package is pinned", nil, nil, "")
				continue
			}
			filtered = append(filtered, pkg)
		}
		outdated = filtered
	}

	if len(outdated) == 0 {
		job.addEvent("info", "All packages up to date or pinned.")
		return nil
	}

	job.addEvent("info", fmt.Sprintf("Upgrading %d package(s)", len(outdated)))
	if err := s.client.UpgradeNative(nil, outdated); err != nil {
		return err
	}
	job.addEvent("info", "Upgrade completed")
	return nil
}

func (s *Server) executeUninstallJob(job *Job, packages []string) error {
	if len(packages) == 0 {
		return fmt.Errorf("uninstall requires at least one package")
	}

	caskInstaller := brew.NewCaskInstaller(s.client)
	caskInstaller.SetOperation(brew.MutationOperationUninstall)
	for _, pkg := range packages {
		isCask, _ := s.client.IsCask(pkg)
		if isCask {
			job.addEvent("info", fmt.Sprintf("Uninstalling cask %s", pkg))
			job.addPackageEvent("info", pkg, JobEventPhaseUninstall, JobEventStatusRunning, "uninstalling cask", nil, nil, "")
			if err := caskInstaller.Uninstall(pkg); err != nil {
				job.addEvent("warn", fmt.Sprintf("Failed to uninstall cask %s: %v", pkg, err))
				job.addPackageEvent("error", pkg, JobEventPhaseUninstall, JobEventStatusFailed, err.Error(), nil, nil, "")
			}
			continue
		}

		pkgPath := filepath.Join(s.client.Cellar, pkg)
		if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
			job.addEvent("warn", fmt.Sprintf("%s is not installed", pkg))
			job.addPackageEvent("warn", pkg, JobEventPhaseUninstall, JobEventStatusSkipped, "package is not installed", nil, nil, "")
			continue
		}

		job.addPackageEvent("info", pkg, JobEventPhaseUninstall, JobEventStatusRunning, "removing package", nil, nil, "")
		_ = s.client.Unlink(pkg)
		optLink := filepath.Join(s.client.Prefix, "opt", pkg)
		if info, err := os.Lstat(optLink); err == nil && info.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(optLink)
		}

		if err := os.RemoveAll(pkgPath); err != nil {
			job.addEvent("warn", fmt.Sprintf("Error removing %s: %v", pkg, err))
			job.addPackageEvent("error", pkg, JobEventPhaseUninstall, JobEventStatusFailed, err.Error(), nil, nil, "")
			continue
		}
		job.addEvent("info", fmt.Sprintf("Uninstalled %s", pkg))
		job.addPackageEvent("info", pkg, JobEventPhaseComplete, JobEventStatusSucceeded, "package uninstalled", nil, nil, "")
	}

	s.cache.invalidate(EventInstalledChanged)
	return nil
}

func (s *Server) executeReinstallJob(job *Job, packages []string) error {
	if len(packages) == 0 {
		return fmt.Errorf("reinstall requires at least one package")
	}

	installer := brew.NewCaskInstaller(s.client)
	installer.SetOperation(brew.MutationOperationReinstall)
	for _, pkg := range packages {
		job.addEvent("info", fmt.Sprintf("Reinstalling %s", pkg))
		job.addPackageEvent("info", pkg, JobEventPhaseInstall, JobEventStatusRunning, "reinstall started", nil, nil, "")

		isCask, _ := s.client.IsCask(pkg)
		if isCask {
			if err := installer.Uninstall(pkg); err != nil {
				job.addEvent("warn", fmt.Sprintf("Uninstall warning for %s: %v", pkg, err))
				job.addPackageEvent("warn", pkg, JobEventPhaseUninstall, JobEventStatusFailed, err.Error(), nil, nil, "")
			}
			if err := installer.Install(pkg, s.client.ProgressManager); err != nil {
				job.addEvent("warn", fmt.Sprintf("Error reinstalling cask %s: %v", pkg, err))
				job.addPackageEvent("error", pkg, JobEventPhaseInstall, JobEventStatusFailed, err.Error(), nil, nil, "")
				continue
			}
			job.addPackageEvent("info", pkg, JobEventPhaseComplete, JobEventStatusSucceeded, "cask reinstalled", nil, nil, "")
			continue
		}

		if err := s.client.Unlink(pkg); err != nil {
			job.addEvent("warn", fmt.Sprintf("Unlink warning for %s: %v", pkg, err))
			job.addPackageEvent("warn", pkg, JobEventPhaseUninstall, JobEventStatusFailed, err.Error(), nil, nil, "")
		}
		if err := os.RemoveAll(filepath.Join(s.client.Cellar, pkg)); err != nil {
			job.addEvent("warn", fmt.Sprintf("Removal warning for %s: %v", pkg, err))
			job.addPackageEvent("warn", pkg, JobEventPhaseUninstall, JobEventStatusFailed, err.Error(), nil, nil, "")
		}

		formula, err := s.client.FetchFormula(pkg)
		if err != nil {
			job.addEvent("warn", fmt.Sprintf("Error fetching formula %s: %v", pkg, err))
			job.addPackageEvent("error", pkg, JobEventPhaseMetadata, JobEventStatusFailed, err.Error(), nil, nil, "")
			continue
		}
		if err := s.client.InstallBottle(formula); err != nil {
			job.addEvent("warn", fmt.Sprintf("Error installing %s: %v", pkg, err))
			job.addPackageEvent("error", pkg, JobEventPhaseInstall, JobEventStatusFailed, err.Error(), nil, nil, "")
			continue
		}
		result, err := s.client.Link(formula.Name, formula.Versions.Stable)
		if err != nil {
			job.addEvent("warn", fmt.Sprintf("Error linking %s: %v", pkg, err))
			job.addPackageEvent("error", pkg, JobEventPhaseLink, JobEventStatusFailed, err.Error(), nil, nil, "")
			continue
		}
		if !result.Success {
			job.addEvent("warn", fmt.Sprintf("Reinstalled %s with %d link error(s)", pkg, len(result.Errors)))
			job.addPackageEvent("warn", pkg, JobEventPhaseLink, JobEventStatusFailed, "link completed with errors", nil, nil, "")
			continue
		}

		job.addEvent("info", fmt.Sprintf("Reinstalled %s successfully", pkg))
		job.addPackageEvent("info", pkg, JobEventPhaseComplete, JobEventStatusSucceeded, "formula reinstalled", nil, nil, "")
	}

	s.cache.invalidate(EventInstalledChanged)
	return nil
}

func (s *Server) watchIdleTimeout(ctx context.Context) {
	if s.idleTimeout <= 0 {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			last := s.lastActivityTime()
			if time.Since(last) > s.idleTimeout {
				_ = s.Close()
				return
			}
		}
	}
}

func (s *Server) touch() {
	s.lastActivity.Store(time.Now().UnixNano())
}

func (s *Server) lastActivityTime() time.Time {
	unixNanos := s.lastActivity.Load()
	if unixNanos == 0 {
		return s.startedAt
	}
	return time.Unix(0, unixNanos)
}

func writeOKResponse(encoder *json.Encoder, payload interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return writeErrorResponse(encoder, ResponseCodeErr, err)
	}
	return encoder.Encode(Response{OK: true, Payload: raw})
}

func writeErrorResponse(encoder *json.Encoder, code string, err error) error {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	return encoder.Encode(Response{
		OK:    false,
		Code:  code,
		Error: message,
	})
}
