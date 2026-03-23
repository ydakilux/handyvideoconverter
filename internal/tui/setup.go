package tui

// setup.go implements the interactive setup phase that runs inside the same
// Bubble Tea program as the conversion TUI.
//
// Flow:
//   1. (optional) folder picker  — skipped when path already provided via CLI
//   2. Series of yes/no and numeric questions
//   3. msgSetupDone fires → parent model transitions to phaseConverting
//
// All questions whose answers were already supplied via CLI flags are skipped
// automatically; the phase is skipped entirely when nothing is needed.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Startup phase messages ────────────────────────────────────────────────────

// msgStartupLine is sent when a new status line arrives from StartupCh, or
// when the channel closes (done == true).
type msgStartupLine struct {
	text string
	done bool
}

// waitStartupLine returns a Cmd that reads the next item from ch.
func waitStartupLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-ch
		if !ok {
			return msgStartupLine{done: true}
		}
		return msgStartupLine{text: text}
	}
}

// ── Public types ──────────────────────────────────────────────────────────────

// SetupOptions describes which questions the setup phase must ask.
// Fields set to their zero value mean "already known — skip this question".
type SetupOptions struct {
	// StartupCh, when non-nil, drives a "startup" screen shown before any
	// questions.  The app sends progress lines (e.g. "Detecting GPU…"); closing
	// the channel signals that startup work is complete and questions can begin.
	StartupCh <-chan string

	// NeedFolder: true when no path was given on the CLI.
	NeedFolder bool
	// StartDir is the initial directory for the folder picker (empty = home).
	StartDir string
	// VideoExtensions is the list of extensions used to count video files in
	// the confirmation preview (e.g. []string{".mp4", ".mkv"}).
	// Case-insensitive.  Empty = count all files.
	VideoExtensions []string

	// Each "Need*" flag = true means the question must be asked.
	// The corresponding "Default*" value is shown as the pre-filled suggestion.
	NeedBypass       bool
	NeedForceHevc    bool
	NeedParallelJobs bool
	DefaultParallel  int
	NeedOutputDrive  bool
	AvailableDrives  []string // populated only when NeedOutputDrive == true
}

// SetupAnswers carries the values collected during setup.
type SetupAnswers struct {
	Paths        []string // selected folders/files; nil when NeedFolder was false
	Bypass       bool
	ForceHevc    bool
	ParallelJobs int    // 0 when NeedParallelJobs was false
	OutputDrive  string // "" = same drive
	Cancelled    bool   // user pressed Esc/q before completing
}

// msgSetupDone is sent by the setup sub-model when all answers are collected.
type msgSetupDone struct{ answers SetupAnswers }

// ── Setup sub-model ───────────────────────────────────────────────────────────

type setupStep int

const (
	stepStartup setupStep = iota // waiting for startup goroutine to finish
	stepFolder                   // directory picker (multi-select)
	stepConfirm                  // preview: dir/file counts before proceeding
	stepBypass
	stepForceHevc
	stepParallelJobs
	stepOutputDrive
	stepDone
)

// confirmScan holds the pre-scanned overview shown in stepConfirm.
type confirmScan struct {
	totalDirs  int
	totalFiles int
	baseDirs   []string // unique root paths the user added
}

type setupModel struct {
	opts    SetupOptions
	answers SetupAnswers

	step setupStep

	// startup status lines (stepStartup)
	startupLines []string
	startupDone  bool
	startupTick  int // spinner frame counter

	// folder picker (stepFolder) — multi-select
	fp            filepicker.Model
	selectedPaths []string      // ordered list of confirmed selections
	fpLastDir     string        // last CurrentDirectory shown in the hint bar
	fpFiles       []os.DirEntry // mirror of fp's private files slice
	fpCursor      int           // mirror of fp's private selected index

	// drive-switcher overlay (shown inside stepFolder via Tab)
	fpDriveOverlay bool
	fpDriveCursor  int

	// stepConfirm
	scan confirmScan

	// text input (stepParallelJobs)
	ti textinput.Model

	// drive selection cursor (stepOutputDrive)
	driveCursor int
	// whether user said yes to "different drive?" before showing drive list
	driveYesAsked  bool
	driveWantsDiff bool

	width  int
	height int
}

