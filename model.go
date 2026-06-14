package main

import (
	//"fmt"

	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.design/x/clipboard"
)

// -----------------------------------------------------------------------
// Application modes
// -----------------------------------------------------------------------

const (
	ModeClipEnter     = "copy-on-enter"
	ModeClipSelect    = "copy-on-select"
	ModeVisibleEnter  = "show-on-enter"
	ModeVisibleSelect = "show-on-select"
)

var modeSet = [4]string{ModeClipEnter, ModeClipSelect, ModeVisibleEnter, ModeVisibleSelect}

// -----------------------------------------------------------------------
// Focus targets
// -----------------------------------------------------------------------

type focus int

const (
	focusTable focus = iota
	focusMenu
	focusPasswordEnter // overlay: enter existing password
	focusPasswordNew   // overlay: create / change password
	focusEdit          // overlay: edit / add record
	focusMode          // overlay: pick mode
	focusConfirmWWW    // overlay: confirm open URL
	focusConfirmDelete // overlay: confirm delete
	focusConfirmSave   // overlay: confirm save
	focusConfirmExit   // overlay: confirm exit (unsaved)
	focusConfirmGit    // overlay: confirm git push
	focusGitResult     // overlay: git push result
)

// -----------------------------------------------------------------------
// Theme
// -----------------------------------------------------------------------

type SpurTheme struct {
	MainFg      lipgloss.Color
	MainBg      lipgloss.Color
	AccentFg    lipgloss.Color
	TrackingFg  lipgloss.Color
	FormFg      lipgloss.Color
	FormBg      lipgloss.Color
	FormInputBg lipgloss.Color
}

var defaultTheme = SpurTheme{
	MainFg:      lipgloss.Color("15"), // white
	MainBg:      lipgloss.Color("0"),  // black
	AccentFg:    lipgloss.Color("15"), // white
	TrackingFg:  lipgloss.Color("1"),  // red
	FormFg:      lipgloss.Color("15"), // white
	FormBg:      lipgloss.Color("8"),  // gray
	FormInputBg: lipgloss.Color("0"),  // black
}

// -----------------------------------------------------------------------
// Messages
// -----------------------------------------------------------------------

type msgDataLoaded struct {
	data        []byte
	passwd      string
	alterColumn int
}

type msgGitResult struct {
	text string
	err  error
}

// -----------------------------------------------------------------------
// Model
// -----------------------------------------------------------------------

const hiddenText = " **************** "
const FmtFieldTitle = "    Column %d     "

// Model is the root Bubble Tea model.
type Model struct {
	// ---- data ----
	keys       []string
	records    map[string][]string
	visibility map[string]string
	width      int // max number of value columns across all records
	commits    []string

	// ---- navigation ----
	activeRow         int  // 1-based, row 0 is header
	activeColumn      int  // 1-based, col 0 is row-number
	tableOffset       int  // first visible data row (0-based)
	menuIndex         int  // focused menu button (0-based)
	enteredCell       bool // user pressed Enter on this cell in visible-on-enter mode
	scrollbarDragging bool // user is dragging the scrollbar
	scrollbarDragY    int  // Y position where scrollbar drag started

	// ---- mode ----
	mode      string
	modeIndex int

	// ---- file / git ----
	cribName string
	cribPath string
	cribBase string
	hasGit   bool
	dirty    bool // unsaved changes

	// ---- password state ----
	passwd          string
	needOldPassword bool

	// ---- focus / overlay ----
	currentFocus focus

	// ---- overlay sub-states ----
	// password forms
	pwdInputs     [3]string // [0]=old [1]=new1 [2]=new2
	pwdCursor     int       // which input field is focused
	pwdTextCursor int       // rune offset within the active password field
	pwdTitle      string

	// edit form
	editKey        string
	editValues     []string
	editVisibility string
	editCursor     int // 0=name, 1..n=values, n+1=Submit, n+2=Hide/Reveal, n+3=Clear, n+4=Cancel
	editTextCursor int // rune offset within the active input field

	// mode picker
	modeCursor int

	// confirm overlays
	confirmText        string
	confirmOK          string
	confirmCancel      string
	confirmYesFn       func() tea.Cmd
	confirmNoFn        func() tea.Cmd
	confirmCursor      int   // 0=OK/Yes, 1=Cancel/No
	confirmReturnFocus focus // where to go after confirm is dismissed

	// git result
	gitResultText string

	// ---- startup ----
	alterColumnAtStart int // -ta flag value, used when decrypting at startup

	// ---- terminal size ----
	termWidth  int
	termHeight int

	// ---- theme ----
	SpurTheme
}

