package ui

import "github.com/jaypopat/duet/internal/room"

// Screen represents which screen is currently active
type Screen int

const (
	ScreenLaunch Screen = iota
	ScreenCreate
	ScreenJoin
	ScreenRoomCreated // Shows room code for copying before entering room
	ScreenRoom
)

// InputMode represents the input mode in the room screen
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeAI
	ModeSandbox
)

// Navigation messages

type GotoScreenMsg struct {
	Screen Screen
}

type RoomCreatedMsg struct {
	RoomID string
	Room   *room.Room
}

type RoomJoinedMsg struct {
	RoomID string
	Room   *room.Room
}

// Toast/notification messages

type ToastMsg struct {
	Text string
}

type ErrorMsg struct {
	Err error
}

// AI and sandbox messages

type AIResponseMsg struct {
	Reply    string
	Messages []AIMessage // Full conversation history from server
}

type AIMessagesMsg struct {
	Messages []AIMessage // AI messages received from another user
}

type SandboxResultMsg struct {
	Output string
	Cmd    string
}

// Timer messages

type tickMsg struct{}

// Terminal messages

type terminalUpdateMsg struct{}

// Room event message (from event channel)
type roomEventMsg struct {
	Event room.RoomEvent
}
