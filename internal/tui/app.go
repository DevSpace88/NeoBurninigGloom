package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	img "NeoBurningGoom/internal/image"
	"NeoBurningGoom/internal/convert"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 2)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)

	highlightStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1, 2)

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#AD58E4")).
				Bold(true)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))
)

type menuItem struct {
	title string
	desc  string
}

func (m menuItem) Title() string       { return m.title }
func (m menuItem) Description() string  { return m.desc }
func (m menuItem) FilterValue() string  { return m.title }

type session struct {
	state    tea.Model
	previous *session
}

type mainModel struct {
	list     list.Model
	spinner  spinner.Model
	sessions []session
	quitting bool
	err      error
	width    int
	height   int
}

func New() *mainModel {
	items := []list.Item{
		menuItem{title: "🔥 Burn Image", desc: "Burn a disc image (ISO, CDI, CUE/BIN, NRG) to CD/DVD"},
		menuItem{title: "📂 Analyze Image", desc: "Read and display details of a disc image file"},
		menuItem{title: "💿 Drive Status", desc: "Show optical drive info and media status"},
		menuItem{title: "🔄 Convert Image", desc: "Convert between image formats (CDI → CUE/BIN, etc.)"},
		menuItem{title: "📁 Extract Files", desc: "Extract files from an ISO image"},
		menuItem{title: "🧹 Erase Disc", desc: "Erase a rewritable disc (CD-RW, DVD-RW)"},
		menuItem{title: "❌ Quit", desc: "Exit NeoBurningGoom"},
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = selectedItemStyle
	delegate.Styles.NormalTitle = itemStyle

	l := list.New(items, delegate, 60, 20)
	l.Title = titleStyle.Render(" 🔥 NeoBurningGoom ")
	l.Styles.Title = lipgloss.NewStyle()
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))

	return &mainModel{
		list:    l,
		spinner: s,
	}
}

func (m *mainModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.EnterAltScreen)
}

