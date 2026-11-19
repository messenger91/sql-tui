package app

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Cycle      key.Binding
	CyclePrev  key.Binding
	Help       key.Binding
	Quit       key.Binding
	Close      key.Binding
	Export     key.Binding
	ViewFormat key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Cycle: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next pane"),
		),
		CyclePrev: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev pane"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		Close: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close"),
		),
		Export: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "export"),
		),
		ViewFormat: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "view"),
		),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Cycle, k.Export, k.ViewFormat, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Cycle, k.CyclePrev},
		{k.Export, k.ViewFormat},
		{k.Help, k.Close},
		{k.Quit},
	}
}
