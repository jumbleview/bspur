package main

import (
	"strings"
	"unicode"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case msgDataLoaded:
		m.attachData(msg.data, msg.passwd, msg.alterColumn)
		m.activeRow = 1
		m.activeColumn = 1
		m.tableOffset = 0
		if m.mode == ModeVisibleSelect {
			// nothing visual to do yet; will show on render
		} else if m.mode == ModeClipSelect {
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
			// reveal handled in view; mark row as tracking colour — no special state needed
		}
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
		clipboard.WriteAll("")
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
	if len(m.keys) == 0 {
		return
	}
	switch m.mode {
	case ModeClipSelect:
		m.copyActiveToClipboard()
	case ModeVisibleSelect, ModeVisibleEnter:
		// hide/reveal handled in View via mode checks
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
		clipboard.WriteAll("")
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) activateMenuButton(label string) (tea.Model, tea.Cmd) {
	// Normalize label (Save! → Save)
	base := strings.TrimSuffix(label, "!")

	switch base {
	case "Select":
		if len(m.keys) > 0 {
			m.currentFocus = focusTable
			if m.mode == ModeVisibleSelect {
				// selection-driven reveal — nothing extra
			} else if m.mode == ModeClipSelect {
				m.copyActiveToClipboard()
			}
		}

	case firstToUpper(modeSet[m.modeIndex]):
		// Mode button
		m.modeCursor = m.modeIndex
		m.currentFocus = focusMode

	case "WWW":
		var key, url string
		if m.activeRow > 0 && len(m.keys) > 0 {
			key = m.keys[m.activeRow-1]
		}
		urlIdx := m.activeColumn - 2
		if key != "" && urlIdx >= 0 && urlIdx < len(m.records[key]) {
			url = m.records[key][urlIdx]
		}
		if len(url) > 0 {
			m.confirmText = "Go to URL:\n[" + url + "] ?"
			m.confirmOK = "Yes"
			m.confirmCancel = "No"
			capturedURL := url
			m.confirmYesFn = func() tea.Cmd {
				if !strings.HasPrefix(capturedURL, "http://") && !strings.HasPrefix(capturedURL, "https://") {
					capturedURL = "http://" + capturedURL
				}
				browser.OpenURL(capturedURL)
				return nil
			}
			m.confirmNoFn = func() tea.Cmd { return nil }
			m.currentFocus = focusConfirmWWW
		} else {
			m.confirmText = "Not valid URL\n[" + url + "]"
			m.confirmOK = "OK"
			m.confirmCancel = ""
			m.confirmYesFn = func() tea.Cmd { return nil }
			m.confirmNoFn = nil
			m.currentFocus = focusConfirmWWW
		}

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
				m.dirty = true
				m.currentFocus = focusTable
				return nil
			}
			m.confirmNoFn = func() tea.Cmd { return nil }
		} else {
			m.confirmText = "Nothing to delete. Record empty"
			m.confirmOK = "OK"
			m.confirmCancel = ""
			m.confirmYesFn = func() tea.Cmd { return nil }
			m.confirmNoFn = nil
		}
		m.currentFocus = focusConfirmDelete

	case "Save":
		m.confirmText = "Save page?"
		m.confirmOK = "Save"
		m.confirmCancel = "Cancel"
		m.confirmYesFn = func() tea.Cmd {
			m.save()
			m.currentFocus = focusMenu
			return nil
		}
		m.confirmNoFn = func() tea.Cmd {
			m.currentFocus = focusMenu
			return nil
		}
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
		m.confirmNoFn = func() tea.Cmd {
			m.currentFocus = focusMenu
			return nil
		}
		m.currentFocus = focusConfirmGit

	case "Password":
		m.pwdInputs = [3]string{}
		m.pwdCursor = 0
		m.needOldPassword = true
		m.pwdTitle = " Change page password "
		m.currentFocus = focusPasswordNew

	case "Exit":
		if !m.dirty {
			clipboard.WriteAll("")
			return m, tea.Quit
		}
		m.confirmText = "Page not saved. Exit?"
		m.confirmOK = "Exit"
		m.confirmCancel = "Cancel"
		m.confirmYesFn = func() tea.Cmd {
			clipboard.WriteAll("")
			return tea.Quit
		}
		m.confirmNoFn = func() tea.Cmd {
			m.currentFocus = focusMenu
			return nil
		}
		m.currentFocus = focusConfirmExit
	}
	return m, nil
}

// -----------------------------------------------------------------------
// Password — enter existing password
// -----------------------------------------------------------------------

