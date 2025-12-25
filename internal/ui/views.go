package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) viewLaunch() string {
	logo := m.styles.logoStyle.Render(asciiLogo)

	createBtn := m.styles.buttonStyle.Render("Create Room  (c)")
	joinBtn := m.styles.buttonStyle.Render("Join Room    (J)")

	switch m.selected {
	case 0:
		createBtn = m.styles.buttonActive.Render("Create Room  (c)")
	case 1:
		joinBtn = m.styles.buttonActive.Render("Join Room    (J)")
	}

	buttons := lipgloss.JoinVertical(lipgloss.Center, createBtn, joinBtn)

	// Show active rooms if any
	if len(m.activeRooms) > 0 {
		roomsHeader := m.styles.dimStyle.Render("\n─── Active Rooms ───\n")
		var roomButtons []string
		for i, meta := range m.activeRooms {
			desc := meta.Description
			if desc == "" {
				desc = "No description"
			}
			label := fmt.Sprintf("%.8s: %s", meta.ID, truncate(desc, 20))
			if m.selected == i+2 {
				roomButtons = append(roomButtons, m.styles.buttonActive.Render(label))
			} else {
				roomButtons = append(roomButtons, m.styles.buttonStyle.Render(label))
			}
		}
		roomsList := lipgloss.JoinVertical(lipgloss.Center, roomButtons...)
		buttons = lipgloss.JoinVertical(lipgloss.Center, buttons, roomsHeader, roomsList)
	}

	help := m.styles.helpStyle.Render("↑/↓ select • enter confirm • q quit")

	content := lipgloss.JoinVertical(lipgloss.Center, logo, buttons, help)

	view := lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, content)

	return view
}

func (m *Model) viewCreate() string {
	title := m.styles.titleStyle.Render("Create Room")
	prompt := m.styles.textStyle.Render("Enter a description for your room:")
	input := m.styles.inputBoxStyle.Render(m.input.View())
	help := m.styles.helpStyle.Render("enter create • esc back")

	content := lipgloss.JoinVertical(lipgloss.Center,
		title, "", prompt, "", input, help,
	)

	view := lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, content)

	return view
}

func (m *Model) viewJoin() string {
	title := m.styles.titleStyle.Render("Join Room")
	prompt := m.styles.textStyle.Render("Enter the room ID:")
	input := m.styles.inputBoxStyle.Render(m.input.View())
	help := m.styles.helpStyle.Render("enter join • esc back")

	content := lipgloss.JoinVertical(lipgloss.Center,
		title, "", prompt, "", input, help,
	)

	view := lipgloss.Place(m.width, m.height-1, lipgloss.Center, lipgloss.Center, content)

	return view
}

func (m *Model) viewRoomCreated() string {
	title := m.styles.titleStyle.Render("Room Created!")

	// Room code box - prominent display for easy copying
	codeLabel := m.styles.dimStyle.Render("Share this code with others to join:")
	codeBox := m.styles.baseStyle.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 3).
		Bold(true).
		Foreground(colorSuccess).
		Render(m.roomID)

	hint := m.styles.dimStyle.Render("(select and copy the code above)")
	help := m.styles.helpStyle.Render("enter → enter room • esc back")

	content := lipgloss.JoinVertical(lipgloss.Center,
		title, "", codeLabel, "", codeBox, "", hint, "", help,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m *Model) viewRoom() string {
	if m.width < MinWidthForSidebar || m.height < MinHeightForSidebar {
		return m.viewResizePrompt()
	}

	// calculates widths based on sidebar visibility
	var sidebarW, terminalW, aiSidebarW int
	if m.showAISidebar {
		sidebarW = m.width / 6   // Users sidebar (narrower)
		aiSidebarW = m.width / 4 // AI sidebar
		terminalW = m.width - sidebarW - aiSidebarW - 2
	} else {
		sidebarW = m.width / 5
		terminalW = m.width - sidebarW - 1
		aiSidebarW = 0
	}

	// Always reserve same height for consistent bottom bar (never changes)
	mainHeight := m.height - 2

	sidebar := m.renderSidebar(sidebarW, mainHeight)
	terminal := m.renderTerminal(terminalW, mainHeight)

	var main string
	if m.showAISidebar {
		aiPanel := m.renderAISidebar(aiSidebarW, mainHeight)
		main = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, terminal, aiPanel)
	} else {
		main = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, terminal)
	}

	// bottom bar (vim-like): input bar or toasts
	var bottom string
	if m.inputMode != ModeNormal {
		bottom = m.renderInputBar()
	} else {
		bottom = m.renderToasts()
	}

	bottom = m.styles.bottomBarStyle.Width(m.width).Render(bottom)

	return lipgloss.JoinVertical(lipgloss.Left, main, bottom)
}

