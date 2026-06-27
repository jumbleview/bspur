package main

import (
	"net/url"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
	"golang.design/x/clipboard"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case msgDataLoaded:
		m.attachData(msg.data, msg.passwd, msg.alterColumn)
		m.activeRow = 1
		m.activeColumn = 1
		m.tableOffset = 0
		if m.mode == ModeClipSelect {
			m.copyActiveToClipboard()
		}
		m.currentFocus = focusTable
		if len(m.keys) == 0 {
			m.currentFocus = focusMenu
			m.menuIndex = 0
		}
		return m, nil

	case msgGitResult:
		m.gitResultText = msg.text
		if msg.err != nil {
			m.gitResultText += " " + msg.err.Error()
		}
		m.currentFocus = focusGitResult
		return m, nil

	case msgWrongPassword:
		m.pwdInputs = [3]string{}
		m.pwdCursor = 0
		m.pwdTextCursor = 0
		m.pwdTitle = msg.title
		m.currentFocus = focusPasswordEnter
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.currentFocus {
	case focusTable:
		return m.handleTableKey(msg)
	case focusMenu:
		return m.handleMenuKey(msg)
	case focusPasswordEnter:
		return m.handlePwdEnterKey(msg)
	case focusPasswordNew:
		return m.handlePwdNewKey(msg)
	case focusEdit:
		return m.handleEditKey(msg)
	case focusMode:
		return m.handleModeKey(msg)
	case focusConfirmWWW, focusConfirmDelete, focusConfirmSave, focusConfirmExit, focusConfirmGit:
		return m.handleConfirmKey(msg)
	case focusGitResult:
		return m.handleGitResultKey(msg)
	}
	return m, nil
}

// -----------------------------------------------------------------------
// Mouse handler
// -----------------------------------------------------------------------

// handleMouse processes left-click events.
//
// Screen layout (Y coordinates, 0-based):
//
//	Y=0        top menu bar
//	Y=1        table top border (┌─…─┐)
//	Y=2        table header row
//	Y=3        table separator  (├─…─┤)
//	Y=4+       table data rows
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Handle scrollbar drag
	if m.scrollbarDragging {
		if msg.Action == tea.MouseActionMotion {
			return m.updateScrollbarDrag(msg.Y)
		} else if msg.Action == tea.MouseActionRelease {
			m.scrollbarDragging = false
			return m, nil
		}
		return m, nil
	}

	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	if msg.Action == tea.MouseActionPress {
		return m.handleMousePress(msg)
	} else if msg.Action == tea.MouseActionRelease {
		return m, nil
	}

	return m, nil
}

// handleMousePress processes initial mouse press events.
func (m Model) handleMousePress(msg tea.MouseMsg) (tea.Model, tea.Cmd) {

	// Click on the menu bar → move focus + highlight the clicked button
	if msg.Y == 0 {
		m.currentFocus = focusMenu
		btns := m.menuButtons()
		x := 0
		for i, label := range btns {
			btnWidth := len(label) + 2 // 1-space padding each side
			if msg.X >= x && msg.X < x+btnWidth {
				m.menuIndex = i
				break
			}
			x += btnWidth
		}
		return m, nil
	}

	// Check if click is on the scrollbar (rightmost column)
	colWidths := m.computeColWidths()
	numDataCols := m.numDataCols()
	totalCols := numDataCols + 2
	tableWidth := 1 // leading │
	for c := 0; c < totalCols; c++ {
		tableWidth += colWidths[c] + 1 // +1 for │ separator
	}
	if msg.X >= tableWidth {
		return m.startScrollbarDrag(msg.Y)
	}

	// Clicks on the top border, header, or separator are ignored
	const tableDataStartY = 4
	if msg.Y < tableDataStartY {
		return m, nil
	}

	// Map Y to a data row (0-based index into m.keys)
	clickedIdx := (msg.Y - tableDataStartY) + m.tableOffset
	if clickedIdx < 0 || clickedIdx >= len(m.keys) {
		return m, nil
	}

	m.activeRow = clickedIdx + 1 // convert to 1-based
	m.currentFocus = focusTable
	m.adjustOffset()

	// Map X to a column.
	// Layout: │<col0>│<col1>│<col2>│…
	// The leading │ sits at screen column 0; cell content begins at X=1.
	xPos := msg.X - 1 // strip the leading │
	if xPos >= 0 {
		cumWidth := 0
		for c := range totalCols {
			if xPos >= cumWidth && xPos < cumWidth+colWidths[c] {
				if c >= 1 { // col 0 is the row-number gutter — not selectable
					m.activeColumn = c
				}
				break
			}
			cumWidth += colWidths[c] + 1 // +1 for the │ separator
		}
	}

	m.onSelectionChanged()
	return m, nil
}

