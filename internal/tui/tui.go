package tui

import (
	"errors"
	"fastbrew/internal/brew"
	"fastbrew/internal/config"
	"fastbrew/internal/daemon"
	"fastbrew/internal/progress"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tprogress "github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	jobPanelStyle = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).Padding(0, 1).BorderForeground(lipgloss.Color("62"))

	titleStyle        = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("205")).Bold(true)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	installedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	spinnerStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	helpStyle         = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)
	searchInputStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Padding(0, 0, 0, 2)
)

const (
	maxJobPanelPackages = 6
	maxJobPanelLogs     = 3
	jobPanelHeight      = 14
)

type item struct {
	title     string
	desc      string
	installed bool
	isCask    bool
}

func (i item) Title() string {
	return i.title
}
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type itemDelegate struct{}

func (d itemDelegate) Height() int                             { return 1 }
func (d itemDelegate) Spacing() int                            { return 0 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := i.title
	if i.isCask {
		str = "🍷 " + str
	} else {
		str = "📦 " + str
	}

	if i.installed {
		str = "✅ " + str
	} else {
		str = "   " + str
	}

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type model struct {
	list      list.Model
	client    *brew.Client
	index     *brew.Index
	installed map[string]bool
	width     int
	height    int

	loaded  bool
	err     error
	spinner spinner.Model

	searchInput textinput.Model
	searchQuery string
	searching   bool
	searchReady bool

	jobActive   bool
	jobVisible  bool
	jobSource   string
	jobTarget   string
	jobStatus   string
	jobError    string
	jobEvents   chan tea.Msg
	jobLogs     []string
	jobPackages map[string]*packageProgress
	progressBar tprogress.Model
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

type searchResultMsg struct {
	Items      []brew.SearchItem
	Query      string
	FromDaemon bool
}

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

func InitialModel() *model {
	client, _ := brew.NewClient()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	ti := textinput.New()
	ti.Placeholder = "Type to search packages..."
	ti.Focus()
	ti.CharLimit = 64
	ti.Width = 40
	ti.Prompt = "🔍 "
	ti.PromptStyle = searchInputStyle

	p := tprogress.New(tprogress.WithDefaultGradient())

	delegate := itemDelegate{}
	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "FastBrew Packages"
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	return &model{
		client:      client,
		list:        l,
		installed:   make(map[string]bool),
		jobPackages: make(map[string]*packageProgress),
		spinner:     s,
		progressBar: p,
		searchInput: ti,
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		textinput.Blink,
		func() tea.Msg {
			pkgs, err := m.client.ListInstalled()
			if err != nil {
				return err
			}
			inst := make(map[string]bool)
			for _, p := range pkgs {
				inst[p.Name] = true
			}
			return installedMsg(inst)
		},
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		if !m.loaded {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.searchInput.Focused() {
			switch msg.String() {
			case "enter":
				m.searchInput.Blur()
				m.searchQuery = m.searchInput.Value()
				m.searching = true
				return m, m.performSearch(m.searchQuery)
			case "esc":
				m.searchInput.SetValue("")
				m.searchInput.Blur()
				m.searchQuery = ""
				m.searching = false
				m.searchReady = false
				m.list.SetItems([]list.Item{})
				m.list.Title = "FastBrew Packages"
				return m, nil
			}
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}

		if msg.String() == "/" {
			m.searchInput.Focus()
			m.searchInput.SetValue("")
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			return m, cmd
		}

		if msg.String() == "enter" || msg.String() == "i" {
			if m.jobActive {
				return m, nil
			}
			if i, ok := m.list.SelectedItem().(item); ok {
				return m, m.startJob(i.title, daemon.JobOperationInstall)
			}
		}

		if msg.String() == "u" || msg.String() == "x" {
			if m.jobActive {
				return m, nil
			}
			if i, ok := m.list.SelectedItem().(item); ok && i.installed {
				return m, m.startJob(i.title, daemon.JobOperationUninstall)
			}
		}

	case tprogress.FrameMsg:
		newModel, cmd := m.progressBar.Update(msg)
		if pm, ok := newModel.(tprogress.Model); ok {
			m.progressBar = pm
		}
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progressBar.Width = msg.Width - 20
		m.updateListSize()

	case error:
		m.err = msg
		return m, tea.Quit

	case installedMsg:
		m.installed = msg
		m.loaded = true
		if m.index != nil {
			return m, m.updateListItems()
		}

	case *brew.Index:
		m.index = msg
		m.loaded = true
		return m, m.updateListItems()

	case searchResultMsg:
		m.searching = false
		m.searchReady = true
		var items []list.Item
		for _, si := range msg.Items {
			items = append(items, item{
				title:     si.Name,
				desc:      si.Desc,
				installed: m.installed[si.Name],
				isCask:    si.IsCask,
			})
		}
		m.list.SetItems(items)
		source := "local"
		if msg.FromDaemon {
			source = "daemon"
		}
		if msg.Query != "" {
			m.list.Title = fmt.Sprintf("Search: '%s' (%s, %d results)", msg.Query, source, len(items))
		} else {
			m.list.Title = "FastBrew Packages"
		}
		m.updateListSize()
		return m, nil

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
			m.appendJobLog(fmt.Sprintf("job failed: %v", msg.Err))
		} else {
			m.jobStatus = daemon.JobStatusSucceeded
			m.appendJobLog("job completed")
		}
		if msg.Installed != nil {
			m.installed = msg.Installed
			if m.searchReady {
				return m, m.refreshSearchResultsCmd()
			} else if m.index != nil {
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

func (m *model) performSearch(query string) tea.Cmd {
	return func() tea.Msg {
		daemonClient, daemonErr := daemonClientForTUI()
		if daemonClient != nil && daemonErr == nil {
			results, err := daemonClient.Search(query)
			if err == nil {
				return searchResultMsg{
					Items:      results,
					Query:      query,
					FromDaemon: true,
				}
			}
		}

		prefixIdx, err := m.client.GetPrefixIndex()
		if err != nil {
			return err
		}
		matches := prefixIdx.SearchFuzzy(query)
		items := prefixIdx.GetItems()
		result := make([]brew.SearchItem, len(matches))
		for i, match := range matches {
			result[i] = items[match.Index]
		}
		return searchResultMsg{
			Items:      result,
			Query:      query,
			FromDaemon: false,
		}
	}
}

// refreshSearchResultsCmd returns a tea.Cmd that safely refreshes search results.
// This follows Bubble Tea's message-passing architecture instead of direct state mutation.
func (m *model) refreshSearchResultsCmd() tea.Cmd {
	if m.searchQuery == "" {
		return nil
	}
	return m.performSearch(m.searchQuery)
}

func (m *model) startJob(target string, operation string) tea.Cmd {
	events := make(chan tea.Msg, 1024)
	m.jobActive = true
	m.jobVisible = true
	m.jobSource = "local"
	m.jobTarget = target
	m.jobStatus = daemon.JobStatusQueued
	m.jobError = ""
	m.jobEvents = events
	m.jobLogs = nil
	m.jobPackages = make(map[string]*packageProgress)
	m.updateListSize()

	runFunc := func() { runLocalInstall(events, m.client, target) }
	if operation == daemon.JobOperationUninstall {
		runFunc = func() { runLocalUninstall(events, m.client, target) }
	}

	if daemonClient, daemonErr := daemonClientForTUI(); daemonErr == nil {
		m.jobSource = "daemon"
		runFunc = func() { runDaemonJob(events, daemonClient, m.client, target, operation) }
	}

	return tea.Batch(
		startJobWorker(events, runFunc),
		waitForJobMsg(events),
	)
}

func (m *model) updateListItems() tea.Cmd {
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

func (m *model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}
	if !m.loaded {
		return fmt.Sprintf("\n\n   %s Loading installed packages...", m.spinner.View())
	}

	var parts []string

	searchLine := m.searchInput.View()
	parts = append(parts, searchLine)

	if m.searching {
		parts = append(parts, fmt.Sprintf("   %s Searching...", m.spinner.View()))
	}

	view := m.list.View()
	parts = append(parts, view)

	if m.jobVisible {
		parts = append(parts, m.renderJobPanel())
	}

	return docStyle.Render(strings.Join(parts, "\n"))
}

// Start launches the TUI with terminal capability detection for graceful degradation.
// It checks terminal capabilities and adjusts the UI accordingly.
func Start() error {
	// Check terminal capabilities first
	if err := CheckTerminal(); err != nil {
		return err
	}

	caps := DetectTerminalCapabilities()

	// Configure program options based on terminal capabilities
	var opts []tea.ProgramOption
	if caps.CanUseAltScreen() {
		opts = append(opts, tea.WithAltScreen())
	}

	// Add mouse support for capable terminals
	if caps.CanUseFancyUI() {
		opts = append(opts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(InitialModel(), opts...)
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

	listHeight := m.height - v - panel - 2
	if listHeight < 6 {
		listHeight = 6
	}
	listWidth := m.width - h
	if listWidth < 20 {
		listWidth = 20
	}
	m.searchInput.Width = listWidth - 4
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

func (m *model) renderJobPanel() string {
	rows := m.sortedJobPackages()

	var lines []string
	summary := fmt.Sprintf("Job %s (%s): %s", m.jobStatus, m.jobSource, m.jobTarget)

	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	if m.jobStatus == daemon.JobStatusSucceeded {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	} else if m.jobStatus == daemon.JobStatusFailed {
		statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	}

	lines = append(lines, statusStyle.Render(summary))

	if m.jobError != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(fmt.Sprintf("Error: %s", m.jobError)))
	}

	if len(rows) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Waiting for job details..."))
	} else {
		for i, row := range rows {
			if i >= maxJobPanelPackages {
				break
			}

			bar := m.progressBar.ViewAs(row.Percent / 100)
			if row.Total <= 0 {
				bar = m.progressBar.ViewAs(0)
				if row.Status == daemon.JobEventStatusSucceeded {
					bar = m.progressBar.ViewAs(1)
				}
			}

			line := fmt.Sprintf("  %-15s %-10s %s", row.Name, row.Phase, bar)
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")

	if len(m.jobLogs) > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render("Logs:"))
		start := 0
		if len(m.jobLogs) > maxJobPanelLogs {
			start = len(m.jobLogs) - maxJobPanelLogs
		}
		for _, line := range m.jobLogs[start:] {
			lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render("  "+line))
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

func runDaemonJob(events chan<- tea.Msg, daemonClient *daemon.Client, brewClient *brew.Client, pkg string, operation string) {
	defer close(events)

	jobID, err := daemonClient.SubmitJob(operation, []string{pkg}, daemon.JobSubmitOptions{})
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

func runLocalUninstall(events chan<- tea.Msg, client *brew.Client, pkg string) {
	defer close(events)

	sendBestEffort(events, jobStatusMsg{Status: daemon.JobStatusRunning})

	pkgPath := filepath.Join(client.Cellar, pkg)

	if _, err := os.Stat(pkgPath); os.IsNotExist(err) {
		sendBestEffort(events, jobStatusMsg{Status: daemon.JobStatusFailed, Error: fmt.Sprintf("%s is not installed", pkg)})
		sendBlocking(events, jobFinishedMsg{Err: err})
		return
	}

	sendBestEffort(events, jobEventMsg{Event: daemon.JobEvent{Kind: daemon.JobEventKindPackage, Operation: daemon.JobOperationUninstall, Package: pkg, Phase: brew.MutationPhaseUninstall, Status: daemon.JobEventStatusRunning}})

	client.Unlink(pkg)

	optLink := filepath.Join(client.Prefix, "opt", pkg)
	if info, err := os.Lstat(optLink); err == nil && info.Mode()&os.ModeSymlink != 0 {
		os.Remove(optLink)
	}

	if err := os.RemoveAll(pkgPath); err != nil {
		sendBestEffort(events, jobStatusMsg{Status: daemon.JobStatusFailed, Error: err.Error()})
		sendBlocking(events, jobFinishedMsg{Err: err})
		return
	}

	sendBestEffort(events, jobEventMsg{Event: daemon.JobEvent{Kind: daemon.JobEventKindPackage, Operation: daemon.JobOperationUninstall, Package: pkg, Phase: brew.MutationPhaseUninstall, Status: daemon.JobEventStatusSucceeded}})
	sendBestEffort(events, jobStatusMsg{Status: daemon.JobStatusSucceeded})

	installed, loadErr := loadInstalledMap(client)
	sendBlocking(events, jobFinishedMsg{
		Err:       loadErr,
		Installed: installed,
	})
}