func newSetupModel(opts SetupOptions) setupModel {
	// Build text input for parallel jobs.
	ti := textinput.New()
	ti.CharLimit = 2
	ti.Width = 4
	if opts.DefaultParallel > 0 {
		ti.Placeholder = strconv.Itoa(opts.DefaultParallel)
	} else {
		ti.Placeholder = "1"
	}

	// Build filepicker.
	startDir := opts.StartDir
	if startDir == "" {
		// Prefer USERPROFILE on Windows; fall back to os.UserHomeDir().
		if h := os.Getenv("USERPROFILE"); h != "" {
			startDir = h
		} else if h, err := os.UserHomeDir(); err == nil {
			startDir = h
		} else {
			startDir = "."
		}
	}
	fp := filepicker.New()
	fp.CurrentDirectory = startDir
	fp.DirAllowed = true
	fp.FileAllowed = true
	fp.AutoHeight = false // must be false; Height set explicitly below
	fp.Height = 12        // safe default before first WindowSizeMsg
	fp.ShowSize = false
	fp.ShowPermissions = false
	fp.ShowHidden = false
	fp.KeyMap.Select.SetKeys(" ")                           // Space = confirm dir; Enter = navigate in
	fp.KeyMap.Back.SetKeys("h", "backspace", "left", "esc") // keep esc for Back (not cancel)

	m := setupModel{opts: opts, fp: fp, ti: ti}
	m.fpLoadDir(startDir)

	// If a startup channel is provided, always begin at stepStartup.
	if opts.StartupCh != nil {
		m.step = stepStartup
	} else {
		m.startupDone = true
		m.step = m.firstStep()
		if m.step == stepParallelJobs {
			m.ti.Focus()
		}
	}
	return m
}

// firstStep returns the first question step that needs user input.
func (m *setupModel) firstStep() setupStep {
	if m.opts.NeedFolder {
		return stepFolder
	}
	return m.nextStepAfter(stepFolder)
}

// nextStepAfter returns the next step that needs user input after s.
func (m *setupModel) nextStepAfter(s setupStep) setupStep {
	for next := s + 1; next < stepDone; next++ {
		switch next {
		case stepStartup:
			// never returned — startup is entered directly, not via advance()
		case stepFolder:
			if m.opts.NeedFolder {
				return next
			}
		case stepConfirm:
			if m.opts.NeedFolder {
				return next
			}
		case stepBypass:
			if m.opts.NeedBypass {
				return next
			}
		case stepForceHevc:
			if m.opts.NeedForceHevc {
				return next
			}
		case stepParallelJobs:
			if m.opts.NeedParallelJobs {
				return next
			}
		case stepOutputDrive:
			if m.opts.NeedOutputDrive {
				return next
			}
		}
	}
	return stepDone
}

func (m setupModel) init() tea.Cmd {
	if m.step == stepStartup && m.opts.StartupCh != nil {
		return waitStartupLine(m.opts.StartupCh)
	}
	if m.step == stepFolder {
		// Height is set in newSetupModel (safe default = 12); WindowSizeMsg
		// will update it once the terminal size is known.
		return m.fp.Init()
	}
	return nil
}