func initialModel(cribName string, mode string, modeIdx int, theme SpurTheme, alterColumn int) Model {
	m := Model{
		records:      make(map[string][]string),
		visibility:   make(map[string]string),
		cribName:     cribName,
		cribPath:     filepath.Dir(cribName),
		cribBase:     filepath.Base(cribName),
		mode:         mode,
		modeIndex:    modeIdx,
		SpurTheme:    theme,
		termWidth:    109,
		termHeight:   45,
		activeRow:    1,
		activeColumn: 1,
	}
	m.hasGit = CheckGit(m.cribPath) == nil
	return m
}

// -----------------------------------------------------------------------
// Data helpers
// -----------------------------------------------------------------------

func (m *Model) attachData(data []byte, passwd string, columnToAlter int) {
	m.passwd = passwd
	m.keys = m.keys[:0]
	m.records = make(map[string][]string)
	m.visibility = make(map[string]string)
	m.width = 0

	var sdata []string
	if len(data) > 0 {
		sdata = strings.Split(string(data), "\n")
	}
	for _, s := range sdata {
		elems := strings.Split(s, ",")
		if len(elems) < 2 {
			continue
		}
		values := elems[2:]
		if columnToAlter > 0 && columnToAlter <= len(values) {
			values = append(values, "")
			copy(values[columnToAlter:], values[columnToAlter-1:])
			values[columnToAlter-1] = ""
		} else if columnToAlter < 0 {
			idx := -columnToAlter - 1
			if idx < len(values) {
				values = append(values[:idx], values[idx+1:]...)
			}
		}
		m.keys = append(m.keys, elems[1])
		if len(values) > m.width {
			m.width = len(values)
		}
		m.records[elems[1]] = values
		m.visibility[elems[1]] = elems[0]
	}
}

// updateRecord adds/updates/deletes a record and returns the sorted index of the key.
func (m *Model) updateRecord(key string, values []string, vis string) int {
	if values == nil {
		m.commits = append(m.commits, key+" deleted")
		delete(m.records, key)
		delete(m.visibility, key)
	} else {
		m.commits = append(m.commits, key+" changed")
		m.records[key] = values
		m.visibility[key] = vis
	}
	m.keys = m.keys[:0]
	m.width = 0
	for k, v := range m.records {
		m.keys = append(m.keys, k)
		if m.width < len(v) {
			m.width = len(v)
		}
	}
	sort.Slice(m.keys, func(i, j int) bool {
		return strings.ToLower(m.keys[i]) < strings.ToLower(m.keys[j])
	})
	for i, k := range m.keys {
		if k == key {
			return i
		}
	}
	return 0
}

func (m *Model) save() {
	var sb strings.Builder
	for _, key := range m.keys {
		sb.WriteString(m.visibility[key])
		sb.WriteByte(',')
		sb.WriteString(key)
		for _, v := range m.records[key] {
			sb.WriteByte(',')
			sb.WriteString(v)
		}
		sb.WriteByte('\n')
	}
	if err := EncryptFile(m.cribName, []byte(sb.String()), m.passwd); err != nil {
		panic(err.Error())
	}
	m.dirty = false
}

