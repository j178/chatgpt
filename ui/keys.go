package ui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/j178/chatgpt"
)

type InputMode int

const (
	InputModelSingleLine InputMode = iota
	InputModelMultiLine
)

func newBinding(keys []string, help string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(keys[0], help))
}

type keyMap struct {
	SwitchMultiline    key.Binding
	Submit             key.Binding
	ToggleHelp         key.Binding
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
	return []key.Binding{k.ToggleHelp}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Submit, k.Quit, k.SwitchMultiline, k.Copy, k.TextAreaKeys.Paste},
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

func newKeyMap(conf chatgpt.KeyMapConfig) keyMap {
	return keyMap{
		SwitchMultiline:    newBinding(conf.SwitchMultiline, "multiline mode"),
		Submit:             newBinding(conf.Submit, "submit"),
		ToggleHelp:         newBinding(conf.Help, "toggle help"),
		Quit:               newBinding(conf.Quit, "quit"),
		Copy:               newBinding(conf.CopyLastAnswer, "copy last answer"),
		PrevHistory:        newBinding(conf.PreviousQuestion, "previous question"),
		NextHistory:        newBinding(conf.NextQuestion, "next question"),
		NewConversation:    newBinding(conf.NewConversation, "new conversation"),
		ForgetContext:      newBinding(conf.ForgetContext, "forget context"),
		RemoveConversation: newBinding(conf.RemoveConversation, "remove current conversation"),
		PrevConversation:   newBinding(conf.PreviousConversation, "previous conversation"),
		NextConversation:   newBinding(conf.NextConversation, "next conversation"),
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
			Paste:                   key.NewBinding(key.WithKeys("ctrl+v", "alt+v"), key.WithHelp("ctrl+v", "paste")),
			InputBegin:              key.NewBinding(key.WithKeys("alt+<", "ctrl+home")),
			InputEnd:                key.NewBinding(key.WithKeys("alt+>", "ctrl+end")),

			CapitalizeWordForward: key.NewBinding(key.WithKeys("alt+c")),
			LowercaseWordForward:  key.NewBinding(key.WithKeys("alt+l")),
			UppercaseWordForward:  key.NewBinding(key.WithKeys("alt+u")),

			TransposeCharacterBackward: key.NewBinding(key.WithDisabled()),
		},
	}
}