func (m setupModel) update(msg tea.Msg) (setupModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.syncFpHeight()

	case msgStartupLine:
		if msg.done {
			// Startup goroutine finished — advance to questions.
			m.startupDone = true
			next := m.firstStep()
			m.step = next
			var cmd tea.Cmd
			if next == stepFolder {
				// Apply known terminal height before Init() loads the directory,
				// so m.fp.max is computed correctly from a non-zero Height.
				m.syncFpHeight()
				m.fpLoadDir(m.fp.CurrentDirectory)
				cmd = m.fp.Init()
			} else if next == stepParallelJobs {
				m.ti.Focus()
			}
			return m, cmd
		}
		// Append the new status line and keep listening.
		m.startupLines = append(m.startupLines, msg.text)
		m.startupTick++
		return m, waitStartupLine(m.opts.StartupCh)

	case tea.KeyMsg:
		switch m.step {
		case stepStartup:
			// Allow early exit; everything else is ignored until startup finishes.
			if msg.String() == "ctrl+c" || msg.String() == "esc" {
				m.answers.Cancelled = true
				m.step = stepDone
				return m, nil
			}
			return m, nil

		case stepFolder:
			// Drive-switcher overlay takes priority.
			if m.fpDriveOverlay {
				switch msg.String() {
				case "up", "k":
					if m.fpDriveCursor > 0 {
						m.fpDriveCursor--
					}
				case "down", "j":
					if m.fpDriveCursor < len(m.opts.AvailableDrives)-1 {
						m.fpDriveCursor++
					}
				case "enter":
					return m, m.jumpToDrive()
				case "tab", "esc":
					m.fpDriveOverlay = false
				case "ctrl+c":
					m.answers.Cancelled = true
					m.step = stepDone
					return m, nil
				}
				return m, nil // swallow all keys while overlay is open
			}
			// Normal folder-picker keys.
			switch msg.String() {
			case "tab":
				if len(m.opts.AvailableDrives) > 0 {
					m.fpDriveOverlay = true
					m.fpDriveCursor = 0
					// Pre-select the drive that matches the current directory.
					cur := strings.ToUpper(m.fp.CurrentDirectory)
					for i, d := range m.opts.AvailableDrives {
						if strings.HasPrefix(cur, strings.ToUpper(strings.SplitN(d, " ", 2)[0])) {
							m.fpDriveCursor = i
							break
						}
					}
				}
				return m, nil
			case "ctrl+c":
				m.answers.Cancelled = true
				m.step = stepDone
				return m, nil
			case "q":
				m.answers.Cancelled = true
				m.step = stepDone
				return m, nil
			}
			// (Space and Enter are intercepted below in the sub-component delegate)

		case stepConfirm:
			switch msg.String() {
			case "enter", " ":
				// User confirmed the scan overview — proceed to next question.
				m.answers.Paths = make([]string, len(m.selectedPaths))
				copy(m.answers.Paths, m.selectedPaths)
				m.advance()
				return m, m.focusCmd()
			case "backspace", "b":
				// Go back to the picker to add/remove selections.
				m.step = stepFolder
				return m, nil
			case "ctrl+c", "esc", "q":
				m.answers.Cancelled = true
				m.step = stepDone
				return m, nil
			}
			return m, nil

		case stepBypass, stepForceHevc:
			switch strings.ToLower(msg.String()) {
			case "y":
				if m.step == stepBypass {
					m.answers.Bypass = true
				} else {
					m.answers.ForceHevc = true
				}
				m.advance()
				return m, m.focusCmd()
			case "n", "enter":
				m.advance()
				return m, m.focusCmd()
			case "ctrl+c", "esc":
				m.answers.Cancelled = true
				m.step = stepDone
				return m, nil
			}
			return m, nil // ignore other keys

		case stepParallelJobs:
			switch msg.String() {
			case "enter":
				n := m.opts.DefaultParallel
				if v, err := strconv.Atoi(strings.TrimSpace(m.ti.Value())); err == nil && v >= 1 {
					if v > 8 {
						v = 8
					}
					n = v
				}
				m.answers.ParallelJobs = n
				m.advance()
				return m, m.focusCmd()
			case "ctrl+c", "esc":
				m.answers.Cancelled = true
				m.step = stepDone
				return m, nil
			}

		case stepOutputDrive:
			if !m.driveYesAsked {
				// Ask "different drive?"
				switch strings.ToLower(msg.String()) {
				case "y":
					m.driveYesAsked = true
					m.driveWantsDiff = true
					return m, nil
				case "n", "enter":
					m.answers.OutputDrive = ""
					m.advance()
					return m, m.focusCmd()
				case "ctrl+c", "esc":
					m.answers.Cancelled = true
					m.step = stepDone
					return m, nil
				}
				return m, nil
			}
			// Drive list navigation.
			switch msg.String() {
			case "up", "k":
				if m.driveCursor > 0 {
					m.driveCursor--
				}
			case "down", "j":
				if m.driveCursor < len(m.opts.AvailableDrives)-1 {
					m.driveCursor++
				}
			case "enter":
				if len(m.opts.AvailableDrives) > 0 {
					selected := m.opts.AvailableDrives[m.driveCursor]
					m.answers.OutputDrive = strings.SplitN(selected, " ", 2)[0]
				}
				m.advance()
				return m, m.focusCmd()
			case "esc":
				// "cancel drive selection" = use same drive
				m.answers.OutputDrive = ""
				m.advance()
				return m, m.focusCmd()
			case "ctrl+c":
				m.answers.Cancelled = true
				m.step = stepDone
				return m, nil
			}
			return m, nil
		}
	}

	// Delegate to sub-components.
	var cmd tea.Cmd
	switch m.step {
	case stepFolder:
		// Don't forward key events to the filepicker while the drive overlay is open.
		if _, isKey := msg.(tea.KeyMsg); isKey && m.fpDriveOverlay {
			break
		}
		if km, isKey := msg.(tea.KeyMsg); isKey {
			switch km.String() {
			case " ":
				// Space: toggle the highlighted entry (dir or file) without
				// navigating into it.  We use our own fpFiles/fpCursor mirror
				// instead of synthetic key tricks.
				if len(m.fpFiles) == 0 {
					return m, nil
				}
				entry := m.fpFiles[m.fpCursor]
				full := filepath.Join(m.fp.CurrentDirectory, entry.Name())
				m.selectedPaths = togglePath(m.selectedPaths, full)
				m.syncFpHeight()
				return m, nil
			case "c":
				// c: confirm selection and advance to the scan overview.
				if len(m.selectedPaths) > 0 {
					m.scan = scanPaths(m.selectedPaths, m.opts.VideoExtensions)
					m.advance() // → stepConfirm
					return m, nil
				}
				// Nothing selected yet — ignore.
				return m, nil
			case "delete", "d":
				// Remove the last selected path.
				if len(m.selectedPaths) > 0 {
					m.selectedPaths = m.selectedPaths[:len(m.selectedPaths)-1]
					m.syncFpHeight()
				}
				return m, nil
			// Mirror cursor-movement keys so fpCursor stays in sync with fp.
			case "j", "down", "ctrl+n":
				if m.fpCursor < len(m.fpFiles)-1 {
					m.fpCursor++
				}
			case "k", "up", "ctrl+p":
				if m.fpCursor > 0 {
					m.fpCursor--
				}
			case "g":
				m.fpCursor = 0
			case "G":
				if len(m.fpFiles) > 0 {
					m.fpCursor = len(m.fpFiles) - 1
				}
			case "J", "pgdown":
				m.fpCursor += m.fp.Height
				if m.fpCursor >= len(m.fpFiles) {
					m.fpCursor = len(m.fpFiles) - 1
				}
			case "K", "pgup":
				m.fpCursor -= m.fp.Height
				if m.fpCursor < 0 {
					m.fpCursor = 0
				}
			}
		}
		prevDir := m.fp.CurrentDirectory
		m.fp, cmd = m.fp.Update(msg)
		// If the filepicker navigated into or out of a directory, reload our
		// mirror so Space always sees the correct entries.
		if m.fp.CurrentDirectory != prevDir {
			m.fpLoadDir(m.fp.CurrentDirectory)
		}
	case stepParallelJobs:
		m.ti, cmd = m.ti.Update(msg)
	}

	return m, cmd
}

