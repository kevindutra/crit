package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Quit         key.Binding
	Tab          key.Binding
	Up           key.Binding
	Down         key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Top          key.Binding
	Bottom       key.Binding
	NextComment  key.Binding
	PrevComment  key.Binding
	Cancel       key.Binding
	Confirm      key.Binding
	VisualMode   key.Binding
	PrevTab      key.Binding
	NextTab      key.Binding
	NextChange   key.Binding
	PrevChange   key.Binding
	TabSearch    key.Binding
}

var keys = keyMap{
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Tab:          key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "toggle sidebar")),
	Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("k", "up")),
	Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("j", "down")),
	HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u", "shift+up", "pgup"), key.WithHelp("shift+up", "half page up")),
	HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d", "shift+down", "pgdown"), key.WithHelp("shift+down", "half page down")),
	Top:          key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
	Bottom:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
	NextComment:  key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next comment")),
	PrevComment:  key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev comment")),
	Cancel:       key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	Confirm:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	VisualMode:   key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "select lines")),
	PrevTab:      key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev tab")),
	NextTab:      key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next tab")),
	NextChange:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next change")),
	PrevChange:   key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev change")),
	TabSearch:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search tabs")),
}