func (m *mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle backMsg from sub-models
	if _, ok := msg.(backMsg); ok {
		if len(m.sessions) > 0 {
			m.sessions = m.sessions[:len(m.sessions)-1]
		}
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-6)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		if msg.String() == "esc" {
			if len(m.sessions) > 0 {
				// Pop back to previous session
				last := len(m.sessions) - 1
				m.sessions = m.sessions[:last]
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	// If we have an active sub-model, delegate to it
	if len(m.sessions) > 0 {
		current := m.sessions[len(m.sessions)-1]
		updated, cmd := current.state.Update(msg)
		m.sessions[len(m.sessions)-1] = session{state: updated, previous: current.previous}
		return m, cmd
	}

	// Main menu handling
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			i, ok := m.list.SelectedItem().(menuItem)
			if !ok {
				return m, nil
			}
			return m.handleMenuChoice(i.title)
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *mainModel) handleMenuChoice(title string) (tea.Model, tea.Cmd) {
	switch {
	case strings.Contains(title, "Burn Image"):
		model := newBurnModel(m.width, m.height)
		m.sessions = append(m.sessions, session{state: model})
		return m, model.Init()
	case strings.Contains(title, "Analyze Image"):
		model := newAnalyzeModel(m.width, m.height)
		m.sessions = append(m.sessions, session{state: model})
		return m, model.Init()
	case strings.Contains(title, "Drive Status"):
		model := newDriveStatusModel()
		m.sessions = append(m.sessions, session{state: model})
		return m, model.Init()
	case strings.Contains(title, "Convert Image"):
		model := newConvertModel(m.width, m.height)
		m.sessions = append(m.sessions, session{state: model})
		return m, model.Init()
	case strings.Contains(title, "Extract Files"):
		model := newExtractModel(m.width, m.height)
		m.sessions = append(m.sessions, session{state: model})
		return m, model.Init()
	case strings.Contains(title, "Erase Disc"):
		model := newEraseModel()
		m.sessions = append(m.sessions, session{state: model})
		return m, model.Init()
	case strings.Contains(title, "Quit"):
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *mainModel) View() string {
	if m.quitting {
		return subtitleStyle.Render("\n  Bye! 👋\n\n")
	}

	if len(m.sessions) > 0 {
		return m.sessions[len(m.sessions)-1].state.View()
	}

	platform := runtime.GOOS
	info := subtitleStyle.Render(fmt.Sprintf("  Platform: %s | ESC to go back | Ctrl+C to quit", platform))
	return "\n" + m.list.View() + "\n" + info + "\n"
}

// --- back message ---

type backMsg struct{}

// --- Helper ---

func formatBytes(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func openFilePicker() (string, error) {
	// Try zenity (Linux), osascript (macOS), or powershell (Windows)
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("osascript", "-e",
			`POSIX path of (choose file with prompt "Select disc image" of type {"iso", "bin", "cdi", "cue", "nrg", "img", "mds", "ccd"})`).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		out, err := exec.Command("zenity", "--file-selection",
			"--title=Select disc image",
			"--file-filter=Disc Images | *.iso *.bin *.cdi *.cue *.nrg *.img *.mds *.ccd",
			"--file-filter=All files | *").Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "windows":
		// PowerShell file dialog
		script := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.OpenFileDialog; $f.Filter = 'Disc Images|*.iso;*.bin;*.cdi;*.cue;*.nrg;*.img;*.mds;*.ccd|All Files|*.*'; if ($f.ShowDialog() -eq 'OK') { $f.FileName }`
		out, err := exec.Command("powershell", "-Command", script).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("file picker not supported on %s", runtime.GOOS)
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// --- Analyze Screen ---

type analyzeModel struct {
	input    textinput.Model
	result   *img.ImageInfo
	loading  bool
	err      error
	width    int
	height   int
	spinner  spinner.Model
	viewMode int // 0 = input, 1 = result
}

func newAnalyzeModel(w, h int) *analyzeModel {
	ti := textinput.New()
	ti.Placeholder = "Path to disc image (ISO, CDI, CUE, NRG...)"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4"))

	return &analyzeModel{
		input:   ti,
		width:   w,
		height:  h,
		spinner: s,
	}
}

func (m *analyzeModel) Init() tea.Cmd { return textinput.Blink }

type analyzeResultMsg struct {
	info *img.ImageInfo
	err  error
}

func (m *analyzeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return backMsg{} }
		case "enter":
			if m.viewMode == 1 {
				m.viewMode = 0
				m.result = nil
				return m, textinput.Blink
			}
			path := expandPath(m.input.Value())
			if path == "" {
				m.err = fmt.Errorf("please enter a file path")
				return m, nil
			}
			m.loading = true
			m.err = nil
			return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
				info, err := img.Analyze(path)
				return analyzeResultMsg{info: info, err: err}
			})
		case "o":
			if m.viewMode == 0 {
				path, err := openFilePicker()
				if err == nil && path != "" {
					m.input.SetValue(path)
				}
				return m, nil
			}
		}
	case analyzeResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.result = msg.info
		m.viewMode = 1
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *analyzeModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 📂 Analyze Image "))
	b.WriteString("\n\n")

	if m.viewMode == 1 && m.result != nil {
		info := m.result
		b.WriteString(highlightStyle.Render(fmt.Sprintf("Format: %s", info.Format)))
		b.WriteString("\n")
		if info.Title != "" {
			b.WriteString(fmt.Sprintf("Title: %s\n", info.Title))
		}
		b.WriteString(fmt.Sprintf("Platform: %s\n", info.Platform))
		b.WriteString(fmt.Sprintf("Size: %s\n", formatBytes(info.Size)))
		b.WriteString(fmt.Sprintf("Sessions: %d\n", info.Sessions))

		if len(info.Tracks) > 0 {
			b.WriteString("\n")
			b.WriteString(highlightStyle.Render("Tracks:"))
			b.WriteString("\n")
			for _, t := range info.Tracks {
				b.WriteString(fmt.Sprintf("  %2d. %-10s  Start: %8d  Sectors: %8d  Size: %s\n",
					t.Number, t.Type, t.StartLBA, t.Sectors, formatBytes(t.SizeBytes)))
			}
		}

		if len(info.RawDetails) > 0 {
			b.WriteString("\n")
			b.WriteString(subtitleStyle.Render("Details:"))
			b.WriteString("\n")
			for k, v := range info.RawDetails {
				b.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
			}
		}

		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Press Enter to analyze another, ESC to go back"))
	} else {
		b.WriteString("Enter path to disc image file:\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("Press [o] to open file picker, Enter to analyze, ESC to go back"))

		if m.loading {
			b.WriteString("\n\n")
			b.WriteString(m.spinner.View() + " Analyzing...")
		}
		if m.err != nil {
			b.WriteString("\n\n")
			b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
		}
	}

	return boxStyle.Render(b.String())
}

// --- Burn Screen ---

type burnModel struct {
	input      textinput.Model
	speedInput textinput.Model
	phase      int // 0 = path input, 1 = options, 2 = burning
	imageInfo  *img.ImageInfo
	loading    bool
	result     string
	err        error
	width      int
	height     int
	spinner    spinner.Model
	dryRun     bool
	verify     bool
	eject      bool
}

func newBurnModel(w, h int) *burnModel {
	ti := textinput.New()
	ti.Placeholder = "Path to disc image to burn"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 50

	si := textinput.New()
	si.Placeholder = "Speed (0=max, 4, 8, 16, 24, 48)"
	si.CharLimit = 4
	si.Width = 20

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))

	return &burnModel{
		input:      ti,
		speedInput: si,
		width:      w,
		height:     h,
		spinner:    s,
		eject:      true,
	}
}

func (m *burnModel) Init() tea.Cmd { return textinput.Blink }

type burnResultMsg struct {
	err error
}

func (m *burnModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.phase > 0 {
				m.phase--
				return m, nil
			}
			return m, func() tea.Msg { return backMsg{} }
		case "enter":
			return m.handleBurnEnter()
		case "o":
			if m.phase == 0 {
				path, err := openFilePicker()
				if err == nil && path != "" {
					m.input.SetValue(path)
				}
				return m, nil
			}
		case "d":
			if m.phase == 1 {
				m.dryRun = !m.dryRun
			}
		case "v":
			if m.phase == 1 {
				m.verify = !m.verify
			}
		case "e":
			if m.phase == 1 {
				m.eject = !m.eject
			}
		}
	case burnResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.result = ""
		} else {
			m.result = "Burn completed successfully!"
			m.phase = 2
		}
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	if m.phase == 0 {
		m.input, cmd = m.input.Update(msg)
	} else if m.phase == 1 {
		m.speedInput, cmd = m.speedInput.Update(msg)
	}
	return m, cmd
}

func (m *burnModel) handleBurnEnter() (tea.Model, tea.Cmd) {
	if m.phase == 0 {
		path := expandPath(m.input.Value())
		if path == "" {
			m.err = fmt.Errorf("please enter a file path")
			return m, nil
		}
		if !fileExists(path) {
			m.err = fmt.Errorf("file not found: %s", path)
			return m, nil
		}

		info, err := img.Analyze(path)
		if err != nil {
			m.err = fmt.Errorf("failed to analyze image: %w", err)
			return m, nil
		}

		m.imageInfo = info
		m.phase = 1
		m.err = nil
		m.speedInput.Focus()
		return m, textinput.Blink
	}

	if m.phase == 1 {
		m.phase = 2
		m.loading = true
		m.err = nil

		speed := 0
		if m.speedInput.Value() != "" {
			fmt.Sscanf(m.speedInput.Value(), "%d", &speed)
		}

		opts := burnerOpts{
			path:       m.input.Value(),
			speed:      speed,
			dryRun:     m.dryRun,
			verify:     m.verify,
			ejectAfter: m.eject,
		}

		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			return doBurn(opts)
		})
	}

	// phase 2 done
	m.phase = 0
	m.result = ""
	m.imageInfo = nil
	return m, textinput.Blink
}

type burnerOpts struct {
	path       string
	speed      int
	dryRun     bool
	verify     bool
	ejectAfter bool
}

func doBurn(opts burnerOpts) burnResultMsg {
	return burnResultMsg{err: fmt.Errorf("burning not yet connected to backend — coming soon")}
}

func (m *burnModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 🔥 Burn Image "))
	b.WriteString("\n\n")

	switch m.phase {
	case 0:
		b.WriteString("Enter path to disc image:\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[o] File picker  [Enter] Continue  [ESC] Back"))
		if m.err != nil {
			b.WriteString("\n\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
		}

	case 1:
		if m.imageInfo != nil {
			b.WriteString(highlightStyle.Render(fmt.Sprintf("Image: %s (%s)", filepath.Base(m.imageInfo.Path), m.imageInfo.Format)))
			b.WriteString("\n")
			if m.imageInfo.Title != "" {
				b.WriteString(fmt.Sprintf("Title: %s | Platform: %s | Size: %s\n",
					m.imageInfo.Title, m.imageInfo.Platform, formatBytes(m.imageInfo.Size)))
			}
			b.WriteString("\n")

			b.WriteString("Burn speed (press Enter for max):\n")
			b.WriteString(m.speedInput.View())
			b.WriteString("\n\n")

			check := func(v bool) string {
				if v {
					return successStyle.Render("✓")
				}
				return "○"
			}

			b.WriteString(fmt.Sprintf("  %s [d] Dry run (simulate)\n", check(m.dryRun)))
			b.WriteString(fmt.Sprintf("  %s [v] Verify after burn\n", check(m.verify)))
			b.WriteString(fmt.Sprintf("  %s [e] Eject after burn\n", check(m.eject)))
			b.WriteString("\n")
			b.WriteString(subtitleStyle.Render("[Enter] Start burn  [ESC] Back"))
		}

	case 2:
		if m.loading {
			b.WriteString(m.spinner.View() + " Burning disc...")
			b.WriteString("\n\n")
			b.WriteString(subtitleStyle.Render("Please wait..."))
		} else if m.result != "" {
			b.WriteString(successStyle.Render("✓ " + m.result))
			b.WriteString("\n\n")
			b.WriteString(subtitleStyle.Render("[Enter] Burn another  [ESC] Back"))
		}
		if m.err != nil {
			b.WriteString("\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
			b.WriteString("\n\n" + subtitleStyle.Render("[Enter] Try again  [ESC] Back"))
		}
	}

	return boxStyle.Render(b.String())
}

// --- Drive Status Screen ---

type driveStatusModel struct {
	loaded   bool
	drives   []string
	spinner  spinner.Model
	err      error
}

func newDriveStatusModel() *driveStatusModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))
	return &driveStatusModel{spinner: s}
}

func (m *driveStatusModel) Init() tea.Cmd {
	return func() tea.Msg {
		return driveStatusLoadedMsg{}
	}
}

type driveStatusLoadedMsg struct{}

func (m *driveStatusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			return m, func() tea.Msg { return backMsg{} }
		}
		if msg.String() == "r" {
			m.loaded = false
			return m, func() tea.Msg { return driveStatusLoadedMsg{} }
		}
	case driveStatusLoadedMsg:
		m.loaded = true
		// Try to get drive info
		switch runtime.GOOS {
		case "darwin":
			out, err := exec.Command("drutil", "status").CombinedOutput()
			if err != nil {
				m.err = err
				return m, nil
			}
			m.drives = strings.Split(string(out), "\n")
		case "linux":
			out, err := exec.Command("xorriso", "-devices").CombinedOutput()
			if err != nil {
				m.err = fmt.Errorf("xorriso not found or no drives: %w", err)
				return m, nil
			}
			m.drives = strings.Split(string(out), "\n")
		default:
			m.drives = []string{"Drive detection not yet implemented for " + runtime.GOOS}
		}
		return m, nil
	case spinner.TickMsg:
		if !m.loaded {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *driveStatusModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 💿 Drive Status "))
	b.WriteString("\n\n")

	if !m.loaded {
		b.WriteString(m.spinner.View() + " Detecting drives...")
	} else if m.err != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
	} else {
		for _, line := range m.drives {
			if strings.TrimSpace(line) != "" {
				b.WriteString(line + "\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("[r] Refresh  [ESC] Back"))

	return boxStyle.Render(b.String())
}

// --- Convert Screen ---

type convertModel struct {
	input    textinput.Model
	output   textinput.Model
	phase    int // 0 = input, 1 = output, 2 = converting, 3 = done
	format   string
	loading  bool
	spinner  spinner.Model
	err      error
	result   string
	width    int
	height   int
}

func newConvertModel(w, h int) *convertModel {
	ti := textinput.New()
	ti.Placeholder = "Path to source image"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 50

	oi := textinput.New()
	oi.Placeholder = "Output path (optional)"
	oi.CharLimit = 500
	oi.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))

	return &convertModel{
		input:   ti,
		output:  oi,
		width:   w,
		height:  h,
		spinner: s,
	}
}

func (m *convertModel) Init() tea.Cmd { return textinput.Blink }

type convertResultMsg struct {
	cuePath string
	binPath string
	err     error
}

func (m *convertModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.phase > 0 && m.phase < 3 {
				m.phase--
				return m, nil
			}
			return m, func() tea.Msg { return backMsg{} }
		case "enter":
			return m.handleConvertEnter()
		case "o":
			if m.phase == 0 {
				path, err := openFilePicker()
				if err == nil && path != "" {
					m.input.SetValue(path)
				}
				return m, nil
			}
		}
	case convertResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.result = fmt.Sprintf("Converted!\n  CUE: %s\n  BIN: %s", msg.cuePath, msg.binPath)
			m.phase = 3
		}
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	if m.phase <= 1 {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m *convertModel) handleConvertEnter() (tea.Model, tea.Cmd) {
	if m.phase == 0 {
		path := expandPath(m.input.Value())
		if path == "" {
			m.err = fmt.Errorf("please enter a source file path")
			return m, nil
		}
		if !fileExists(path) {
			m.err = fmt.Errorf("file not found: %s", path)
			return m, nil
		}

		format, err := img.DetectFormat(path)
		if err != nil {
			m.err = err
			return m, nil
		}
		m.format = string(format)
		m.phase = 1
		m.err = nil
		return m, textinput.Blink
	}

	if m.phase == 1 {
		m.phase = 2
		m.loading = true

		srcPath := expandPath(m.input.Value())
		outPath := expandPath(m.output.Value())

		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			switch m.format {
			case "CDI":
				cue, bin, err := convert.CDIToCUE(srcPath, outPath)
				return convertResultMsg{cuePath: cue, binPath: bin, err: err}
			case "ISO":
				cue, bin, err := convert.ISOToCUE(srcPath, outPath)
				return convertResultMsg{cuePath: cue, binPath: bin, err: err}
			default:
				return convertResultMsg{err: fmt.Errorf("conversion from %s not yet supported", m.format)}
			}
		})
	}

	// Done, reset
	m.phase = 0
	m.result = ""
	m.format = ""
	return m, textinput.Blink
}

func (m *convertModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 🔄 Convert Image "))
	b.WriteString("\n\n")

	switch m.phase {
	case 0:
		b.WriteString("Source image file:\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[o] File picker  [Enter] Continue  [ESC] Back"))
	case 1:
		b.WriteString(highlightStyle.Render(fmt.Sprintf("Converting: %s (%s)", filepath.Base(m.input.Value()), m.format)))
		b.WriteString("\n\nOutput path (leave empty for same directory):\n\n")
		b.WriteString(m.output.View())
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[Enter] Start conversion  [ESC] Back"))
	case 2:
		b.WriteString(m.spinner.View() + " Converting...")
	case 3:
		b.WriteString(successStyle.Render(m.result))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[Enter] Convert another  [ESC] Back"))
	}

	if m.err != nil {
		b.WriteString("\n\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
	}

	return boxStyle.Render(b.String())
}

// --- Extract Screen ---

type extractModel struct {
	input    textinput.Model
	destInput textinput.Model
	phase    int
	loading  bool
	spinner  spinner.Model
	err      error
	result   string
	width    int
	height   int
}

func newExtractModel(w, h int) *extractModel {
	ti := textinput.New()
	ti.Placeholder = "Path to ISO image"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 50

	di := textinput.New()
	di.Placeholder = "Destination directory"
	di.CharLimit = 500
	di.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))

	return &extractModel{
		input:     ti,
		destInput: di,
		width:     w,
		height:    h,
		spinner:   s,
	}
}

func (m *extractModel) Init() tea.Cmd { return textinput.Blink }

type extractResultMsg struct {
	err error
}

func (m *extractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.phase > 0 && m.phase < 3 {
				m.phase--
				return m, nil
			}
			return m, func() tea.Msg { return backMsg{} }
		case "enter":
			return m.handleExtractEnter()
		case "o":
			if m.phase == 0 {
				path, err := openFilePicker()
				if err == nil && path != "" {
					m.input.SetValue(path)
				}
				return m, nil
			}
		}
	case extractResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.result = "Files extracted successfully!"
			m.phase = 3
		}
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *extractModel) handleExtractEnter() (tea.Model, tea.Cmd) {
	if m.phase == 0 {
		path := expandPath(m.input.Value())
		if path == "" {
			m.err = fmt.Errorf("please enter an ISO file path")
			return m, nil
		}
		m.phase = 1
		m.err = nil
		return m, textinput.Blink
	}

	if m.phase == 1 {
		m.phase = 2
		m.loading = true
		srcPath := expandPath(m.input.Value())
		destDir := expandPath(m.destInput.Value())
		if destDir == "" {
			destDir = strings.TrimSuffix(srcPath, filepath.Ext(srcPath)) + "_extracted"
		}

		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			err := convert.ExtractFilesFromISO(srcPath, destDir)
			return extractResultMsg{err: err}
		})
	}

	m.phase = 0
	m.result = ""
	return m, textinput.Blink
}

func (m *extractModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 📁 Extract Files "))
	b.WriteString("\n\n")

	switch m.phase {
	case 0:
		b.WriteString("ISO image path:\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[o] File picker  [Enter] Continue  [ESC] Back"))
	case 1:
		b.WriteString(highlightStyle.Render(fmt.Sprintf("Extracting: %s", filepath.Base(m.input.Value()))))
		b.WriteString("\n\nDestination directory (leave empty for auto):\n\n")
		b.WriteString(m.destInput.View())
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[Enter] Extract  [ESC] Back"))
	case 2:
		b.WriteString(m.spinner.View() + " Extracting files...")
	case 3:
		b.WriteString(successStyle.Render("✓ " + m.result))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[Enter] Extract another  [ESC] Back"))
	}

	if m.err != nil {
		b.WriteString("\n\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
	}

	return boxStyle.Render(b.String())
}

// --- Erase Screen ---

type eraseModel struct {
	loading bool
	spinner spinner.Model
	err     error
	result  string
	quick   bool
	phase   int
}

func newEraseModel() *eraseModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B"))
	return &eraseModel{spinner: s, quick: true}
}

func (m *eraseModel) Init() tea.Cmd { return nil }

type eraseResultMsg struct {
	err error
}

func (m *eraseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, func() tea.Msg { return backMsg{} }
		case "q":
			m.quick = true
		case "f":
			m.quick = false
		case "enter":
			if m.phase == 0 {
				m.phase = 1
				m.loading = true
				return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
					return eraseResultMsg{err: fmt.Errorf("erase not yet connected to backend")}
				})
			}
			m.phase = 0
			m.result = ""
		}
	case eraseResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.result = "Disc erased successfully!"
		}
		return m, nil
	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *eraseModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 🧹 Erase Disc "))
	b.WriteString("\n\n")

	if m.phase == 0 {
		b.WriteString("Select erase mode:\n\n")
		check := func(v bool) string {
			if v {
				return highlightStyle.Render("►")
			}
			return " "
		}
		b.WriteString(fmt.Sprintf("  %s [q] Quick erase (fast)\n", check(m.quick)))
		b.WriteString(fmt.Sprintf("  %s [f] Full erase (thorough)\n", check(!m.quick)))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("[Enter] Start erase  [ESC] Back"))
	} else if m.loading {
		b.WriteString(m.spinner.View() + " Erasing disc...")
	} else if m.result != "" {
		b.WriteString(successStyle.Render("✓ " + m.result))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[Enter] Done  [ESC] Back"))
	}

	if m.err != nil {
		b.WriteString("\n\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
		b.WriteString("\n\n" + subtitleStyle.Render("[Enter] Try again  [ESC] Back"))
	}

	return boxStyle.Render(b.String())
}
