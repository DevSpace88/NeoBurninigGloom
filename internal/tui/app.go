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
	"NeoBurningGoom/internal/drive"
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
		menuItem{title: "📀 Dump Disc to Image", desc: "Read a CD/DVD and save as ISO or BIN/CUE image"},
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
	case strings.Contains(title, "Dump Disc"):
		model := newDumpModel(m.width, m.height)
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

func findDarwinDevicePath() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	// Use drutil to find the optical drive's BSD device path
	out, err := exec.Command("drutil", "status").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "/dev/disk") || strings.Contains(line, "/dev/rdisk") {
			// Extract /dev/diskN
			for _, part := range strings.Fields(line) {
				if strings.HasPrefix(part, "/dev/disk") {
					return part
				}
				if strings.HasPrefix(part, "/dev/rdisk") {
					return strings.Replace(part, "rdisk", "disk", 1)
				}
			}
		}
	}
	// Fallback: try diskutil list external
	out2, err := exec.Command("diskutil", "list", "external").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out2), "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "/dev/disk"); idx >= 0 {
			dev := line[idx:]
			for i := len("/dev/disk"); i < len(dev); i++ {
				if dev[i] < '0' || dev[i] > '9' {
					dev = dev[:i]
					break
				}
			}
			if dev != "/dev/disk0" {
				// Verify it's optical
				infoOut, err := exec.Command("diskutil", "info", dev).CombinedOutput()
				if err == nil && (strings.Contains(string(infoOut), "Optical") || strings.Contains(string(infoOut), "CD") || strings.Contains(string(infoOut), "DVD")) {
					return dev
				}
			}
		}
	}
	return ""
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

func saveFilePicker(defaultName string) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("osascript", "-e",
			fmt.Sprintf(`POSIX path of (choose file name with prompt "Save as" default name "%s")`, defaultName)).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "linux":
		out, err := exec.Command("zenity", "--file-selection",
			"--save", "--confirm-overwrite",
			"--title=Save as",
			"--filename="+defaultName).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	case "windows":
		script := fmt.Sprintf(
			`Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.SaveFileDialog; $f.FileName = '%s'; $f.Filter = 'All Files|*.*'; if ($f.ShowDialog() -eq 'OK') { $f.FileName }`,
			defaultName)
		out, err := exec.Command("powershell", "-Command", script).Output()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	return "", fmt.Errorf("save dialog not supported on %s", runtime.GOOS)
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
	input       textinput.Model
	result      *img.ImageInfo
	loading     bool
	err         error
	width       int
	height      int
	spinner     spinner.Model
	viewMode    int // 0 = input, 1 = result
	skipNextKey bool
	typingMode  bool
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
		input:      ti,
		width:      w,
		height:     h,
		spinner:    s,
		typingMode: true,
	}
}

func (m *analyzeModel) Init() tea.Cmd { return textinput.Blink }

type analyzeResultMsg struct {
	info *img.ImageInfo
	err  error
}

