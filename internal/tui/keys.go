package tui

import tea "charm.land/bubbletea/v2"

type action int

const (
	actNone action = iota
	actUp
	actDown
	actLeft
	actRight
	actMoveUp
	actMoveDown
	actToggle
	actSwitch
	actSave
	actQuit
	actYes
	actNo
)

var keymap = map[string]action{
	"up": actUp, "down": actDown,
	"left": actLeft, "right": actRight,
	"shift+up": actMoveUp, "shift+down": actMoveDown,
	"space": actToggle, "tab": actSwitch,
	"s": actSave, "q": actQuit, "ctrl+c": actQuit,
	"y": actYes, "n": actNo, "esc": actNo,
}

func actionFor(msg tea.KeyPressMsg) action { return keymap[msg.Keystroke()] }
