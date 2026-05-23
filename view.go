package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// -----------------------------------------------------------------------
// View entry point
// -----------------------------------------------------------------------

func (m Model) View() string {
	switch m.currentFocus {
	case focusPasswordEnter:
		return m.viewOverlay(m.viewPasswordEnter())
	case focusPasswordNew:
		return m.viewOverlay(m.viewPasswordNew())
	case focusEdit:
		return m.viewOverlay(m.viewEditForm())
	case focusMode:
		return m.viewOverlay(m.viewModePicker())
	case focusConfirmWWW, focusConfirmDelete, focusConfirmSave, focusConfirmExit, focusConfirmGit:
		return m.viewOverlay(m.viewConfirm())
	case focusGitResult:
		return m.viewOverlay(m.viewGitResult())
	default:
		return m.viewMain()
	}
}

// -----------------------------------------------------------------------
// Main layout: top menu + table
// -----------------------------------------------------------------------

func (m Model) viewMain() string {
	menu := m.viewTopMenu()
	table := m.viewTable()
	return lipgloss.JoinVertical(lipgloss.Left, menu, table)
}

// -----------------------------------------------------------------------
// Top menu
// -----------------------------------------------------------------------

func (m Model) viewTopMenu() string {
	btns := m.menuButtons()
	var parts []string
	for i, label := range btns {
		s := lipgloss.NewStyle().
			Foreground(m.AccentFg).
			Background(m.MainBg).
			Padding(0, 1)
		if m.currentFocus == focusMenu && i == m.menuIndex {
			s = s.Reverse(true)
		}
		parts = append(parts, s.Render("[ "+label+" ]"))
	}
	bar := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return lipgloss.NewStyle().
		Background(m.MainBg).
		Width(m.termWidth).
		Render(bar)
}

// -----------------------------------------------------------------------
// Table
// -----------------------------------------------------------------------

func (m Model) viewTable() string {
	numDataCols := m.numDataCols()
	totalCols := numDataCols + 2 // col0=row#, col1=Name, col2..=values

	// Column widths
	colWidths := make([]int, totalCols)
	colWidths[0] = 6  // row number
	colWidths[1] = 20 // Record Name
	for c := 2; c < totalCols; c++ {
		colWidths[c] = len(fmt.Sprintf(FmtFieldTitle, c-1))
	}

	border := lipgloss.NewStyle().Foreground(m.AccentFg)
	hdrFg := m.AccentFg
	cellFg := m.MainFg
	cellBg := m.MainBg

	renderRow := func(cells []string, widths []int, fg lipgloss.Color, bg lipgloss.Color, selected bool, selCol int) string {
		var cols []string
		for i, cell := range cells {
			w := widths[i]
			text := cell
			if len(text) > w {
				text = text[:w]
			}
			text = centerPad(text, w)
			style := lipgloss.NewStyle().Foreground(fg).Background(bg)
			if selected && i == selCol {
				style = style.Reverse(true)
			}
			cols = append(cols, style.Render(text))
		}
		return "│" + strings.Join(cols, "│") + "│"
	}

	// Header row
	hdrCells := make([]string, totalCols)
	hdrCells[0] = fmt.Sprintf("%d Rows", len(m.records))
	hdrCells[1] = "Record Name"
	for c := 2; c < totalCols; c++ {
		hdrCells[c] = fmt.Sprintf(FmtFieldTitle, c-1)
	}
	hdr := renderRow(hdrCells, colWidths, hdrFg, m.MainBg, false, -1)

	// Separator
	sep := "├"
	for c, w := range colWidths {
		sep += strings.Repeat("─", w)
		if c < totalCols-1 {
			sep += "┼"
		}
	}
	sep += "┤"
	_ = border

	// Top border
	top := "┌"
	for c, w := range colWidths {
		top += strings.Repeat("─", w)
		if c < totalCols-1 {
			top += "┬"
		}
	}
	top += "┐"

	// Bottom border
	bot := "└"
	for c, w := range colWidths {
		bot += strings.Repeat("─", w)
		if c < totalCols-1 {
			bot += "┴"
		}
	}
	bot += "┘"

	var lines []string
	lines = append(lines, top)
	lines = append(lines, hdr)
	lines = append(lines, sep)

	vis := m.visibleRows()
	end := m.tableOffset + vis
	if end > len(m.keys) {
		end = len(m.keys)
	}

	for r := m.tableOffset; r < end; r++ {
		dataRow := r + 1 // 1-based
		key := m.keys[r]
		values := m.records[key]
		vis2 := m.visibility[key]

		cells := make([]string, totalCols)
		cells[0] = fmt.Sprintf("%d", dataRow)
		cells[1] = key

		for c := 2; c < totalCols; c++ {
			idx := c - 2
			val := ""
			if idx < len(values) {
				val = values[idx]
			}
			if vis2 == "h" && len(val) > 0 {
				// Show real value when show-on-select/enter and this is selected cell
				if (m.mode == ModeVisibleSelect || (m.mode == ModeVisibleEnter && m.currentFocus == focusTable)) &&
					dataRow == m.activeRow && c == m.activeColumn {
					// show real
				} else {
					val = hiddenText
				}
			}
			cells[c] = val
		}

		isSelected := m.currentFocus == focusTable && dataRow == m.activeRow
		selCol := m.activeColumn // 1-based, but renderRow uses 0-based index

		// Tracking color for Enter-activated cells
		rowFg := cellFg
		if isSelected {
			rowFg = m.TrackingFg
		}

		row := renderRow(cells, colWidths, rowFg, cellBg, isSelected, selCol)
		lines = append(lines, row)
	}

	// Fill remaining rows if table is short
	emptyRow := "│"
	for c, w := range colWidths {
		emptyRow += strings.Repeat(" ", w)
		if c < totalCols-1 {
			emptyRow += "│"
		}
	}
	emptyRow += "│"
	for len(lines)-3 < vis { // -3 for top/hdr/sep
		lines = append(lines, emptyRow)
	}

	lines = append(lines, bot)

	tbl := strings.Join(lines, "\n")
	return lipgloss.NewStyle().Foreground(m.MainFg).Background(m.MainBg).Render(tbl)
}