// syncFpHeight recalculates m.fp.Height so that the filepicker + the
// selected-paths list below it always fit within the terminal window.
// Call this whenever m.selectedPaths or m.height changes.
func (m *setupModel) syncFpHeight() {
	fpH, _ := m.splitHeights()
	m.fp.Height = fpH
}

// splitHeights returns (fpHeight, selHeight) — the number of rows to give the
// filepicker and the selection list respectively, so that everything fits
// within m.height.
//
// Chrome lines in stepFolder (non-pane content):
//
//	title+blank=2, label=1, hint=1, curdir+blank=2, border top+bottom=2 → 8
//	plus blank-before-sel=1 and sel-header=1 when selection is non-empty → +2
func (m *setupModel) splitHeights() (fpHeight, selHeight int) {
	const chromeNoSel = 8 // chrome when selection list is absent
	available := m.height - chromeNoSel
	if available < 6 {
		available = 6
	}

	n := len(m.selectedPaths)
	if n == 0 {
		return available, 0
	}

	// 2 extra chrome lines when the selection pane is present (blank + header).
	available -= 2

	// Cap selection pane at 40% of available, min 1 item row, max 8 rows.
	maxSel := available * 40 / 100
	if maxSel < 1 {
		maxSel = 1
	}
	if maxSel > 8 {
		maxSel = 8
	}
	selH := n // one row per item
	if selH > maxSel {
		selH = maxSel
	}

	fpH := available - selH
	if fpH < 3 {
		fpH = 3
	}
	return fpH, selH
}

