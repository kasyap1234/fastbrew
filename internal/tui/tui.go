package tui

import (
	"errors"
	"fastbrew/internal/brew"
	"fastbrew/internal/config"
	"fastbrew/internal/daemon"
	"fastbrew/internal/progress"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)
var jobPanelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)

const (
	maxJobPanelPackages = 6
	maxJobPanelLogs     = 3
	jobPanelHeight      = 12
)

type item struct {
	title     string
	desc      string
	installed bool
	isCask    bool
}

func (i item) Title() string {
	prefix := ""
	if i.installed {
		prefix = "✅ "
	}
	if i.isCask {
		return prefix + "🍷 " + i.title
	}
	return prefix + i.title
}
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type model struct {
	list      list.Model
	client    *brew.Client
	index     *brew.Index
	installed map[string]bool
	width     int
	height    int

	loaded      bool
	err         error
	jobActive   bool
	jobVisible  bool
	jobSource   string
	jobTarget   string
	jobStatus   string
	jobError    string
	jobEvents   chan tea.Msg
	jobLogs     []string
	jobPackages map[string]*packageProgress
}

type installedMsg map[string]bool
type jobEventMsg struct {
	Event daemon.JobEvent
}
type jobStatusMsg struct {
	Status string
	Error  string
}
type jobFinishedMsg struct {
	Err       error
	Installed map[string]bool
}
type jobWorkerStartedMsg struct{}
type jobWorkerClosedMsg struct{}

type packageProgress struct {
	Name      string
	Phase     string
	Status    string
	Current   int64
	Total     int64
	Unit      string
	Percent   float64
	UpdatedAt time.Time
}