// startScrollbarDrag initiates a scrollbar drag operation.
func (m Model) startScrollbarDrag(y int) (tea.Model, tea.Cmd) {
	const tableDataStartY = 4
	if y < tableDataStartY || len(m.keys) == 0 {
		return m, nil
	}

	vis := m.visibleRows()
	totalRows := len(m.keys)
	if vis >= totalRows {
		return m, nil // entire table fits, no scrolling needed
	}

	m.scrollbarDragging = true
	m.scrollbarDragY = y
	m.currentFocus = focusTable
	return m.updateScrollbarDrag(y)
}

// updateScrollbarDrag updates the table offset during a scrollbar drag.
func (m Model) updateScrollbarDrag(y int) (tea.Model, tea.Cmd) {
	const tableDataStartY = 4
	if len(m.keys) == 0 {
		return m, nil
	}

	vis := m.visibleRows()
	totalRows := len(m.keys)
	if vis >= totalRows {
		return m, nil
	}

	// Clamp Y to the valid scrollbar track range
	minY := tableDataStartY
	maxY := tableDataStartY + vis - 1
	y = min(max(y, minY), maxY)

	// Map Y position to tableOffset
	trackPos := y - tableDataStartY
	m.tableOffset = (trackPos * totalRows) / vis
	m.tableOffset = min(m.tableOffset, max(0, totalRows-vis))

	m.adjustOffset()
	return m, nil
}

// -----------------------------------------------------------------------
// Table focus
// -----------------------------------------------------------------------

func (m Model) handleTableKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.currentFocus = focusMenu
		m.menuIndex = 0
		return m, nil

	case tea.KeyEnter:
		if len(m.records) == 0 {
			return m, nil
		}
		switch m.mode {
		case ModeClipEnter:
			m.copyActiveToClipboard()
		case ModeClipSelect:
			m.copyActiveToClipboard()
		case ModeVisibleEnter:
			m.enteredCell = true
		}
		return m, nil

	case tea.KeyPgUp:
		m.activeRow -= m.visibleRows()
		if m.activeRow < 1 {
			m.activeRow = 1
		}
		m.adjustOffset()
		m.onSelectionChanged()
		return m, nil

	case tea.KeyPgDown:
		m.activeRow += m.visibleRows()
		if m.activeRow > len(m.keys) {
			m.activeRow = len(m.keys)
		}
		m.adjustOffset()
		m.onSelectionChanged()
		return m, nil

	case tea.KeyHome:
		m.activeRow = 1
		m.adjustOffset()
		m.onSelectionChanged()
		return m, nil

	case tea.KeyEnd:
		m.activeRow = len(m.keys)
		m.adjustOffset()
		m.onSelectionChanged()
		return m, nil

	case tea.KeyUp:
		if m.activeRow > 1 {
			m.activeRow--
			m.adjustOffset()
			m.onSelectionChanged()
		}
		return m, nil

	case tea.KeyDown:
		if m.activeRow < len(m.keys) {
			m.activeRow++
			m.adjustOffset()
			m.onSelectionChanged()
		}
		return m, nil

	case tea.KeyLeft:
		if m.activeColumn > 1 {
			m.activeColumn--
			m.onSelectionChanged()
		}
		return m, nil

	case tea.KeyRight:
		if m.activeColumn <= m.numDataCols() {
			m.activeColumn++
			m.onSelectionChanged()
		}
		return m, nil

	case tea.KeyCtrlC:
		clipboard.Write(clipboard.FmtText, []byte(""))
		return m, tea.Quit

	case tea.KeyRunes:
		// Letter navigation: jump to first key >= pressed letter
		r := msg.Runes[0]
		if unicode.IsLetter(r) {
			target := strings.ToUpper(string(r))
			for ix, ky := range m.keys {
				if strings.ToUpper(ky) >= target {
					m.activeRow = ix + 1
					m.adjustOffset()
					m.onSelectionChanged()
					break
				}
			}
		}
		return m, nil
	}
	return m, nil
}