// advance moves to the next needed step.
func (m *setupModel) advance() {
	m.step = m.nextStepAfter(m.step)
	if m.step == stepParallelJobs {
		m.ti.Focus()
	}
}

// focusCmd returns filepicker Init when we've moved back to the folder step
// (shouldn't happen) or nil otherwise.
func (m *setupModel) focusCmd() tea.Cmd {
	return nil
}

// jumpToDrive teleports the filepicker to the drive selected in the overlay,
// closes the overlay, and reloads the directory listing.
func (m *setupModel) jumpToDrive() tea.Cmd {
	m.fpDriveOverlay = false
	if len(m.opts.AvailableDrives) == 0 {
		return nil
	}
	raw := m.opts.AvailableDrives[m.fpDriveCursor]
	root := strings.SplitN(raw, " ", 2)[0] // e.g. "D:\"
	m.fp.CurrentDirectory = root
	// Reset scroll/selection stacks so the new directory starts at the top.
	m.fp = resetFilepickerView(m.fp)
	m.fpLoadDir(root)
	return m.fp.Init()
}

// resetFilepickerView zeroes the visible-window state of a filepicker so that
// after a directory jump the list starts at the top.
func resetFilepickerView(fp filepicker.Model) filepicker.Model {
	// Re-create a fresh model copying all config fields, preserving Height.
	fresh := filepicker.New()
	fresh.CurrentDirectory = fp.CurrentDirectory
	fresh.DirAllowed = fp.DirAllowed
	fresh.FileAllowed = fp.FileAllowed
	fresh.AutoHeight = fp.AutoHeight
	fresh.Height = fp.Height
	fresh.ShowSize = fp.ShowSize
	fresh.ShowPermissions = fp.ShowPermissions
	fresh.ShowHidden = fp.ShowHidden
	fresh.KeyMap = fp.KeyMap
	fresh.Styles = fp.Styles
	return fresh
}

// fpLoadDir reads dir synchronously and stores the (sorted, hidden-filtered)
// entries in m.fpFiles, resetting m.fpCursor to 0.  Mirrors what the
// filepicker does internally so we always know which entry is highlighted.
func (m *setupModel) fpLoadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.fpFiles = nil
		m.fpCursor = 0
		return
	}
	// Sort: dirs first, then alphabetical — same order as filepicker.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() == entries[j].IsDir() {
			return entries[i].Name() < entries[j].Name()
		}
		return entries[i].IsDir()
	})
	// Filter hidden files (same logic as filepicker when ShowHidden=false).
	if !m.fp.ShowHidden {
		var visible []os.DirEntry
		for _, e := range entries {
			hidden, _ := filepicker.IsHidden(e.Name())
			if !hidden {
				visible = append(visible, e)
			}
		}
		entries = visible
	}
	m.fpFiles = entries
	m.fpCursor = 0
}

// togglePath adds path to the slice if absent, or removes it if present.
func togglePath(paths []string, path string) []string {
	for i, p := range paths {
		if strings.EqualFold(p, path) {
			return append(paths[:i], paths[i+1:]...)
		}
	}
	return append(paths, path)
}