func (m Model) handlePwdEnterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Single field [0], two buttons: Submit / Cancel
	// Cursor: 0=field, 1=Submit, 2=Cancel
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		m.pwdCursor = (m.pwdCursor + 1) % 3
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.pwdCursor = (m.pwdCursor - 1 + 3) % 3
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if m.pwdCursor == 0 && len(m.pwdInputs[0]) > 0 {
			m.pwdInputs[0] = m.pwdInputs[0][:len(m.pwdInputs[0])-1]
		}
		return m, nil
	case tea.KeyEsc:
		// Cancel → quit
		clipboard.WriteAll("")
		return m, tea.Quit
	case tea.KeyEnter:
		if m.pwdCursor == 2 {
			// Cancel
			clipboard.WriteAll("")
			return m, tea.Quit
		}
		// Submit (cursor==1 or cursor==0 after typing)
		return m.submitEnterPassword()
	case tea.KeyRunes:
		if m.pwdCursor == 0 {
			m.pwdInputs[0] += string(msg.Runes)
		} else if m.pwdCursor == 1 {
			// Submit
			return m.submitEnterPassword()
		} else {
			// Cancel
			clipboard.WriteAll("")
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
	// Fields: [old(optional), new1, new2], buttons: Submit / Cancel
	numFields := 2
	if m.needOldPassword {
		numFields = 3
	}
	totalItems := numFields + 2 // +Submit +Cancel
	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		m.pwdCursor = (m.pwdCursor + 1) % totalItems
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.pwdCursor = (m.pwdCursor - 1 + totalItems) % totalItems
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if m.pwdCursor < numFields {
			f := &m.pwdInputs[m.pwdCursor]
			if len(*f) > 0 {
				*f = (*f)[:len(*f)-1]
			}
		}
		return m, nil
	case tea.KeyEsc:
		m.currentFocus = focusMenu
		return m, nil
	case tea.KeyEnter:
		if m.pwdCursor == numFields+1 {
			// Cancel
			m.currentFocus = focusMenu
			return m, nil
		}
		return m.submitNewPassword()
	case tea.KeyRunes:
		if m.pwdCursor < numFields {
			m.pwdInputs[m.pwdCursor] += string(msg.Runes)
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
		m.pwdTitle = title
		return m, nil
	}
	m.passwd = p1
	m.save()
	if !m.needOldPassword {
		// creating new page — stay on menu
	}
	m.pwdInputs = [3]string{}
	m.pwdCursor = 0
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
	count := m.width
	if count < 2 {
		count = 2
	}
	// Pad editValues to count+1 (name + count fields + one extra)
	for len(m.editValues) <= count {
		m.editValues = append(m.editValues, "")
	}
	m.editCursor = 0
	m.currentFocus = focusEdit
}

// editNumFields returns 1(name) + len(editValues)
func (m *Model) editNumFields() int { return 1 + len(m.editValues) }

// editTotalItems = fields + 4 buttons (Submit, Hide/Reveal, Clear, Cancel)
func (m *Model) editTotalItems() int { return m.editNumFields() + 4 }

func (m Model) handleEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	numFields := m.editNumFields()
	total := m.editTotalItems()

	switch msg.Type {
	case tea.KeyTab, tea.KeyDown:
		m.editCursor = (m.editCursor + 1) % total
		return m, nil
	case tea.KeyShiftTab, tea.KeyUp:
		m.editCursor = (m.editCursor - 1 + total) % total
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if m.editCursor == 0 {
			if len(m.editKey) > 0 {
				m.editKey = m.editKey[:len(m.editKey)-1]
			}
		} else if m.editCursor < numFields {
			idx := m.editCursor - 1
			if len(m.editValues[idx]) > 0 {
				m.editValues[idx] = m.editValues[idx][:len(m.editValues[idx])-1]
			}
		}
		return m, nil
	case tea.KeyEsc:
		m.currentFocus = focusMenu
		return m, nil
	case tea.KeyEnter:
		return m.handleEditButton()
	case tea.KeyRunes:
		if m.editCursor == 0 {
			m.editKey += string(msg.Runes)
			if m.editVisibility == "" {
				m.editVisibility = "v"
			}
		} else if m.editCursor < numFields {
			idx := m.editCursor - 1
			m.editValues[idx] += string(msg.Runes)
			clipboard.WriteAll(m.editValues[idx])
		} else {
			return m.handleEditButton()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleEditButton() (tea.Model, tea.Cmd) {
	numFields := m.editNumFields()
	btnIdx := m.editCursor - numFields // 0=Submit,1=Hide/Reveal,2=Clear,3=Cancel
	switch btnIdx {
	case 0: // Submit
		if len(m.editKey) > 0 {
			// trim trailing empty values
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
			m.mode = modeSet[m.modeCursor]
			m.modeIndex = m.modeCursor
		}
		return m, nil
	case tea.KeyDown:
		if m.modeCursor < 3 {
			m.modeCursor++
			m.mode = modeSet[m.modeCursor]
			m.modeIndex = m.modeCursor
		}
		return m, nil
	case tea.KeyEnter, tea.KeyEsc:
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
	switch msg.Type {
	case tea.KeyEnter, tea.KeyLeft, tea.KeyRight, tea.KeyTab:
		// "Yes/OK" or "No/Cancel"
		if msg.Type == tea.KeyEnter || msg.Type == tea.KeyLeft || msg.Type == tea.KeyTab {
			if m.confirmYesFn != nil {
				cmd := m.confirmYesFn()
				m.currentFocus = focusMenu
				return m, cmd
			}
		} else {
			if m.confirmNoFn != nil {
				cmd := m.confirmNoFn()
				m.currentFocus = focusMenu
				return m, cmd
			}
		}
	case tea.KeyEsc:
		if m.confirmNoFn != nil {
			cmd := m.confirmNoFn()
			m.currentFocus = focusMenu
			return m, cmd
		}
		m.currentFocus = focusMenu
	case tea.KeyRunes:
		r := msg.Runes[0]
		switch strings.ToLower(string(r)) {
		case "y", "o", " ":
			if m.confirmYesFn != nil {
				cmd := m.confirmYesFn()
				m.currentFocus = focusMenu
				return m, cmd
			}
		case "n", "c":
			if m.confirmNoFn != nil {
				cmd := m.confirmNoFn()
				m.currentFocus = focusMenu
				return m, cmd
			}
			m.currentFocus = focusMenu
		}
	}
	return m, nil
}

// -----------------------------------------------------------------------
// Git result overlay
// -----------------------------------------------------------------------

func (m Model) handleGitResultKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Any key dismisses
	m.currentFocus = focusMenu
	return m, nil
}