func centerPad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	total := width - len(s)
	left := total / 2
	right := total - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// -----------------------------------------------------------------------
// Overlay wrapper — centers a box on top of dimmed background
// -----------------------------------------------------------------------

func (m Model) viewOverlay(box string) string {
	bg := m.viewMain()
	lines := strings.Split(bg, "\n")
	boxLines := strings.Split(box, "\n")

	boxH := len(boxLines)
	boxW := 0
	for _, l := range boxLines {
		if lipgloss.Width(l) > boxW {
			boxW = lipgloss.Width(l)
		}
	}

	startY := (m.termHeight - boxH) / 2
	startX := (m.termWidth - boxW) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i, bl := range boxLines {
		y := startY + i
		if y >= len(lines) {
			lines = append(lines, "")
		}
		line := lines[y]
		// pad line to startX
		for lipgloss.Width(line) < startX {
			line += " "
		}
		// splice box line in
		before := truncateToWidth(line, startX)
		lines[y] = before + bl
	}

	return strings.Join(lines, "\n")
}

func truncateToWidth(s string, width int) string {
	runes := []rune(s)
	result := ""
	w := 0
	for _, r := range runes {
		if w >= width {
			break
		}
		result += string(r)
		w++
	}
	for w < width {
		result += " "
		w++
	}
	return result
}

// -----------------------------------------------------------------------
// Password — enter existing password overlay
// -----------------------------------------------------------------------

func (m Model) viewPasswordEnter() string {
	title := m.pwdTitle
	fieldLabel := "Password:"
	fieldVal := strings.Repeat("*", len(m.pwdInputs[0]))

	fStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormInputBg).Width(24)
	lStyle := lipgloss.NewStyle().Foreground(m.FormFg)

	fActive := fStyle
	if m.pwdCursor != 0 {
		fActive = fStyle
	} else {
		fActive = fStyle.Reverse(true)
	}

	field := lStyle.Render(fieldLabel) + " " + fActive.Render(fieldVal+"_")

	submitStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
	cancelStyle := submitStyle
	if m.pwdCursor == 1 {
		submitStyle = submitStyle.Reverse(true)
	}
	if m.pwdCursor == 2 {
		cancelStyle = cancelStyle.Reverse(true)
	}
	btns := lipgloss.JoinHorizontal(lipgloss.Top, submitStyle.Render("[ Submit ]"), "  ", cancelStyle.Render("[ Cancel ]"))

	content := lipgloss.JoinVertical(lipgloss.Left,
		"",
		"  "+field,
		"",
		"  "+btns,
		"",
	)

	return m.boxStyle(40, title).Render(content)
}

// -----------------------------------------------------------------------
// Password — new/change password overlay
// -----------------------------------------------------------------------

func (m Model) viewPasswordNew() string {
	numFields := 2
	if m.needOldPassword {
		numFields = 3
	}

	labels := []string{"New Password:", "New Password:"}
	if m.needOldPassword {
		labels = []string{"Old Password:", "New Password:", "New Password:"}
	}
	inputs := m.pwdInputs[:numFields]

	fStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormInputBg).Width(20)
	lStyle := lipgloss.NewStyle().Foreground(m.FormFg)

	var fieldLines []string
	for i, lbl := range labels {
		val := strings.Repeat("*", len(inputs[i]))
		fs := fStyle
		if m.pwdCursor == i {
			fs = fs.Reverse(true)
		}
		fieldLines = append(fieldLines, "  "+lStyle.Render(lbl)+" "+fs.Render(val+"_"))
	}

	submitStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
	cancelStyle := submitStyle
	if m.pwdCursor == numFields {
		submitStyle = submitStyle.Reverse(true)
	}
	if m.pwdCursor == numFields+1 {
		cancelStyle = cancelStyle.Reverse(true)
	}
	btns := lipgloss.JoinHorizontal(lipgloss.Top, submitStyle.Render("[ Submit ]"), "  ", cancelStyle.Render("[ Cancel ]"))

	lines := append([]string{""}, fieldLines...)
	lines = append(lines, "", "  "+btns, "")

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	return m.boxStyle(44, m.pwdTitle).Render(content)
}