func (m *Model) renderSidebar(w, h int) string {
	var b strings.Builder

	youLabel := m.styles.dimStyle.Render("you: ")
	youName := m.styles.accentStyle.Bold(true).Render(m.username)
	b.WriteString(youLabel + youName + "\n\n")

	roomLabel := m.styles.dimStyle.Render("room: ")
	roomID := m.styles.textStyle.Render(truncate(m.roomID, w-8))
	b.WriteString(roomLabel + roomID + "\n")
	
	if m.currentRoom != nil && m.currentRoom.Description != "" {
		desc := m.currentRoom.Description
		if len(desc) > w-4 {
			desc = truncate(desc, w-4)
		}
		descText := m.styles.dimStyle.Render("      " + "\"" + desc + "\"")
		b.WriteString(descText + "\n")
	}
	b.WriteString(m.styles.dimStyle.Render(strings.Repeat("─", w-2)) + "\n\n")

	// Users
	usersLabel := m.styles.dimStyle.Render(fmt.Sprintf("connected (%d):", len(m.users)))
	b.WriteString(usersLabel + "\n")
	for _, u := range m.users {
		b.WriteString(m.styles.textStyle.Render("  • "+u) + "\n")
	}

	// Typing indicator
	if m.typingUser != "" {
		b.WriteString("\n")
		typingText := fmt.Sprintf("✎ %s is typing...", m.typingUser)
		b.WriteString(m.styles.accentStyle.Render(typingText) + "\n")
	}
	b.WriteString(m.styles.dimStyle.Render(strings.Repeat("─", w-2)) + "\n\n")

	// Keybinds
	keysLabel := m.styles.dimStyle.Render("keys:")
	b.WriteString(keysLabel + "\n")
	b.WriteString(m.styles.textStyle.Render("  ctrl+g  AI prompt") + "\n")
	b.WriteString(m.styles.textStyle.Render("  ctrl+a  toggle AI") + "\n")
	b.WriteString(m.styles.textStyle.Render("  ctrl+r  run command") + "\n")
	b.WriteString(m.styles.textStyle.Render("  ctrl+l  leave room") + "\n")

	return m.styles.sidebarStyle.Width(w).Height(h).Render(b.String())
}

func (m *Model) renderTerminal(w, h int) string {
	header := m.styles.titleStyle.Render("shared terminal")

	// Use rendered terminal content instead of viewport
	content := m.termContent
	if content == "" {
		content = m.styles.dimStyle.Render("Starting terminal...")
	}

	return m.styles.terminalStyle.Width(w).Height(h).Render(
		lipgloss.JoinVertical(lipgloss.Left, header, "", content),
	)
}

func (m *Model) renderInputBar() string {
	// Constrain input to single line to prevent height expansion
	input := m.cmdInput.View()
	if len(input) > m.width-30 {
		input = truncate(input, m.width-30)
	}

	// Add mode status on the right (fixed)
	modeText := m.getModeStatus()
	rightStyled := m.styles.accentStyle.Bold(true).Render(modeText)

	// Compute widths
	rightW := int(lipgloss.Width(rightStyled))
	leftAvail := max(m.width - rightW - 1, 0)

	// Available width for input text (no left prefix)
	inputAvail := max(leftAvail, 0)

	// Truncate input to available runes
	if len(input) > inputAvail {
		input = truncate(input, inputAvail)
	}

	// Build left side: input (plain)
	left := input

	// Pad between left and right to ensure fixed width
	padCount := max(m.width - int(lipgloss.Width(left)) - rightW, 0)
	return left + strings.Repeat(" ", padCount) + rightStyled
}

func (m *Model) renderToasts() string {
	if len(m.toasts) == 0 {
		// vim like mode display on right
		modeText := m.getModeStatus()
		rightStyled := m.styles.accentStyle.Bold(true).Render(modeText)

		// default on left side of bar
		helpText := "ctrl+g AI • ctrl+a toggle AI • ctrl+r sandbox"
		
		// available width for left plain help (account for right width)
		avail := max(m.width - int(lipgloss.Width(rightStyled)) - 1, 0)
		leftPlain := truncate(helpText, avail)
		leftStyled := m.styles.dimStyle.Render(leftPlain)
		padCount := max(m.width - int(lipgloss.Width(leftStyled)) - int(lipgloss.Width(rightStyled)), 0)
		return leftStyled + strings.Repeat(" ", padCount) + rightStyled
	}

	var parts []string
	for _, t := range m.toasts {
		parts = append(parts, t.text)
	}
	toastText := "▸ " + strings.Join(parts, " • ")
	return m.styles.accentStyle.Bold(true).Render(toastText)
}