func (m *analyzeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.skipNextKey {
		m.skipNextKey = false
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.typingMode {
			switch msg.String() {
			case "tab":
				m.typingMode = false
				m.input.Blur()
				return m, nil
			case "esc":
				return m, func() tea.Msg { return backMsg{} }
			case "enter":
				if m.viewMode == 1 {
					m.viewMode = 0
					m.result = nil
					m.input.Focus()
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
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		// Command mode
		switch msg.String() {
		case "tab":
			if m.viewMode == 0 {
				m.typingMode = true
				m.input.Focus()
				return m, textinput.Blink
			}
		case "esc":
			return m, func() tea.Msg { return backMsg{} }
		case "enter":
			if m.viewMode == 1 {
				m.viewMode = 0
				m.result = nil
				m.input.Focus()
				m.typingMode = true
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
					m.skipNextKey = true
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
	return m, nil
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
		modeHint := "[Tab] Type"
		if m.typingMode {
			modeHint = "[Tab] Commands"
		}
		b.WriteString("Enter path to disc image file:\n\n")
		b.WriteString(m.input.View())
		b.WriteString("\n\n")
		if m.typingMode {
			b.WriteString(subtitleStyle.Render(modeHint + "  [Enter] Analyze  [ESC] Back"))
		} else {
			b.WriteString(subtitleStyle.Render(modeHint + "  [o] Browse  [Enter] Analyze  [ESC] Back"))
		}

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
	input       textinput.Model
	speedInput  textinput.Model
	phase       int // 0 = path input, 1 = options, 2 = burning
	imageInfo   *img.ImageInfo
	loading     bool
	result      string
	err         error
	width       int
	height      int
	spinner     spinner.Model
	dryRun      bool
	verify      bool
	eject       bool
	skipNextKey bool
	typingMode  bool
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
		typingMode: true,
	}
}

func (m *burnModel) Init() tea.Cmd { return textinput.Blink }

type burnResultMsg struct {
	err error
}

func (m *burnModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.skipNextKey {
		m.skipNextKey = false
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.typingMode {
			switch msg.String() {
			case "tab":
				m.typingMode = false
				if m.phase == 0 {
					m.input.Blur()
				} else if m.phase == 1 {
					m.speedInput.Blur()
				}
				return m, nil
			case "esc":
				if m.phase > 0 {
					m.phase--
					if m.phase == 0 {
						m.input.Focus()
					}
					return m, textinput.Blink
				}
				return m, func() tea.Msg { return backMsg{} }
			case "enter":
				return m.handleBurnEnter()
			}
			var cmd tea.Cmd
			if m.phase == 0 {
				m.input, cmd = m.input.Update(msg)
			} else if m.phase == 1 {
				m.speedInput, cmd = m.speedInput.Update(msg)
			}
			return m, cmd
		}
		// Command mode
		switch msg.String() {
		case "tab":
			m.typingMode = true
			if m.phase == 0 {
				m.input.Focus()
				return m, textinput.Blink
			} else if m.phase == 1 {
				m.speedInput.Focus()
				return m, textinput.Blink
			}
			return m, nil
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
					m.skipNextKey = true
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
	return m, nil
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
		m.typingMode = true
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
	m.typingMode = true
	m.input.Focus()
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
		if m.typingMode {
			b.WriteString(subtitleStyle.Render("[Tab] Commands  [Enter] Continue  [ESC] Back"))
		} else {
			b.WriteString(subtitleStyle.Render("[Tab] Type  [o] Browse  [Enter] Continue  [ESC] Back"))
		}
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
			if m.typingMode {
				b.WriteString(subtitleStyle.Render("[Tab] Commands  [Enter] Start burn  [ESC] Back"))
			} else {
				b.WriteString(subtitleStyle.Render("[Tab] Type  [d] Dry run  [v] Verify  [e] Eject  [Enter] Start  [ESC] Back"))
			}
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
	input       textinput.Model
	output      textinput.Model
	phase       int // 0 = source, 1 = target format, 2 = output path, 3 = converting, 4 = done
	srcFormat   img.Format
	targets     []string // available target formats
	cursor      int      // selected target format
	loading     bool
	spinner     spinner.Model
	err         error
	result      string
	outputFiles []string
	width       int
	height      int
	skipNextKey bool
	typingMode  bool
}

func newConvertModel(w, h int) *convertModel {
	ti := textinput.New()
	ti.Placeholder = "Path to source image (ISO, CDI, CUE, NRG...)"
	ti.Focus()
	ti.CharLimit = 500
	ti.Width = 50

	oi := textinput.New()
	oi.Placeholder = "Output directory (leave empty = same as source)"
	oi.CharLimit = 500
	oi.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))

	return &convertModel{
		input:      ti,
		output:     oi,
		width:      w,
		height:     h,
		spinner:    s,
		typingMode: true,
	}
}

func (m *convertModel) Init() tea.Cmd { return textinput.Blink }

type convertResultMsg struct {
	outputFiles []string
	err         error
}

func (m *convertModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.skipNextKey {
		m.skipNextKey = false
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.typingMode {
			switch msg.String() {
			case "tab":
				m.typingMode = false
				if m.phase == 0 {
					m.input.Blur()
				} else if m.phase == 2 {
					m.output.Blur()
				}
				return m, nil
			case "esc":
				if m.phase > 0 && m.phase < 4 {
					m.phase--
					if m.phase == 0 {
						m.input.Focus()
						m.typingMode = true
					}
					return m, textinput.Blink
				}
				return m, func() tea.Msg { return backMsg{} }
			case "enter":
				return m.handleConvertEnter()
			}
			var cmd tea.Cmd
			if m.phase == 0 {
				m.input, cmd = m.input.Update(msg)
			} else if m.phase == 2 {
				m.output, cmd = m.output.Update(msg)
			}
			return m, cmd
		}
		// Command mode
		switch msg.String() {
		case "tab":
			if m.phase == 0 {
				m.typingMode = true
				m.input.Focus()
				return m, textinput.Blink
			} else if m.phase == 2 {
				m.typingMode = true
				m.output.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case "esc":
			if m.phase > 0 && m.phase < 4 {
				m.phase--
				if m.phase == 0 {
					m.input.Focus()
					m.typingMode = true
				}
				return m, textinput.Blink
			}
			return m, func() tea.Msg { return backMsg{} }
		case "up", "k":
			if m.phase == 1 && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.phase == 1 && m.cursor < len(m.targets)-1 {
				m.cursor++
			}
		case "o":
			if m.phase == 0 {
				path, err := openFilePicker()
				if err == nil && path != "" {
					m.input.SetValue(path)
					m.skipNextKey = true
				}
				return m, nil
			}
			if m.phase == 2 {
				path, err := openFilePicker()
				if err == nil && path != "" {
					m.output.SetValue(path)
					m.skipNextKey = true
				}
				return m, nil
			}
		case "enter":
			return m.handleConvertEnter()
		}
	case convertResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.outputFiles = msg.outputFiles
			var sb strings.Builder
			sb.WriteString("Conversion complete!\n")
			for _, f := range msg.outputFiles {
				sb.WriteString(fmt.Sprintf("  → %s\n", f))
			}
			m.result = sb.String()
			m.phase = 4
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
		m.srcFormat = format

		targets := convert.SupportedConversions(format)
		if len(targets) == 0 {
			m.err = fmt.Errorf("no conversions available for %s format", format)
			return m, nil
		}
		m.targets = targets
		m.cursor = 0
		m.phase = 1
		m.err = nil
		return m, nil
	}

	if m.phase == 1 {
		// Set default output path to source directory
		srcPath := expandPath(m.input.Value())
		srcDir := filepath.Dir(srcPath)
		baseName := strings.TrimSuffix(filepath.Base(srcPath), filepath.Ext(srcPath))
		target := m.targets[m.cursor]

		defaultOutput := ""
		switch target {
		case "CUE/BIN":
			defaultOutput = filepath.Join(srcDir, baseName+".bin")
		case "ISO":
			defaultOutput = filepath.Join(srcDir, baseName+"_converted.iso")
		}

		m.output.SetValue(defaultOutput)
		m.output.Focus()
		m.phase = 2
		m.err = nil
		m.typingMode = true
		return m, textinput.Blink
	}

	if m.phase == 2 {
		m.phase = 3
		m.loading = true

		srcPath := expandPath(m.input.Value())
		outPath := expandPath(m.output.Value())
		target := m.targets[m.cursor]

		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			files, err := convert.Convert(srcPath, outPath, target)
			return convertResultMsg{outputFiles: files, err: err}
		})
	}

	// Done, reset
	m.phase = 0
	m.result = ""
	m.outputFiles = nil
	m.srcFormat = ""
	m.targets = nil
	m.cursor = 0
	m.input.SetValue("")
	m.output.SetValue("")
	m.input.Focus()
	m.typingMode = true
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
		if m.typingMode {
			b.WriteString(subtitleStyle.Render("[Tab] Commands  [Enter] Continue  [ESC] Back"))
		} else {
			b.WriteString(subtitleStyle.Render("[Tab] Type  [o] Browse  [Enter] Continue  [ESC] Back"))
		}

	case 1:
		b.WriteString(highlightStyle.Render(fmt.Sprintf("Source: %s (%s)", filepath.Base(expandPath(m.input.Value())), m.srcFormat)))
		b.WriteString("\n\nConvert to:\n\n")
		for i, t := range m.targets {
			if i == m.cursor {
				b.WriteString(highlightStyle.Render(fmt.Sprintf("  ► %s", t)))
			} else {
				b.WriteString(fmt.Sprintf("    %s", t))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("[↑/↓] Navigate  [Enter] Continue  [ESC] Back"))

	case 2:
		b.WriteString(highlightStyle.Render(fmt.Sprintf("%s (%s) → %s", filepath.Base(expandPath(m.input.Value())), m.srcFormat, m.targets[m.cursor])))
		b.WriteString("\n\nOutput path:\n\n")
		b.WriteString(m.output.View())
		b.WriteString("\n\n")
		if m.typingMode {
			b.WriteString(subtitleStyle.Render("[Tab] Commands  [Enter] Start conversion  [ESC] Back"))
		} else {
			b.WriteString(subtitleStyle.Render("[Tab] Type  [o] Browse  [Enter] Start conversion  [ESC] Back"))
		}

	case 3:
		b.WriteString(m.spinner.View() + " Converting...")
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("Converting %s → %s", m.srcFormat, m.targets[m.cursor])))

	case 4:
		b.WriteString(successStyle.Render(m.result))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("[Enter] Convert another  [ESC] Back"))
	}

	if m.err != nil {
		b.WriteString("\n\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
	}

	return boxStyle.Render(b.String())
}

// --- Extract Screen ---

type extractModel struct {
	input       textinput.Model
	destInput   textinput.Model
	phase       int
	loading     bool
	spinner     spinner.Model
	err         error
	result      string
	width       int
	height      int
	skipNextKey bool
	typingMode  bool
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
		input:      ti,
		destInput:  di,
		width:      w,
		height:     h,
		spinner:    s,
		typingMode: true,
	}
}

func (m *extractModel) Init() tea.Cmd { return textinput.Blink }

type extractResultMsg struct {
	err error
}

func (m *extractModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.skipNextKey {
		m.skipNextKey = false
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.typingMode {
			switch msg.String() {
			case "tab":
				m.typingMode = false
				if m.phase == 0 {
					m.input.Blur()
				} else if m.phase == 1 {
					m.destInput.Blur()
				}
				return m, nil
			case "esc":
				if m.phase > 0 && m.phase < 3 {
					m.phase--
					if m.phase == 0 {
						m.input.Focus()
					}
					return m, textinput.Blink
				}
				return m, func() tea.Msg { return backMsg{} }
			case "enter":
				return m.handleExtractEnter()
			}
			var cmd tea.Cmd
			if m.phase == 0 {
				m.input, cmd = m.input.Update(msg)
			} else if m.phase == 1 {
				m.destInput, cmd = m.destInput.Update(msg)
			}
			return m, cmd
		}
		// Command mode
		switch msg.String() {
		case "tab":
			m.typingMode = true
			if m.phase == 0 {
				m.input.Focus()
			} else if m.phase == 1 {
				m.destInput.Focus()
			}
			return m, textinput.Blink
		case "esc":
			if m.phase > 0 && m.phase < 3 {
				m.phase--
				if m.phase == 0 {
					m.input.Focus()
					m.typingMode = true
				}
				return m, textinput.Blink
			}
			return m, func() tea.Msg { return backMsg{} }
		case "o":
			if m.phase == 0 {
				path, err := openFilePicker()
				if err == nil && path != "" {
					m.input.SetValue(path)
					m.skipNextKey = true
				}
				return m, nil
			}
		case "enter":
			return m.handleExtractEnter()
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
	return m, nil
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
		m.destInput.Focus()
		m.typingMode = true
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
	m.input.Focus()
	m.typingMode = true
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
		if m.typingMode {
			b.WriteString(subtitleStyle.Render("[Tab] Commands  [Enter] Continue  [ESC] Back"))
		} else {
			b.WriteString(subtitleStyle.Render("[Tab] Type  [o] Browse  [Enter] Continue  [ESC] Back"))
		}
	case 1:
		b.WriteString(highlightStyle.Render(fmt.Sprintf("Extracting: %s", filepath.Base(m.input.Value()))))
		b.WriteString("\n\nDestination directory (leave empty for auto):\n\n")
		b.WriteString(m.destInput.View())
		b.WriteString("\n\n")
		if m.typingMode {
			b.WriteString(subtitleStyle.Render("[Tab] Commands  [Enter] Extract  [ESC] Back"))
		} else {
			b.WriteString(subtitleStyle.Render("[Tab] Type  [Enter] Extract  [ESC] Back"))
		}
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

// --- Dump Disc Screen ---

type dumpModel struct {
	output      textinput.Model
	phase       int // 0 = select format, 1 = output path, 2 = dumping, 3 = done
	cursor      int // 0 = iso, 1 = bin/cue
	format      string // "iso", "bin/cue"
	loading     bool
	spinner     spinner.Model
	err         error
	result      string
	width       int
	height      int
	driveIdx    int
	devicePath  string // actual /dev/diskN
	skipNextKey bool
	typingMode  bool
}

var dumpFormats = []struct {
	key     string
	label   string
	desc    string
}{
	{"iso", "ISO", "standard disc image"},
	{"bin/cue", "BIN/CUE", "raw dump with track info (Audio CDs, Dreamcast)"},
}

func newDumpModel(w, h int) *dumpModel {
	ti := textinput.New()
	ti.Placeholder = "Output file path (e.g. ~/disc.iso)"
	ti.CharLimit = 500
	ti.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575"))

	return &dumpModel{
		output:     ti,
		width:      w,
		height:     h,
		spinner:    s,
		cursor:     0,
		format:     "iso",
		driveIdx:   0,
		devicePath: findDarwinDevicePath(),
	}
}

func (m *dumpModel) Init() tea.Cmd { return nil }

type dumpResultMsg struct {
	err error
}

func (m *dumpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.skipNextKey {
		m.skipNextKey = false
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.typingMode {
			switch msg.String() {
			case "tab":
				m.typingMode = false
				m.output.Blur()
				return m, nil
			case "esc":
				if m.phase > 0 && m.phase < 3 {
					m.phase--
					return m, nil
				}
				return m, func() tea.Msg { return backMsg{} }
			case "enter":
				return m.handleDumpEnter()
			}
			var cmd tea.Cmd
			m.output, cmd = m.output.Update(msg)
			return m, cmd
		}
		// Command mode
		switch msg.String() {
		case "tab":
			if m.phase == 1 {
				m.typingMode = true
				m.output.Focus()
				return m, textinput.Blink
			}
			return m, nil
		case "esc":
			if m.phase > 0 && m.phase < 3 {
				m.phase--
				return m, nil
			}
			return m, func() tea.Msg { return backMsg{} }
		case "up", "k":
			if m.phase == 0 && m.cursor > 0 {
				m.cursor--
				m.format = dumpFormats[m.cursor].key
			}
		case "down", "j":
			if m.phase == 0 && m.cursor < len(dumpFormats)-1 {
				m.cursor++
				m.format = dumpFormats[m.cursor].key
			}
		case "1":
			if m.phase == 0 {
				m.cursor = 0
				m.format = "iso"
			}
		case "2":
			if m.phase == 0 {
				m.cursor = 1
				m.format = "bin/cue"
			}
		case "o":
			if m.phase == 1 {
				defaultName := "disc.iso"
				if m.format == "bin/cue" {
					defaultName = "disc.bin"
				}
				path, err := saveFilePicker(defaultName)
				if err == nil && path != "" {
					m.output.SetValue(path)
					m.skipNextKey = true
				}
				return m, nil
			}
		case "enter":
			return m.handleDumpEnter()
		}
	case dumpResultMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.result = fmt.Sprintf("Disc dumped to %s", m.output.Value())
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
	return m, nil
}

func (m *dumpModel) handleDumpEnter() (tea.Model, tea.Cmd) {
	if m.phase == 0 {
		m.phase = 1
		m.output.Focus()
		m.typingMode = true
		return m, textinput.Blink
	}

	if m.phase == 1 {
		path := expandPath(m.output.Value())
		if path == "" {
			m.err = fmt.Errorf("please enter an output path")
			return m, nil
		}
		m.phase = 2
		m.loading = true
		m.err = nil

		return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
			progress := make(chan drive.DumpProgress)
			go func() {
				for range progress {
				}
			}()
			err := drive.DumpDisc(drive.DumpOptions{
				DriveIndex: m.driveIdx,
				DevicePath: m.devicePath,
				OutputPath: path,
				Format:     m.format,
			}, progress)
			return dumpResultMsg{err: err}
		})
	}

	m.phase = 0
	m.result = ""
	return m, nil
}

func (m *dumpModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" 📀 Dump Disc to Image "))
	b.WriteString("\n\n")

	switch m.phase {
	case 0:
		b.WriteString("Select output format:\n\n")
		for i, f := range dumpFormats {
			if i == m.cursor {
				b.WriteString(highlightStyle.Render(fmt.Sprintf("  ► %s  — %s", f.label, f.desc)))
			} else {
				b.WriteString(fmt.Sprintf("    %s  — %s", f.label, f.desc))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("[↑/↓] Navigate  [Enter] Continue  [ESC] Back"))

	case 1:
		b.WriteString(highlightStyle.Render(fmt.Sprintf("Format: %s", m.format)))
		b.WriteString("\n\nOutput file path:\n\n")
		b.WriteString(m.output.View())
		b.WriteString("\n\n")
		if m.format == "bin/cue" {
			b.WriteString(subtitleStyle.Render("A .toc file will be created alongside the .bin"))
			b.WriteString("\n\n")
		}
		if m.typingMode {
			b.WriteString(subtitleStyle.Render("[Tab] Commands  [Enter] Start dump  [ESC] Back"))
		} else {
			b.WriteString(subtitleStyle.Render("[Tab] Type  [o] Browse  [Enter] Start dump  [ESC] Back"))
		}

	case 2:
		b.WriteString(m.spinner.View() + " Reading disc...")
		b.WriteString("\n\n")
		b.WriteString(fmt.Sprintf("Saving to: %s", expandPath(m.output.Value())))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("This may take a while depending on disc size"))

	case 3:
		b.WriteString(successStyle.Render("✓ " + m.result))
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render("[Enter] Dump another  [ESC] Back"))
	}

	if m.err != nil {
		b.WriteString("\n\n" + errorStyle.Render(fmt.Sprintf("Error: %s", m.err)))
		b.WriteString("\n\n" + subtitleStyle.Render("[Enter] Try again  [ESC] Back"))
	}

	return boxStyle.Render(b.String())
}