// -----------------------------------------------------------------------
// Edit form overlay
// -----------------------------------------------------------------------

func (m Model) viewEditForm() string {
	fStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormInputBg).Width(28)
	lStyle := lipgloss.NewStyle().Foreground(m.FormFg)

	renderField := func(label, val string, cursor bool, hidden bool) string {
		display := val
		if hidden {
			display = strings.Repeat("*", len(val))
		}
		fs := fStyle
		if cursor {
			fs = fs.Reverse(true)
		}
		return "  " + lStyle.Render(label) + " " + fs.Render(display+"_")
	}

	var lines []string
	lines = append(lines, "")
	lines = append(lines, renderField("Record Name", m.editKey, m.editCursor == 0, false))

	isHidden := m.editVisibility == "h"
	for i, val := range m.editValues {
		label := fmt.Sprintf("Field %d   ", i+1)
		if i == len(m.editValues)-1 {
			label = "+          "
		}
		lines = append(lines, renderField(label, val, m.editCursor == i+1, isHidden))
	}

	numFields := m.editNumFields()
	btnLabels := []string{"Submit", "Hide/Reveal", "Clear", "Cancel"}
	var btns []string
	for i, bl := range btnLabels {
		s := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)
		if m.editCursor == numFields+i {
			s = s.Reverse(true)
		}
		btns = append(btns, s.Render("[ "+bl+" ]"))
	}
	lines = append(lines, "")
	lines = append(lines, "  "+lipgloss.JoinHorizontal(lipgloss.Top, btns...))
	lines = append(lines, "")

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	h := len(lines) + 2
	return m.boxStyle(56, " Edit Record ").Height(h).Render(content)
}

// -----------------------------------------------------------------------
// Mode picker overlay
// -----------------------------------------------------------------------

func (m Model) viewModePicker() string {
	var lines []string
	lines = append(lines, "")
	for i, mode := range modeSet {
		s := lipgloss.NewStyle().
			Foreground(m.FormFg).
			Background(m.FormBg).
			Width(len(ModeClipSelect) + 2).
			Align(lipgloss.Center)
		if i == m.modeCursor {
			s = s.Reverse(true)
		}
		lines = append(lines, "  "+s.Render(mode))
	}
	lines = append(lines, "")
	content := lipgloss.JoinVertical(lipgloss.Right, lines...)
	return m.boxStyle(len(ModeClipSelect)+6, " Mode: ").Render(content)
}

// -----------------------------------------------------------------------
// Confirm overlay
// -----------------------------------------------------------------------

func (m Model) viewConfirm() string {
	msgStyle := lipgloss.NewStyle().Foreground(m.FormFg)
	btnStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)

	okBtn := btnStyle.Render("[ " + m.confirmOK + " ]")
	var btns string
	if m.confirmCancel != "" {
		cancelBtn := btnStyle.Render("[ " + m.confirmCancel + " ]")
		btns = lipgloss.JoinHorizontal(lipgloss.Top, okBtn, "  ", cancelBtn)
	} else {
		btns = okBtn
	}

	msgLines := strings.Split(m.confirmText, "\n")
	var lines []string
	lines = append(lines, "")
	for _, ml := range msgLines {
		lines = append(lines, "  "+msgStyle.Render(ml))
	}
	lines = append(lines, "")
	lines = append(lines, "  "+btns)
	lines = append(lines, "")

	content := lipgloss.JoinVertical(lipgloss.Left, lines...)
	w := 36
	for _, l := range msgLines {
		if len(l)+6 > w {
			w = len(l) + 6
		}
	}
	return m.boxStyle(w, "").Render(content)
}

// -----------------------------------------------------------------------
// Git result overlay
// -----------------------------------------------------------------------

func (m Model) viewGitResult() string {
	msgStyle := lipgloss.NewStyle().Foreground(m.FormFg)
	btnStyle := lipgloss.NewStyle().Foreground(m.FormFg).Background(m.FormBg).Padding(0, 1)

	content := lipgloss.JoinVertical(lipgloss.Left,
		"",
		"  "+msgStyle.Render(m.gitResultText),
		"",
		"  "+btnStyle.Render("[ OK ]"),
		"",
	)
	return m.boxStyle(len(m.gitResultText)+8, " Git ").Render(content)
}

// -----------------------------------------------------------------------
// Box style helper
// -----------------------------------------------------------------------

func (m Model) boxStyle(width int, title string) lipgloss.Style {
	s := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(m.AccentFg).
		Background(m.FormBg).
		Foreground(m.FormFg).
		Width(width)
	if title != "" {
		s = s.BorderTop(true).BorderBottom(true).BorderLeft(true).BorderRight(true)
	}
	return s
}