func (m *Model) getModeStatus() string {
	switch m.inputMode {
	case ModeAI:
		return "-- AI --"
	case ModeSandbox:
		return "-- RUN --"
	default:
		return "-- NORMAL --"
	}
}

func (m *Model) viewResizePrompt() string {
	title := m.styles.titleStyle.Render("Terminal Too Small")
	msg := m.styles.textStyle.Render(fmt.Sprintf(
		"Please resize your terminal to at least %dx%d",
		MinWidthForSidebar, MinHeightForSidebar,
	))
	current := m.styles.dimStyle.Render(fmt.Sprintf("Current: %dx%d", m.width, m.height))
	hint := m.styles.dimStyle.Render("(or press ctrl+a to hide AI sidebar)")

	content := lipgloss.JoinVertical(lipgloss.Center,
		title, "", msg, current, "", hint,
	)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m *Model) renderAISidebar(w, h int) string {
	var b strings.Builder

	// Header
	header := m.styles.titleStyle.Render("AI Assistant")
	b.WriteString(header + "\n")
	b.WriteString(m.styles.dimStyle.Render("─────────────────────") + "\n\n")

	// calculate available height for messages
	msgHeight := h - 6

	// show loading spinner if waiting for response
	if m.aiLoading {
		loadingText := fmt.Sprintf("%s Thinking...", m.aiSpinner.View())
		b.WriteString(m.styles.accentStyle.Render(loadingText))
	} else if len(m.aiMessages) == 0 {
		// Empty state
		emptyMsg := m.styles.dimStyle.Render("No messages yet.\nPress ctrl+g to ask AI.")
		b.WriteString(emptyMsg)
	} else {
		// Render messages, showing most recent that fit
		messages := m.formatAIMessages(w-4, msgHeight)
		b.WriteString(messages)
	}

	return m.styles.aiSidebarStyle.Width(w).Height(h).Render(b.String())
}

func (m *Model) formatAIMessages(maxWidth, maxLines int) string {
	var lines []string

	for _, msg := range m.aiMessages {
		var prefix, style string
		if msg.Role == "user" {
			// Show username for user messages
			username := msg.UserID
			if username == "" {
				username = "you"
			}
			prefix = m.styles.accentStyle.Render(username + ": ")
			style = "user"
		} else {
			prefix = m.styles.dimStyle.Render("AI: ")
			style = "agent"
		}

		// Word wrap the message text
		wrapped := m.wrapText(msg.Text, maxWidth-4)
		wrappedLines := strings.Split(wrapped, "\n")

		for i, line := range wrappedLines {
			if i == 0 {
				if style == "user" {
					lines = append(lines, prefix+m.styles.accentStyle.Render(line))
				} else {
					lines = append(lines, prefix+m.styles.textStyle.Render(line))
				}
			} else {
				// Continuation lines - indent to align with text
				indent := "    "
				if style == "user" {
					lines = append(lines, indent+m.styles.accentStyle.Render(line))
				} else {
					lines = append(lines, indent+m.styles.textStyle.Render(line))
				}
			}
		}
		lines = append(lines, "") // Blank line between messages
	}

	// Show only the last N lines that fit
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return strings.Join(lines, "\n")
}

func (m *Model) wrapText(text string, width int) string {
	if width <= 0 {
		width = 40
	}

	var result strings.Builder
	words := strings.Fields(text)
	lineLen := 0

	for i, word := range words {
		wordLen := len(word)

		if lineLen+wordLen+1 > width && lineLen > 0 {
			result.WriteString("\n")
			lineLen = 0
		}

		if lineLen > 0 {
			result.WriteString(" ")
			lineLen++
		}

		result.WriteString(word)
		lineLen += wordLen

		// Handle newlines in original text
		if i < len(words)-1 && strings.Contains(text, "\n") {
			// Check if there was a newline between this word and next
			idx := strings.Index(text, word)
			if idx >= 0 {
				afterWord := text[idx+len(word):]
				if len(afterWord) > 0 && afterWord[0] == '\n' {
					result.WriteString("\n")
					lineLen = 0
				}
			}
		}
	}

	return result.String()
}