// onSelectionChanged implements copy-on-select / visible-on-select behaviour.
func (m *Model) onSelectionChanged() {
	if m.mode == ModeVisibleEnter {
		m.enteredCell = false
	}
	if len(m.keys) == 0 {
		return
	}
	switch m.mode {
	case ModeClipSelect:
		m.copyActiveToClipboard()
	}
}

// -----------------------------------------------------------------------
// Menu focus
// -----------------------------------------------------------------------

func (m Model) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	btns := m.menuButtons()
	switch msg.Type {
	case tea.KeyRight, tea.KeyDown, tea.KeyTab:
		m.menuIndex = (m.menuIndex + 1) % len(btns)
		return m, nil
	case tea.KeyLeft, tea.KeyUp, tea.KeyShiftTab:
		m.menuIndex = (m.menuIndex - 1 + len(btns)) % len(btns)
		return m, nil
	case tea.KeyEnter, tea.KeySpace:
		return m.activateMenuButton(btns[m.menuIndex])
	case tea.KeyEsc:
		if len(m.keys) > 0 {
			m.currentFocus = focusTable
		}
		return m, nil
	case tea.KeyCtrlC:
		clipboard.Write(clipboard.FmtText, []byte(""))
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) activateMenuButton(label string) (tea.Model, tea.Cmd) {
	base := strings.TrimSuffix(label, "!")
	switch base {
	case "Select":
		if len(m.keys) > 0 {
			m.currentFocus = focusTable
			if m.mode == ModeClipSelect {
				m.copyActiveToClipboard()
			}
		}

	case firstToUpper(modeSet[m.modeIndex]):
		m.modeCursor = m.modeIndex
		m.currentFocus = focusMode

	case "WWW":
		var key, rawURL string
		if m.activeRow > 0 && len(m.keys) > 0 {
			key = m.keys[m.activeRow-1]
		}
		urlIdx := m.activeColumn - 2
		if key != "" && urlIdx >= 0 && urlIdx < len(m.records[key]) {
			rawURL = m.records[key][urlIdx]
		}
		openURL := rawURL
		if !strings.HasPrefix(openURL, "http://") && !strings.HasPrefix(openURL, "https://") {
			openURL = "https://" + openURL
		}
		parsed, parseErr := url.Parse(openURL)
		valid := parseErr == nil && parsed.Host != "" && strings.Contains(parsed.Host, ".")
		if valid {
			m.confirmText = "Go to URL:\n" + rawURL
			m.confirmOK = "Yes"
			m.confirmCancel = "No"
			capturedURL := openURL
			m.confirmYesFn = func() tea.Cmd {
				browser.OpenURL(capturedURL)
				return nil
			}
			m.confirmNoFn = func() tea.Cmd { return nil }
		} else {
			m.confirmText = "Not a valid URL:\n" + rawURL
			m.confirmOK = "OK"
			m.confirmCancel = ""
			m.confirmYesFn = func() tea.Cmd { return nil }
			m.confirmNoFn = nil
		}
		m.confirmCursor = 0
		m.confirmReturnFocus = focusTable
		m.currentFocus = focusConfirmWWW

	case "Edit":
		m.openEditForm()

	case "Delete":
		var key string
		if m.activeRow > 0 && len(m.keys) > 0 {
			key = m.keys[m.activeRow-1]
		}
		if len(key) > 0 {
			m.confirmText = "Delete record: " + key + "?"
			m.confirmOK = "Delete"
			m.confirmCancel = "Cancel"
			capturedKey := key
			m.confirmYesFn = func() tea.Cmd {
				m.updateRecord(capturedKey, nil, "")
				m.activeRow--
				m.clampRow()
				m.activeColumn = 1
				m.adjustOffset()
				return nil
			}
			m.confirmNoFn = func() tea.Cmd { return nil }
			m.confirmReturnFocus = focusTable
		} else {
			m.confirmText = "Nothing to delete. Record empty"
			m.confirmOK = "OK"
			m.confirmCancel = ""
			m.confirmYesFn = func() tea.Cmd { return nil }
			m.confirmNoFn = nil
			m.confirmReturnFocus = focusMenu
		}
		m.confirmCursor = 0
		m.currentFocus = focusConfirmDelete

	case "Save":
		m.confirmText = "Save page?"
		m.confirmOK = "Save"
		m.confirmCancel = "Cancel"
		m.confirmYesFn = func() tea.Cmd {
			m.save()
			return nil
		}
		m.confirmNoFn = func() tea.Cmd { return nil }
		m.confirmCursor = 0
		m.confirmReturnFocus = focusMenu
		m.currentFocus = focusConfirmSave

	case "Git":
		m.confirmText = "Push to remote?"
		m.confirmOK = "Yes"
		m.confirmCancel = "No"
		m.confirmYesFn = func() tea.Cmd {
			return func() tea.Msg {
				txt, err := PushRemote(m.cribPath, m.cribBase, m.commits)
				m.commits = m.commits[:0]
				return msgGitResult{text: txt, err: err}
			}
		}
		m.confirmNoFn = func() tea.Cmd { return nil }
		m.confirmCursor = 0
		m.confirmReturnFocus = focusMenu
		m.currentFocus = focusConfirmGit

	case "Password":
		m.pwdInputs = [3]string{}
		m.pwdCursor = 0
		m.pwdTextCursor = 0
		m.needOldPassword = true
		m.pwdTitle = " Change page password "
		m.currentFocus = focusPasswordNew

	case "Exit":
		if !m.dirty {
			clipboard.Write(clipboard.FmtText, []byte(""))
			return m, tea.Quit
		}
		m.confirmText = "Page not saved. Exit?"
		m.confirmOK = "Exit"
		m.confirmCancel = "Cancel"
		m.confirmYesFn = func() tea.Cmd {
			clipboard.Write(clipboard.FmtText, []byte(""))
			return tea.Quit
		}
		m.confirmNoFn = func() tea.Cmd { return nil }
		m.confirmCursor = 0
		m.confirmReturnFocus = focusMenu
		m.currentFocus = focusConfirmExit
	}
	return m, nil
}

