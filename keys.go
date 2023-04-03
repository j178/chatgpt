package main

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
)

type keyMap struct {
	SwitchMultiline    key.Binding
	Submit             key.Binding
	ShowHelp           key.Binding
	HideHelp           key.Binding
	Quit               key.Binding
	Copy               key.Binding
	PrevHistory        key.Binding
	NextHistory        key.Binding
	NewConversation    key.Binding
	PrevConversation   key.Binding
	NextConversation   key.Binding
	RemoveConversation key.Binding
	ForgetContext      key.Binding
	ViewPortKeys       viewport.KeyMap
	TextAreaKeys       textarea.KeyMap
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.ShowHelp, k.Submit, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.HideHelp, k.Submit, k.Quit, k.SwitchMultiline, k.Copy},
		{k.NewConversation, k.PrevConversation, k.NextConversation, k.ForgetContext, k.RemoveConversation},
		{
			k.PrevHistory,
			k.NextHistory,
			k.ViewPortKeys.Up,
			k.ViewPortKeys.Down,
			k.ViewPortKeys.PageUp,
			k.ViewPortKeys.PageDown,
		},
	}
}

func defaultKeyMap() keyMap {
	return keyMap{
		SwitchMultiline: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "multiline mode")),
		Submit:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit")),
		ShowHelp:        key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "show help")),
		HideHelp:        key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "hide help")),
		Quit:            key.NewBinding(key.WithKeys("esc", "ctrl+c"), key.WithHelp("esc", "quit")),
		Copy:            key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "copy last answer")),
		PrevHistory:     key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "previous question")),
		NextHistory:     key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "next question")),
		NewConversation: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "new conversation")),
		ForgetContext:   key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("ctrl+x", "forget context")),
		RemoveConversation: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "remove current conversation"),
		),
		PrevConversation: key.NewBinding(
			key.WithKeys("ctrl+left"),
			key.WithHelp("ctrl+left", "previous conversation"),
		),
		NextConversation: key.NewBinding(
			key.WithKeys("ctrl+right"),
			key.WithHelp("ctrl+right", "next conversation"),
		),
		ViewPortKeys: viewport.KeyMap{
			PageDown: key.NewBinding(
				key.WithKeys("pgdown"),
				key.WithHelp("pgdn", "page down"),
			),
			PageUp: key.NewBinding(
				key.WithKeys("pgup"),
				key.WithHelp("pgup", "page up"),
			),
			HalfPageUp:   key.NewBinding(key.WithDisabled()),
			HalfPageDown: key.NewBinding(key.WithDisabled()),
			Up: key.NewBinding(
				key.WithKeys("up"),
				key.WithHelp("↑", "up"),
			),
			Down: key.NewBinding(
				key.WithKeys("down"),
				key.WithHelp("↓", "down"),
			),
		},
		TextAreaKeys: textarea.KeyMap{
			CharacterForward:        key.NewBinding(key.WithKeys("right", "ctrl+f")),
			CharacterBackward:       key.NewBinding(key.WithKeys("left", "ctrl+b")),
			WordForward:             key.NewBinding(key.WithKeys("alt+right", "alt+f")),
			WordBackward:            key.NewBinding(key.WithKeys("alt+left", "alt+b")),
			LineNext:                key.NewBinding(key.WithKeys("down")),
			LinePrevious:            key.NewBinding(key.WithKeys("up")),
			DeleteWordBackward:      key.NewBinding(key.WithKeys("alt+backspace", "ctrl+w")),
			DeleteWordForward:       key.NewBinding(key.WithKeys("alt+delete", "alt+d")),
			DeleteAfterCursor:       key.NewBinding(key.WithKeys("ctrl+k")),
			DeleteBeforeCursor:      key.NewBinding(key.WithKeys("ctrl+u")),
			InsertNewline:           key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "insert new line")),
			DeleteCharacterBackward: key.NewBinding(key.WithKeys("backspace")),
			DeleteCharacterForward:  key.NewBinding(key.WithKeys("delete")),
			LineStart:               key.NewBinding(key.WithKeys("home", "ctrl+a")),
			LineEnd:                 key.NewBinding(key.WithKeys("end", "ctrl+e")),
			Paste:                   key.NewBinding(key.WithKeys("ctrl+v", "alt+v")),
			InputBegin:              key.NewBinding(key.WithKeys("alt+<", "ctrl+home")),
			InputEnd:                key.NewBinding(key.WithKeys("alt+>", "ctrl+end")),

			CapitalizeWordForward: key.NewBinding(key.WithKeys("alt+c")),
			LowercaseWordForward:  key.NewBinding(key.WithKeys("alt+l")),
			UppercaseWordForward:  key.NewBinding(key.WithKeys("alt+u")),

			TransposeCharacterBackward: key.NewBinding(key.WithDisabled()),
		},
	}
}

type InputMode int

const (
	InputModelSingleLine InputMode = iota
	InputModelMultiLine
)

func UseSingleLineInputMode(m *model) {
	m.inputMode = InputModelSingleLine
	m.keymap.SwitchMultiline = key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "multiline mode"))
	m.keymap.Submit = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "submit"))
	m.keymap.TextAreaKeys.InsertNewline = key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "insert new line"),
	)
	m.viewport.KeyMap = m.keymap.ViewPortKeys
	m.textarea.KeyMap = m.keymap.TextAreaKeys
}

func UseMultiLineInputMode(m *model) {
	m.inputMode = InputModelMultiLine
	m.keymap.SwitchMultiline = key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "single line mode"))
	m.keymap.Submit = key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "submit"))
	m.keymap.TextAreaKeys.InsertNewline = key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "insert new line"),
	)
	m.viewport.KeyMap = m.keymap.ViewPortKeys
	m.textarea.KeyMap = m.keymap.TextAreaKeys
}