func InitialModel() model {
	client, _ := brew.NewClient()
	return model{
		client:      client,
		list:        list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		installed:   make(map[string]bool),
		jobPackages: make(map[string]*packageProgress),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg {
			idx, err := m.client.LoadIndex()
			if err != nil {
				return err
			}
			return idx
		},
		func() tea.Msg {
			pkgs, err := m.client.ListInstalled()
			if err != nil {
				return err // Or ignore
			}
			inst := make(map[string]bool)
			for _, p := range pkgs {
				inst[p.Name] = true
			}
			return installedMsg(inst)
		},
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			if m.jobActive {
				return m, nil
			}

			if i, ok := m.list.SelectedItem().(item); ok {
				events := make(chan tea.Msg, 1024)
				m.jobActive = true
				m.jobVisible = true
				m.jobSource = "local"
				m.jobTarget = i.title
				m.jobStatus = daemon.JobStatusQueued
				m.jobError = ""
				m.jobEvents = events
				m.jobLogs = nil
				m.jobPackages = make(map[string]*packageProgress)
				m.updateListSize()

				if daemonClient, daemonErr := daemonClientForTUI(); daemonErr == nil {
					m.jobSource = "daemon"
					return m, tea.Batch(
						startJobWorker(events, func() {
							runDaemonInstall(events, daemonClient, m.client, i.title)
						}),
						waitForJobMsg(events),
					)
				}

				return m, tea.Batch(
					startJobWorker(events, func() {
						runLocalInstall(events, m.client, i.title)
					}),
					waitForJobMsg(events),
				)
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateListSize()

	case error:
		m.err = msg
		return m, tea.Quit

	case installedMsg:
		m.installed = msg
		// If index is already loaded, refresh list
		if m.index != nil {
			return m, m.updateListItems()
		}

	case *brew.Index:
		m.index = msg
		m.loaded = true
		return m, m.updateListItems()

	case jobWorkerStartedMsg:
		return m, waitForJobMsg(m.jobEvents)

	case jobEventMsg:
		m.applyJobEvent(msg.Event)
		if m.jobActive {
			return m, waitForJobMsg(m.jobEvents)
		}
		return m, nil

	case jobStatusMsg:
		if msg.Status != "" {
			m.jobStatus = msg.Status
		}
		if msg.Error != "" {
			m.jobError = msg.Error
			m.appendJobLog(msg.Error)
		}
		if m.jobActive {
			return m, waitForJobMsg(m.jobEvents)
		}
		return m, nil

	case jobFinishedMsg:
		m.jobActive = false
		if msg.Err != nil {
			m.jobStatus = daemon.JobStatusFailed
			m.jobError = msg.Err.Error()
			m.appendJobLog(fmt.Sprintf("install failed: %v", msg.Err))
		} else {
			m.jobStatus = daemon.JobStatusSucceeded
			m.appendJobLog("install completed")
		}
		if msg.Installed != nil {
			m.installed = msg.Installed
			if m.index != nil {
				cmd := m.updateListItems()
				m.updateListSize()
				return m, cmd
			}
		}
		m.updateListSize()
		return m, nil

	case jobWorkerClosedMsg:
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) updateListItems() tea.Cmd {
	var items []list.Item
	for _, f := range m.index.Formulae {
		items = append(items, item{
			title:     f.Name,
			desc:      f.Desc,
			installed: m.installed[f.Name],
			isCask:    false,
		})
	}
	for _, c := range m.index.Casks {
		items = append(items, item{
			title:     c.Token,
			desc:      c.Desc,
			installed: m.installed[c.Token],
			isCask:    true,
		})
	}
	cmd := m.list.SetItems(items)
	if m.jobActive {
		m.list.Title = fmt.Sprintf("FastBrew Packages (installing %s via %s)", m.jobTarget, m.jobSource)
	} else {
		m.list.Title = "FastBrew Packages"
	}
	return cmd
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}
	if !m.loaded {
		return "Loading FastBrew Index (this happens once per day)..."
	}

	view := m.list.View()
	if m.jobVisible {
		view = view + "\n" + m.renderJobPanel()
	}
	return docStyle.Render(view)
}

func Start() error {
	p := tea.NewProgram(InitialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m *model) updateListSize() {
	if m.width == 0 || m.height == 0 {
		return
	}

	h, v := docStyle.GetFrameSize()
	panel := 0
	if m.jobVisible {
		panel = jobPanelHeight
	}

	listHeight := m.height - v - panel
	if listHeight < 6 {
		listHeight = 6
	}
	listWidth := m.width - h
	if listWidth < 20 {
		listWidth = 20
	}
	m.list.SetSize(listWidth, listHeight)
}

func (m *model) applyJobEvent(event daemon.JobEvent) {
	if event.Message != "" && event.Status != daemon.JobEventStatusProgress {
		m.appendJobLog(event.Message)
	}

	if event.Kind != daemon.JobEventKindPackage || event.Package == "" {
		return
	}

	entry, ok := m.jobPackages[event.Package]
	if !ok {
		entry = &packageProgress{Name: event.Package}
		m.jobPackages[event.Package] = entry
	}

	if event.Phase != "" {
		entry.Phase = event.Phase
	}
	if event.Status != "" {
		entry.Status = event.Status
	}
	if event.Unit != "" {
		entry.Unit = event.Unit
	}
	if event.Current != nil {
		entry.Current = *event.Current
	}
	if event.Total != nil {
		entry.Total = *event.Total
	}

	if entry.Total > 0 {
		entry.Percent = math.Min(100, math.Max(0, float64(entry.Current)/float64(entry.Total)*100))
	} else if entry.Status == daemon.JobEventStatusSucceeded {
		entry.Percent = 100
	}
	if entry.Status == daemon.JobEventStatusSucceeded {
		entry.Percent = 100
		if entry.Total > 0 {
			entry.Current = entry.Total
		}
	}
	entry.UpdatedAt = time.Now()
}

func (m *model) appendJobLog(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}

	m.jobLogs = append(m.jobLogs, line)
	if len(m.jobLogs) > 50 {
		m.jobLogs = m.jobLogs[len(m.jobLogs)-50:]
	}
}

func (m model) renderJobPanel() string {
	rows := m.sortedJobPackages()

	var lines []string
	summary := fmt.Sprintf("Job %s (%s): %s", daemon.JobOperationInstall, m.jobSource, m.jobStatus)
	if m.jobTarget != "" {
		summary = fmt.Sprintf("%s [%s]", summary, m.jobTarget)
	}
	lines = append(lines, summary)

	if m.jobError != "" {
		lines = append(lines, fmt.Sprintf("Error: %s", m.jobError))
	}

	if len(rows) == 0 {
		lines = append(lines, "No package progress yet...")
	} else {
		lines = append(lines, "Packages:")
		for i, row := range rows {
			if i >= maxJobPanelPackages {
				break
			}
			lines = append(lines, formatPackageRow(row))
		}
	}

	if len(m.jobLogs) > 0 {
		lines = append(lines, "Logs:")
		start := 0
		if len(m.jobLogs) > maxJobPanelLogs {
			start = len(m.jobLogs) - maxJobPanelLogs
		}
		for _, line := range m.jobLogs[start:] {
			lines = append(lines, "  "+line)
		}
	}

	panel := strings.Join(lines, "\n")
	if m.width > 6 {
		return jobPanelStyle.Width(m.width - 6).Render(panel)
	}
	return jobPanelStyle.Render(panel)
}

func (m model) sortedJobPackages() []packageProgress {
	out := make([]packageProgress, 0, len(m.jobPackages))
	for _, pkg := range m.jobPackages {
		out = append(out, *pkg)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Name < out[j].Name
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func formatPackageRow(pkg packageProgress) string {
	phase := pkg.Phase
	if phase == "" {
		phase = "install"
	}
	status := pkg.Status
	if status == "" {
		status = "running"
	}

	if pkg.Total > 0 {
		return fmt.Sprintf("  %s %s %s %.1f%%", pkg.Name, phase, status, pkg.Percent)
	}
	return fmt.Sprintf("  %s %s %s", pkg.Name, phase, status)
}

func startJobWorker(events chan tea.Msg, runner func()) tea.Cmd {
	return func() tea.Msg {
		go runner()
		return jobWorkerStartedMsg{}
	}
}

func waitForJobMsg(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return jobWorkerClosedMsg{}
		}
		return msg
	}
}

func daemonClientForTUI() (*daemon.Client, error) {
	cfg := config.Get()
	if !cfg.Daemon.Enabled {
		return nil, errors.New("daemon disabled")
	}

	client := daemon.NewClient(cfg.GetDaemonSocketPath(), "")
	if err := client.Ping(); err != nil {
		return nil, err
	}
	return client, nil
}

func loadInstalledMap(client *brew.Client) (map[string]bool, error) {
	pkgs, err := client.ListInstalled()
	if err != nil {
		return nil, err
	}

	out := make(map[string]bool, len(pkgs))
	for _, pkg := range pkgs {
		out[pkg.Name] = true
	}
	return out, nil
}

func sendBestEffort(events chan<- tea.Msg, msg tea.Msg) {
	select {
	case events <- msg:
	default:
	}
}

func sendBlocking(events chan<- tea.Msg, msg tea.Msg) {
	events <- msg
}

func runDaemonInstall(events chan<- tea.Msg, daemonClient *daemon.Client, brewClient *brew.Client, pkg string) {
	defer close(events)

	jobID, err := daemonClient.SubmitJob(daemon.JobOperationInstall, []string{pkg}, daemon.JobSubmitOptions{})
	if err != nil {
		sendBlocking(events, jobFinishedMsg{Err: err})
		return
	}

	lastStatus := ""
	fromSeq := 0
	for {
		stream, streamErr := daemonClient.JobStream(jobID, fromSeq, true)
		if streamErr != nil {
			sendBlocking(events, jobFinishedMsg{Err: streamErr})
			return
		}

		for _, event := range stream.Events {
			sendBestEffort(events, jobEventMsg{Event: event})
			fromSeq = event.Seq + 1
		}

		if stream.Job.Status != lastStatus || stream.Job.Error != "" {
			sendBestEffort(events, jobStatusMsg{
				Status: stream.Job.Status,
				Error:  stream.Job.Error,
			})
			lastStatus = stream.Job.Status
		}

		switch stream.Job.Status {
		case daemon.JobStatusSucceeded:
			installed, loadErr := loadInstalledMap(brewClient)
			sendBlocking(events, jobFinishedMsg{
				Err:       loadErr,
				Installed: installed,
			})
			return
		case daemon.JobStatusFailed:
			if stream.Job.Error != "" {
				sendBlocking(events, jobFinishedMsg{Err: errors.New(stream.Job.Error)})
			} else {
				sendBlocking(events, jobFinishedMsg{Err: fmt.Errorf("daemon job %s failed", jobID)})
			}
			return
		}
	}
}

type localProgressState struct {
	lastPercent float64
	lastAt      time.Time
}

type localProgressThrottle struct {
	mu    sync.Mutex
	state map[string]localProgressState
}

func newLocalProgressThrottle() *localProgressThrottle {
	return &localProgressThrottle{
		state: make(map[string]localProgressState),
	}
}

func (t *localProgressThrottle) shouldEmit(event progress.ProgressEvent) bool {
	if event.Type == progress.EventDownloadStart || event.Type == progress.EventDownloadComplete || event.Type == progress.EventDownloadError {
		t.mu.Lock()
		t.state[event.ID] = localProgressState{lastPercent: event.CalculatePercentage(), lastAt: time.Now()}
		t.mu.Unlock()
		return true
	}

	now := time.Now()
	percent := event.CalculatePercentage()

	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.state[event.ID]
	if !ok {
		t.state[event.ID] = localProgressState{lastPercent: percent, lastAt: now}
		return true
	}

	if event.Total > 0 && percent-state.lastPercent >= 1.0 {
		t.state[event.ID] = localProgressState{lastPercent: percent, lastAt: now}
		return true
	}

	if now.Sub(state.lastAt) >= 250*time.Millisecond {
		t.state[event.ID] = localProgressState{lastPercent: percent, lastAt: now}
		return true
	}

	return false
}

func progressToJobEvent(event progress.ProgressEvent) daemon.JobEvent {
	status := daemon.JobEventStatusProgress
	level := "info"
	switch event.Type {
	case progress.EventDownloadStart:
		status = daemon.JobEventStatusRunning
	case progress.EventDownloadProgress:
		status = daemon.JobEventStatusProgress
	case progress.EventDownloadComplete:
		status = daemon.JobEventStatusSucceeded
	case progress.EventDownloadError:
		status = daemon.JobEventStatusFailed
		level = "error"
	}

	current := event.Current
	total := event.Total

	return daemon.JobEvent{
		Kind:      daemon.JobEventKindPackage,
		Operation: daemon.JobOperationInstall,
		Package:   event.ID,
		Phase:     daemon.JobEventPhaseDownload,
		Status:    status,
		Level:     level,
		Message:   event.Message,
		Current:   &current,
		Total:     &total,
		Unit:      "bytes",
	}
}

func mutationToJobEvent(event brew.MutationEvent) daemon.JobEvent {
	level := "info"
	switch event.Status {
	case brew.MutationStatusFailed:
		level = "error"
	case brew.MutationStatusSkipped:
		level = "warn"
	}

	var currentPtr *int64
	var totalPtr *int64
	if event.Current != 0 || event.Total != 0 || event.Status == brew.MutationStatusProgress || event.Status == brew.MutationStatusRunning {
		current := event.Current
		total := event.Total
		currentPtr = &current
		totalPtr = &total
	}

	operation := event.Operation
	if operation == "" {
		operation = daemon.JobOperationInstall
	}

	return daemon.JobEvent{
		Kind:      daemon.JobEventKindPackage,
		Operation: operation,
		Package:   event.Package,
		Phase:     event.Phase,
		Status:    event.Status,
		Level:     level,
		Message:   event.Message,
		Current:   currentPtr,
		Total:     totalPtr,
		Unit:      event.Unit,
	}
}

func runLocalInstall(events chan<- tea.Msg, client *brew.Client, pkg string) {
	defer close(events)

	client.EnableProgress()
	pm := client.ProgressManager
	subID := fmt.Sprintf("tui-progress-%d", time.Now().UnixNano())
	progressCh := make(chan progress.ProgressEvent, 256)
	pm.SubscribeToEvents(subID, progressCh)

	stopProgress := make(chan struct{})
	progressDone := make(chan struct{})
	throttle := newLocalProgressThrottle()

	go func() {
		defer close(progressDone)
		for {
			select {
			case <-stopProgress:
				return
			case event := <-progressCh:
				if !throttle.shouldEmit(event) {
					continue
				}
				sendBestEffort(events, jobEventMsg{Event: progressToJobEvent(event)})
			}
		}
	}()

	client.SetMutationHook(func(event brew.MutationEvent) {
		if event.Package == "" {
			return
		}
		sendBestEffort(events, jobEventMsg{Event: mutationToJobEvent(event)})
	})

	sendBestEffort(events, jobStatusMsg{Status: daemon.JobStatusRunning})
	err := client.InstallNative([]string{pkg})
	client.SetMutationHook(nil)
	pm.UnsubscribeFromEvents(subID)
	close(stopProgress)
	<-progressDone

	if err != nil {
		sendBestEffort(events, jobStatusMsg{Status: daemon.JobStatusFailed, Error: err.Error()})
		sendBlocking(events, jobFinishedMsg{Err: err})
		return
	}

	sendBestEffort(events, jobStatusMsg{Status: daemon.JobStatusSucceeded})
	installed, loadErr := loadInstalledMap(client)
	sendBlocking(events, jobFinishedMsg{
		Err:       loadErr,
		Installed: installed,
	})
}