// -----------------------------------------------------------------------
// Password — enter existing password
// -----------------------------------------------------------------------

func (m Model) handlePwdEnterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		m.pwdCursor = (m.pwdCursor + 1) % 3
		m.pwdTextCursor = len([]rune(m.pwdInputs[0]))
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.pwdCursor = (m.pwdCursor - 1 + 3) % 3
		m.pwdTextCursor = len([]rune(m.pwdInputs[0]))
		return m, nil
	case tea.KeyLeft:
		if m.pwdCursor == 0 {
			if m.pwdTextCursor > 0 {
				m.pwdTextCursor--
			}
		} else if m.pwdCursor > 1 {
			m.pwdCursor--
		}
		return m, nil
	case tea.KeyRight:
		if m.pwdCursor == 0 {
			if m.pwdTextCursor < len([]rune(m.pwdInputs[0])) {
				m.pwdTextCursor++
			}
		} else if m.pwdCursor < 2 {
			m.pwdCursor++
		}
		return m, nil
	case tea.KeyBackspace:
		if m.pwdCursor == 0 && m.pwdTextCursor > 0 {
			r := []rune(m.pwdInputs[0])
			r = append(r[:m.pwdTextCursor-1], r[m.pwdTextCursor:]...)
			m.pwdInputs[0] = string(r)
			m.pwdTextCursor--
		}
		return m, nil
	case tea.KeyDelete:
		if m.pwdCursor == 0 {
			r := []rune(m.pwdInputs[0])
			if m.pwdTextCursor < len(r) {
				r = append(r[:m.pwdTextCursor], r[m.pwdTextCursor+1:]...)
				m.pwdInputs[0] = string(r)
			}
		}
		return m, nil
	case tea.KeyEsc:
		clipboard.Write(clipboard.FmtText, []byte(""))
		return m, tea.Quit
	case tea.KeyEnter:
		if m.pwdCursor == 2 {
			clipboard.Write(clipboard.FmtText, []byte(""))
			return m, tea.Quit
		}
		return m.submitEnterPassword()
	case tea.KeyRunes:
		if m.pwdCursor == 0 {
			r := []rune(m.pwdInputs[0])
			r = append(r[:m.pwdTextCursor], append(msg.Runes, r[m.pwdTextCursor:]...)...)
			m.pwdInputs[0] = string(r)
			m.pwdTextCursor += len(msg.Runes)
		} else if m.pwdCursor == 1 {
			return m.submitEnterPassword()
		} else {
			clipboard.Write(clipboard.FmtText, []byte(""))
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

func (m Model) submitEnterPassword() (tea.Model, tea.Cmd) {
	passwd := m.pwdInputs[0]
	cribName := m.cribName
	alterColumn := m.alterColumnAtStart
	return m, func() tea.Msg {
		data, err := DecryptFile(cribName, passwd)
		if err != nil {
			return msgWrongPassword{title: " Wrong password. Repeat "}
		}
		return msgDataLoaded{data: data, passwd: passwd, alterColumn: alterColumn}
	}
}

type msgWrongPassword struct{ title string }

func (m Model) handlePwdNewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	numFields := 2
	if m.needOldPassword {
		numFields = 3
	}
	totalItems := numFields + 2
	inField := m.pwdCursor < numFields
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		m.pwdCursor = (m.pwdCursor + 1) % totalItems
		if m.pwdCursor < numFields {
			m.pwdTextCursor = len([]rune(m.pwdInputs[m.pwdCursor]))
		}
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.pwdCursor = (m.pwdCursor - 1 + totalItems) % totalItems
		if m.pwdCursor < numFields {
			m.pwdTextCursor = len([]rune(m.pwdInputs[m.pwdCursor]))
		}
		return m, nil
	case tea.KeyLeft:
		if inField {
			if m.pwdTextCursor > 0 {
				m.pwdTextCursor--
			}
		} else if m.pwdCursor > numFields {
			m.pwdCursor--
		}
		return m, nil
	case tea.KeyRight:
		if inField {
			if m.pwdTextCursor < len([]rune(m.pwdInputs[m.pwdCursor])) {
				m.pwdTextCursor++
			}
		} else if m.pwdCursor < totalItems-1 {
			m.pwdCursor++
		}
		return m, nil
	case tea.KeyBackspace:
		if inField && m.pwdTextCursor > 0 {
			r := []rune(m.pwdInputs[m.pwdCursor])
			r = append(r[:m.pwdTextCursor-1], r[m.pwdTextCursor:]...)
			m.pwdInputs[m.pwdCursor] = string(r)
			m.pwdTextCursor--
		}
		return m, nil
	case tea.KeyDelete:
		if inField {
			r := []rune(m.pwdInputs[m.pwdCursor])
			if m.pwdTextCursor < len(r) {
				r = append(r[:m.pwdTextCursor], r[m.pwdTextCursor+1:]...)
				m.pwdInputs[m.pwdCursor] = string(r)
			}
		}
		return m, nil
	case tea.KeyEsc:
		m.currentFocus = focusMenu
		return m, nil
	case tea.KeyEnter:
		if m.pwdCursor == numFields+1 {
			m.currentFocus = focusMenu
			return m, nil
		}
		return m.submitNewPassword()
	case tea.KeyRunes:
		if inField {
			r := []rune(m.pwdInputs[m.pwdCursor])
			r = append(r[:m.pwdTextCursor], append(msg.Runes, r[m.pwdTextCursor:]...)...)
			m.pwdInputs[m.pwdCursor] = string(r)
			m.pwdTextCursor += len(msg.Runes)
		} else if m.pwdCursor == numFields {
			return m.submitNewPassword()
		} else {
			m.currentFocus = focusMenu
		}
		return m, nil
	}
	return m, nil
}