// cellText returns the display text for a given (1-based) row and column.
// col=1 → record name, col>=2 → values
func (m *Model) cellText(row, col int) string {
	if row < 1 || row > len(m.keys) {
		return ""
	}
	key := m.keys[row-1]
	if col == 1 {
		return key
	}
	values := m.records[key]
	idx := col - 2
	if idx < 0 || idx >= len(values) {
		return ""
	}
	if m.visibility[key] == "h" && len(values[idx]) > 0 {
		return hiddenText
	}
	return values[idx]
}

// visibleCellText is like cellText but always shows real value (for revealed cells).
func (m *Model) realCellText(row, col int) string {
	if row < 1 || row > len(m.keys) {
		return ""
	}
	key := m.keys[row-1]
	if col == 1 {
		return key
	}
	values := m.records[key]
	idx := col - 2
	if idx < 0 || idx >= len(values) {
		return ""
	}
	return values[idx]
}

func (m *Model) copyActiveToClipboard() {
	text := m.realCellText(m.activeRow, m.activeColumn)
	clipboard.Write(clipboard.FmtText, []byte(text))
}

// numDataCols returns the number of value columns to display (at least 4).
func (m *Model) numDataCols() int {
	return max(m.width, 4)
}

// saveMenuLabel returns "Save!" when dirty, "Save" otherwise.
func (m *Model) saveMenuLabel() string {
	if m.dirty {
		return "Save!"
	}
	return "Save"
}

// menuButtons returns the ordered list of top-menu button labels.
func (m *Model) menuButtons() []string {
	btns := []string{"Select", firstToUpper(m.modeSet()[m.modeIndex]), "WWW", "Edit", "Delete", m.saveMenuLabel()}
	if m.hasGit {
		btns = append(btns, "Git")
	}
	btns = append(btns, "Password", "Exit")
	return btns
}

func (m *Model) modeSet() [4]string { return modeSet }

func firstToUpper(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// clampRow keeps activeRow in valid range [1, len(keys)].
func (m *Model) clampRow() {
	if len(m.keys) == 0 {
		m.activeRow = 1
		return
	}
	m.activeRow = max(m.activeRow, 1)
	m.activeRow = min(m.activeRow, len(m.keys))
}

// clampCol keeps activeColumn in [1, 1+numDataCols].
func (m *Model) clampCol() {
	maxCol := 1 + m.numDataCols()
	m.activeColumn = max(m.activeColumn, 1)
	m.activeColumn = min(m.activeColumn, maxCol)
}

// visibleRows returns how many data rows fit given terminal height.
func (m *Model) visibleRows() int {
	// header=1, menu=1, border lines ~3
	// return max(m.termHeight-5, 1)
	return max(m.termHeight-6, 1)
}

// adjustOffset keeps tableOffset so activeRow is visible.
func (m *Model) adjustOffset() {
	vis := m.visibleRows()
	dataRow := m.activeRow - 1 // 0-based
	if dataRow < m.tableOffset {
		m.tableOffset = dataRow
	}
	if dataRow >= m.tableOffset+vis {
		m.tableOffset = dataRow - vis + 1
	}
}

// gitButtonIndex returns the index of the Git button, or -1.
func (m *Model) gitButtonIndex() int {
	if !m.hasGit {
		return -1
	}
	// Save is at index 5, Git at 6 when present
	return 6
}

// saveButtonIndex returns the index of the Save button.
func (m *Model) saveButtonIndex() int { return 5 }

// -----------------------------------------------------------------------
// Init
// -----------------------------------------------------------------------

func (m Model) Init() tea.Cmd {
	return nil
}

// -----------------------------------------------------------------------
// Styling helpers (computed on demand so they respect theme)
// -----------------------------------------------------------------------

func (m *Model) styleBorder(fg, bg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(fg).
		Foreground(fg).
		Background(bg)
}

func (m *Model) styleCell(fg, bg lipgloss.Color, bold bool) lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(fg).Background(bg).Padding(0, 1)
	if bold {
		s = s.Bold(true)
	}
	return s
}
