package tui

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type item struct {
	title       string
	desc        string
	installed   bool
}

func (i item) Title() string {
	if i.installed {
		return "✅ " + i.title
	}
	return i.title
}
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type model struct {
	list      list.Model
	client    *brew.Client
	index     *brew.Index
	installed map[string]bool
	loaded    bool
	err       error
}

type installedMsg map[string]bool

func InitialModel() model {
	client, _ := brew.NewClient()
	return model{
		client:    client,
		list:      list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0),
		installed: make(map[string]bool),
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
			// Install selected item
			if i, ok := m.list.SelectedItem().(item); ok {
				exe, _ := os.Executable()
				return m, tea.ExecProcess(exec.Command(exe, "install", i.title), func(err error) tea.Msg {
					if err != nil {
						return err
					}
					// Refetch installed list
					client, _ := brew.NewClient()
					pkgs, _ := client.ListInstalled()
					inst := make(map[string]bool)
					for _, p := range pkgs {
						inst[p.Name] = true
					}
					return installedMsg(inst)
				})
			}
		}

	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

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
		})
	}
	for _, c := range m.index.Casks {
		items = append(items, item{
			title:     c.Token,
			desc:      c.Desc,
			installed: m.installed[c.Token],
		})
	}
	cmd := m.list.SetItems(items)
	m.list.Title = "FastBrew Packages"
	return cmd
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v", m.err)
	}
	if !m.loaded {
		return "Loading FastBrew Index (this happens once per day)..."
	}
	return docStyle.Render(m.list.View())
}

func Start() error {
	p := tea.NewProgram(InitialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}