func (m Model) submitNewPassword() (tea.Model, tea.Cmd) {
	var oldP, p1, p2 string
	if m.needOldPassword {
		oldP = m.pwdInputs[0]
		p1 = m.pwdInputs[1]
		p2 = m.pwdInputs[2]
	} else {
		p1 = m.pwdInputs[0]
		p2 = m.pwdInputs[1]
	}
	if (m.needOldPassword && oldP != m.passwd) || p1 != p2 {
		title := " New passwords do not match. Repeat "
		if m.needOldPassword && oldP != m.passwd {
			title = " Wrong old password. Repeat "
		}
		m.pwdInputs = [3]string{}
		m.pwdCursor = 0
		m.pwdTextCursor = 0
		m.pwdTitle = title
		return m, nil
	}
	m.passwd = p1
	m.save()
	m.pwdInputs = [3]string{}
	m.pwdCursor = 0
	m.pwdTextCursor = 0
	m.currentFocus = focusMenu
	return m, nil
}

// -----------------------------------------------------------------------
// Edit form
// -----------------------------------------------------------------------

func (m *Model) openEditForm() {
	m.editKey = ""
	m.editValues = nil
	if m.activeRow > 0 && len(m.keys) > 0 {
		m.editKey = m.keys[m.activeRow-1]
	}
	m.editVisibility = "v"
	if m.editKey != "" {
		m.editVisibility = m.visibility[m.editKey]
		m.editValues = append([]string{}, m.records[m.editKey]...)
	}
	count := max(m.width, 2)
	for len(m.editValues) <= count {
		m.editValues = append(m.editValues, "")
	}
	m.editCursor = 0
	m.editTextCursor = len([]rune(m.editKey))
	m.currentFocus = focusEdit
}