// scanPaths walks all selected paths and counts directories and matching files.
// totalDirs counts each selected root dir plus every subdirectory inside them.
func scanPaths(paths []string, extensions []string) confirmScan {
	extSet := make(map[string]bool, len(extensions))
	for _, e := range extensions {
		extSet[strings.ToLower(e)] = true
	}
	matchAll := len(extSet) == 0

	seen := make(map[string]bool)
	sc := confirmScan{}

	for _, root := range paths {
		root = filepath.Clean(root)
		if seen[root] {
			continue
		}
		seen[root] = true
		sc.baseDirs = append(sc.baseDirs, root)

		info, err := os.Stat(root)
		if err != nil {
			continue
		}

		if !info.IsDir() {
			// Single file selected.
			ext := strings.ToLower(filepath.Ext(root))
			if matchAll || extSet[ext] {
				sc.totalFiles++
			}
			continue
		}

		// Count the root dir itself.
		sc.totalDirs++

		filepath.Walk(root, func(p string, fi os.FileInfo, err error) error { //nolint:errcheck
			if err != nil || fi == nil {
				return nil
			}
			if fi.IsDir() {
				if p != root {
					sc.totalDirs++
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(p))
			if matchAll || extSet[ext] {
				sc.totalFiles++
			}
			return nil
		})
	}
	return sc
}

// done returns true when setup has completed (all answers collected).
func (m *setupModel) done() bool { return m.step == stepDone }

// ── Setup view ────────────────────────────────────────────────────────────────

var (
	setupStyleTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true)
	setupStyleLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Bold(true)
	setupStyleHint    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	setupStyleAnswer  = lipgloss.NewStyle().Foreground(lipgloss.Color("#22C55E")).Bold(true)
	setupStyleCurrent = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA"))
	setupStyleBorder  = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#4B5563"))
	setupStyleCursor        = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED")).Bold(true)
	setupStyleDriveNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	setupStyleDriveSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7C3AED")).
				Bold(true)
	setupStyleStartupLine = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	setupStyleStartupDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	setupStyleSpinner     = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m setupModel) view() string {
	w := m.width - 2
	if w < 50 {
		w = 50
	}
	inner := w - 4 // border + padding

	var b strings.Builder

	// Title
	b.WriteString(setupStyleTitle.Render("Video Converter  —  Setup") + "\n\n")

	switch m.step {
	case stepStartup:
		spinner := setupStyleSpinner.Render(spinnerFrames[m.startupTick%len(spinnerFrames)])
		b.WriteString(spinner + "  " + setupStyleLabel.Render("Initialising…") + "\n\n")
		// Show all received status lines; dim older ones.
		for i, line := range m.startupLines {
			if i == len(m.startupLines)-1 {
				b.WriteString("  " + setupStyleStartupLine.Render(line) + "\n")
			} else {
				b.WriteString("  " + setupStyleStartupDim.Render(line) + "\n")
			}
		}
		b.WriteString("\n" + setupStyleHint.Render("  [Esc] cancel") + "\n")

	case stepFolder:
		b.WriteString(setupStyleLabel.Render("Select input folder") + "\n")
		if m.fpDriveOverlay {
			hint := setupStyleHint.Render("[↑/k][↓/j] navigate drives   [Enter] jump   [Tab/Esc] close")
			b.WriteString(hint + "\n")
			b.WriteString(setupStyleCurrent.Render("  "+m.fp.CurrentDirectory) + "\n\n")
			// Drive list overlay.
			for i, d := range m.opts.AvailableDrives {
				cursor := "  "
				style := setupStyleDriveNormal
				if i == m.fpDriveCursor {
					cursor = setupStyleCursor.Render("❯ ")
					style = setupStyleDriveSelected
				}
				b.WriteString(style.Render(truncate(cursor+d, inner)) + "\n")
			}
		} else {
			tabHint := ""
			if len(m.opts.AvailableDrives) > 0 {
				tabHint = "  [Tab] switch drive"
			}
			confirmHint := ""
			if len(m.selectedPaths) > 0 {
				confirmHint = "  [c] Confirm"
			}
			hint := setupStyleHint.Render("[Space] toggle highlighted  [←/h] up  [→/l/Enter] open  [d] remove last  [q] cancel" + tabHint + confirmHint)
			b.WriteString(hint + "\n")
			b.WriteString(setupStyleCurrent.Render("  "+m.fp.CurrentDirectory) + "\n\n")
			b.WriteString(m.fp.View())
			// Show selected paths below the filepicker — capped to selHeight
			// rows so the list never pushes content off-screen.
			if len(m.selectedPaths) > 0 {
				_, selH := m.splitHeights()
				// selH includes the header line; items get selH-1 rows.
				itemRows := selH - 1
				if itemRows < 1 {
					itemRows = 1
				}
				paths := m.selectedPaths
				// Show the most-recently-added items (tail of the slice).
				if len(paths) > itemRows {
					paths = paths[len(paths)-itemRows:]
				}
				b.WriteString("\n")
				label := fmt.Sprintf("  Selected (%d)", len(m.selectedPaths))
				if len(m.selectedPaths) > itemRows {
					label += fmt.Sprintf("  [showing last %d]", itemRows)
				}
				b.WriteString(setupStyleLabel.Render(label+"  [d] remove last") + "\n")
				for _, p := range paths {
					b.WriteString(setupStyleAnswer.Render("  ✓ "+truncate(p, inner-4)) + "\n")
				}
			}
		}

	case stepConfirm:
		b.WriteString(setupStyleLabel.Render("Conversion overview") + "\n\n")
		// Stats row.
		b.WriteString(setupStyleHint.Render("  Directories : ") +
			setupStyleAnswer.Render(strconv.Itoa(m.scan.totalDirs)) + "\n")
		b.WriteString(setupStyleHint.Render("  Video files : ") +
			setupStyleAnswer.Render(strconv.Itoa(m.scan.totalFiles)) + "\n\n")
		// Base dirs list.
		b.WriteString(setupStyleLabel.Render("  Source paths:") + "\n")
		for _, d := range m.scan.baseDirs {
			b.WriteString(setupStyleAnswer.Render("    • "+truncate(d, inner-6)) + "\n")
		}
		b.WriteString("\n" + setupStyleHint.Render("  [Enter/Space] Start   [b/Backspace] Back   [q] Cancel") + "\n")

	case stepBypass, stepForceHevc:
		// Show already-answered items above.
		b.WriteString(m.renderAnsweredAbove())
		var question string
		if m.step == stepBypass {
			question = "Force re-conversion of already-processed files? (bypass DB check)"
		} else {
			question = "Re-compress files that are already HEVC/H.265?"
		}
		b.WriteString(setupStyleLabel.Render(question) + "\n")
		b.WriteString(setupStyleHint.Render("  [y] Yes   [n / Enter] No   [Esc] cancel") + "\n")

	case stepParallelJobs:
		b.WriteString(m.renderAnsweredAbove())
		b.WriteString(setupStyleLabel.Render(
			fmt.Sprintf("Parallel conversion jobs  (recommended: %d)", m.opts.DefaultParallel),
		) + "\n")
		b.WriteString(setupStyleHint.Render("  Enter a number 1–8, or press Enter to accept the recommendation") + "\n  ")
		b.WriteString(m.ti.View() + "\n")

	case stepOutputDrive:
		b.WriteString(m.renderAnsweredAbove())
		if !m.driveYesAsked {
			b.WriteString(setupStyleLabel.Render("Write output to a different drive?") + "\n")
			b.WriteString(setupStyleHint.Render("  [y] Yes   [n / Enter] No — use source drive   [Esc] cancel") + "\n")
		} else {
			b.WriteString(setupStyleLabel.Render("Select output drive") + "\n")
			b.WriteString(setupStyleHint.Render("  [↑/k] [↓/j] navigate   [Enter] confirm   [Esc] use source drive") + "\n\n")
			for i, d := range m.opts.AvailableDrives {
				cursor := "  "
				style := setupStyleDriveNormal
				if i == m.driveCursor {
					cursor = setupStyleCursor.Render("❯ ")
					style = setupStyleDriveSelected
				}
				line := truncate(cursor+d, inner)
				b.WriteString(style.Render(line) + "\n")
			}
		}
	}

	return setupStyleBorder.Width(w).MaxHeight(m.height).Render(b.String()) + "\n"
}

// renderAnsweredAbove renders a compact summary of questions already answered.
func (m *setupModel) renderAnsweredAbove() string {
	var b strings.Builder
	if !m.opts.NeedFolder && len(m.answers.Paths) == 0 {
		// paths came from CLI — nothing to show
	} else if len(m.answers.Paths) > 0 {
		label := "Paths"
		if len(m.answers.Paths) == 1 {
			label = "Folder"
		}
		b.WriteString(setupStyleHint.Render("  "+label+"  ") +
			setupStyleAnswer.Render(strings.Join(m.answers.Paths, ", ")) + "\n")
	}
	if m.step > stepBypass && m.opts.NeedBypass {
		val := "No"
		if m.answers.Bypass {
			val = "Yes"
		}
		b.WriteString(setupStyleHint.Render("  Bypass DB check  ") +
			setupStyleAnswer.Render(val) + "\n")
	}
	if m.step > stepForceHevc && m.opts.NeedForceHevc {
		val := "No"
		if m.answers.ForceHevc {
			val = "Yes"
		}
		b.WriteString(setupStyleHint.Render("  Re-compress HEVC  ") +
			setupStyleAnswer.Render(val) + "\n")
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}
