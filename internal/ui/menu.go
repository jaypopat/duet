package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type MenuChoice int

const (
	ChoiceNone MenuChoice = iota
	ChoiceCreate
	ChoiceJoin
)

type MenuModel struct {
	choice   int
	selected MenuChoice
	roomID   string
	inputMode bool
	input    string
}

var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginBottom(1)

	optionStyle = lipgloss.NewStyle().
		PaddingLeft(2).
		MarginTop(1)

	selectedStyle = lipgloss.NewStyle().
		PaddingLeft(1).
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		MarginTop(2)
)

func NewMenuModel() MenuModel {
	return MenuModel{
		choice:   0,
		selected: ChoiceNone,
		inputMode: false,
		input:    "",
	}
}

func (m MenuModel) Init() tea.Cmd {
	return nil
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.inputMode {
			switch msg.String() {
			case "enter":
				m.roomID = m.input
				m.selected = ChoiceJoin
				return m, tea.Quit
			case "esc":
				m.inputMode = false
				m.input = ""
			case "backspace":
				if len(m.input) > 0 {
					m.input = m.input[:len(m.input)-1]
				}
			default:
				if len(msg.String()) == 1 {
					m.input += msg.String()
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.choice > 0 {
				m.choice--
			}
		case "down", "j":
			if m.choice < 1 {
				m.choice++
			}
		case "1":
			m.choice = 0
			m.selected = ChoiceCreate
			return m, tea.Quit
		case "2":
			m.choice = 1
			m.inputMode = true
		case "enter":
			if m.choice == 0 {
				m.selected = ChoiceCreate
				return m, tea.Quit
			} else {
				m.inputMode = true
			}
		}
	}
	return m, nil
}

func (m MenuModel) View() string {
	if m.inputMode {
		return fmt.Sprintf(
			"%s\n\n%s\n> %s\n\n%s",
			titleStyle.Render("🎯 Duet - SSH Pair Programming"),
			"Enter room ID to join:",
			m.input+"█",
			helpStyle.Render("Press ESC to go back • ENTER to join"),
		)
	}

	option1 := optionStyle.Render("1. Create new session")
	option2 := optionStyle.Render("2. Join existing session")

	if m.choice == 0 {
		option1 = selectedStyle.Render("▸ 1. Create new session")
	} else {
		option2 = selectedStyle.Render("▸ 2. Join existing session")
	}

	return fmt.Sprintf(
		"%s\n\n%s\n%s\n\n%s",
		titleStyle.Render("🎯 Duet - SSH Pair Programming"),
		option1,
		option2,
		helpStyle.Render("↑/↓ or j/k to navigate • 1/2 or ENTER to select • q to quit"),
	)
}

func (m MenuModel) GetChoice() MenuChoice {
	return m.selected
}

func (m MenuModel) GetRoomID() string {
	return m.roomID
}