func (m *Model) editNumFields() int  { return 1 + len(m.editValues) }
func (m *Model) editTotalItems() int { return m.editNumFields() + 4 }

func (m *Model) editActiveRunes() []rune {
	if m.editCursor == 0 {
		return []rune(m.editKey)
	}
	if m.editCursor < m.editNumFields() {
		return []rune(m.editValues[m.editCursor-1])
	}
	return nil
}

func (m *Model) editSetActiveText(s string) {
	r := []rune(s)
	if m.editCursor == 0 {
		m.editKey = s
	} else if m.editCursor < m.editNumFields() {
		m.editValues[m.editCursor-1] = s
	}
	m.editTextCursor = min(m.editTextCursor, len(r))
}

func (m *Model) editMoveCursorToEnd() {
	m.editTextCursor = len(m.editActiveRunes())
}

func (m Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	numFields := m.editNumFields()
	total := m.editTotalItems()
	inField := m.editCursor < numFields

	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		m.editCursor = (m.editCursor + 1) % total
		m.editMoveCursorToEnd()
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.editCursor = (m.editCursor - 1 + total) % total
		m.editMoveCursorToEnd()
		return m, nil

	case tea.KeyLeft:
		if inField {
			if m.editTextCursor > 0 {
				m.editTextCursor--
			}
		} else {
			if m.editCursor > numFields {
				m.editCursor--
			}
		}
		return m, nil
	case tea.KeyRight:
		if inField {
			if m.editTextCursor < len(m.editActiveRunes()) {
				m.editTextCursor++
			}
		} else {
			if m.editCursor < total-1 {
				m.editCursor++
			}
		}
		return m, nil

	case tea.KeyBackspace:
		if inField && m.editTextCursor > 0 {
			r := m.editActiveRunes()
			r = append(r[:m.editTextCursor-1], r[m.editTextCursor:]...)
			m.editTextCursor--
			m.editSetActiveText(string(r))
		}
		return m, nil
	case tea.KeyDelete:
		if inField {
			r := m.editActiveRunes()
			if m.editTextCursor < len(r) {
				r = append(r[:m.editTextCursor], r[m.editTextCursor+1:]...)
				m.editSetActiveText(string(r))
			}
		}
		return m, nil

	case tea.KeyEsc:
		m.currentFocus = focusMenu
		return m, nil
	case tea.KeyEnter:
		return m.handleEditButton()
	case tea.KeyRunes:
		if inField {
			r := m.editActiveRunes()
			r = append(r[:m.editTextCursor], append(msg.Runes, r[m.editTextCursor:]...)...)
			m.editTextCursor += len(msg.Runes)
			m.editSetActiveText(string(r))
			if m.editCursor == 0 && m.editVisibility == "" {
				m.editVisibility = "v"
			}
			if m.editCursor > 0 {
				clipboard.Write(clipboard.FmtText, []byte(m.editValues[m.editCursor-1]))
			}
		} else {
			return m.handleEditButton()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleEditButton() (tea.Model, tea.Cmd) {
	numFields := m.editNumFields()
	btnIdx := m.editCursor - numFields
	switch btnIdx {
	case 0: // Submit
		if len(m.editKey) > 0 {
			v := append([]string{}, m.editValues...)
			for len(v) > 0 && len(v[len(v)-1]) == 0 {
				v = v[:len(v)-1]
			}
			keyPlace := m.updateRecord(m.editKey, v, m.editVisibility)
			m.activeRow = keyPlace + 1
			m.activeColumn = 1
			m.adjustOffset()
			m.dirty = true
		}
		m.currentFocus = focusTable
	case 1: // Hide/Reveal
		if m.editVisibility == "h" {
			m.editVisibility = "v"
		} else {
			m.editVisibility = "h"
		}
	case 2: // Clear
		m.editKey = ""
		m.editValues = make([]string, len(m.editValues))
		m.editCursor = 0
	case 3: // Cancel
		m.currentFocus = focusMenu
	}
	return m, nil
}

// -----------------------------------------------------------------------
// Mode picker
// -----------------------------------------------------------------------

func (m Model) handleModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		if m.modeCursor > 0 {
			m.modeCursor--
		}
		return m, nil
	case tea.KeyDown:
		if m.modeCursor < 3 {
			m.modeCursor++
		}
		return m, nil
	case tea.KeyEnter:
		m.mode = modeSet[m.modeCursor]
		m.modeIndex = m.modeCursor
		fallthrough
	case tea.KeyEsc:
		m.currentFocus = focusTable
		if len(m.keys) == 0 {
			m.currentFocus = focusMenu
		}
		return m, nil
	}
	return m, nil
}

