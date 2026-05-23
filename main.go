package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// -----------------------------------------------------------------------
// Flag helpers (same interface as original)
// -----------------------------------------------------------------------

type ColorValues struct {
	Colors     []lipgloss.Color
	ColorsList string
	Count      int
}

func (v *ColorValues) String() string { return v.ColorsList }

func (v *ColorValues) Set(s string) error {
	if len(s) == 0 {
		return nil
	}
	v.ColorsList = s
	vals := strings.Split(s, ",")
	for _, val := range vals {
		val = strings.TrimSpace(val)
		// lipgloss accepts tcell color names as ANSI names or hex
		v.Colors = append(v.Colors, lipgloss.Color(val))
	}
	if len(v.Colors) != v.Count {
		return fmt.Errorf("wrong number of colors: expected %d, got %d", v.Count, len(v.Colors))
	}
	return nil
}

type ModeValue struct {
	Mode  string
	Index int
}

func (v *ModeValue) String() string { return v.Mode }

func (v *ModeValue) Set(s string) error {
	if len(s) == 0 {
		return nil
	}
	for ix, mode := range modeSet {
		if mode == s {
			v.Mode = s
			v.Index = ix
			return nil
		}
	}
	return fmt.Errorf("mode %s is unknown", s)
}

// -----------------------------------------------------------------------
// main
// -----------------------------------------------------------------------

func main() {
	greeting := "tspur [-cm] [-cf] [-ct] [-md] [-ta] path_to_data_file"
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, greeting)
	}

	var mainColors ColorValues
	mainColors.Count = 2
	flag.Var(&mainColors, "cm", "Colors main: two comma-separated colors: font & background")

	var formColors ColorValues
	formColors.Count = 3
	flag.Var(&formColors, "cf", "Colors form: three comma-separated colors: font, background & input background")

	var trackingColor ColorValues
	trackingColor.Count = 1
	flag.Var(&trackingColor, "ct", "Color trace: single color for tracking font")

	var tsprMode ModeValue
	var modes []string
	for _, m := range modeSet {
		modes = append(modes, m)
	}
	flag.Var(&tsprMode, "md", "Mode: possible values are: "+strings.Join(modes, ","))

	var alterColumn int
	flag.IntVar(&alterColumn, "ta", 0, "Table altering: n>0 insert column before n; n<0 delete column -n")

	flag.Parse()
	cmd := flag.Args()
	if len(cmd) != 1 {
		fmt.Fprintf(os.Stderr, "Number of arguments: %d\n", len(cmd))
		flag.Usage()
		os.Exit(1)
	}

	// Build theme
	theme := defaultTheme
	if len(mainColors.Colors) == 2 {
		theme.MainFg = mainColors.Colors[0]
		theme.MainBg = mainColors.Colors[1]
		theme.AccentFg = mainColors.Colors[0]
	}
	if len(formColors.Colors) == 3 {
		theme.FormFg = formColors.Colors[0]
		theme.FormBg = formColors.Colors[1]
		theme.FormInputBg = formColors.Colors[2]
	}
	if len(trackingColor.Colors) == 1 {
		theme.TrackingFg = trackingColor.Colors[0]
	}

	cribName := cmd[0]
	//SetDimensions(ConsoleWidth, ConsoleHeight)

	// Resolve absolute path so cribPath is always valid
	if !filepath.IsAbs(cribName) {
		if abs, err := filepath.Abs(cribName); err == nil {
			cribName = abs
		}
	}

	m := initialModel(cribName, tsprMode.Mode, tsprMode.Index, theme, alterColumn)

	// Determine initial focus based on file existence
	_, errFile := os.Stat(cribName)
	storedAlterColumn := alterColumn // capture for the Init closure

	if errFile == nil {
		m.currentFocus = focusPasswordEnter
		m.pwdTitle = fmt.Sprintf("Enter Password: %d", storedAlterColumn)
	} else {
		_, errDir := os.Stat(filepath.Dir(cribName))
		if errDir != nil {
			clipboard.WriteAll("")
			panic(errDir)
		}
		m.currentFocus = focusPasswordNew
		m.pwdTitle = "Create new Page"
		m.needOldPassword = false
	}

	// Clipboard cleanup on SIGTERM / SIGINT
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		clipboard.WriteAll("")
		os.Exit(0)
	}()

	// We need to pass alterColumn into the password-submit handler.
	// We do this by embedding it into the model's Init, patching submitEnterPassword.
	// The cleanest Bubble Tea way: store it in the model.
	m.alterColumnAtStart = storedAlterColumn

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		clipboard.WriteAll("")
		panic(err)
	}
	clipboard.WriteAll("")
}