// -----------------------------------------------------------------------
// Confirm overlays
// -----------------------------------------------------------------------

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	hasCancel := m.confirmCancel != ""

	doActivate := func() (tea.Model, tea.Cmd) {
		var cmd tea.Cmd
		if m.confirmCursor == 0 {
			if m.confirmYesFn != nil {
				cmd = m.confirmYesFn()
			}
			if m.currentFocus == focusConfirmDelete {
				m.dirty = true
			}
			if m.currentFocus == focusConfirmSave {
				m.dirty = false
				m.gitty = true
			}
			if m.currentFocus == focusConfirmGit {
				m.gitty = false
			}
		} else {
			if m.confirmNoFn != nil {
				cmd = m.confirmNoFn()
			}
		}
		m.currentFocus = m.confirmReturnFocus
		return m, cmd
	}

	switch msg.Type {
	case tea.KeyLeft, tea.KeyShiftTab:
		if hasCancel {
			m.confirmCursor = 0
		}
		return m, nil
	case tea.KeyRight, tea.KeyTab:
		if hasCancel {
			m.confirmCursor = 1
		}
		return m, nil
	case tea.KeyEnter, tea.KeySpace:
		return doActivate()
	case tea.KeyEsc:
		m.confirmCursor = 1
		return doActivate()
	case tea.KeyRunes:
		r := msg.Runes[0]
		switch strings.ToLower(string(r)) {
		case "y", "o":
			m.confirmCursor = 0
			return doActivate()
		case "n", "c":
			if hasCancel {
				m.confirmCursor = 1
				return doActivate()
			}
		}
	}
	return m, nil
}

// -----------------------------------------------------------------------
// Git result overlay
// -----------------------------------------------------------------------

func (m Model) handleGitResultKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.currentFocus = focusMenu
	return m, nil
}
