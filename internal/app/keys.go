package app

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Cycle        key.Binding
	CyclePrev    key.Binding
	Help         key.Binding
	Quit         key.Binding
	Close        key.Binding
	Export       key.Binding
	ViewFormat   key.Binding
	SaveFav      key.Binding
	BrowseFav    key.Binding
	BrowseSchema key.Binding
	Explain      key.Binding
	Command      key.Binding
	NewTab       key.Binding
	CloseTab     key.Binding
	NextTab      key.Binding
	PrevTab      key.Binding
	JumpTab      key.Binding
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
		SaveFav: key.NewBinding(
			key.WithKeys("ctrl+s"),
			key.WithHelp("ctrl+s", "save favorite"),
		),
		BrowseFav: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "favorites"),
		),
		BrowseSchema: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("ctrl+b", "schema"),
		),
		Explain: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "explain"),
		),
		Command: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "menu"),
		),
		NewTab: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "new tab"),
		),
		CloseTab: key.NewBinding(
			key.WithKeys("ctrl+w"),
			key.WithHelp("ctrl+w", "close tab"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("ctrl+right"),
			key.WithHelp("ctrl+→", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("ctrl+left"),
			key.WithHelp("ctrl+←", "prev tab"),
		),
		JumpTab: key.NewBinding(
			key.WithKeys("alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6", "alt+7", "alt+8", "alt+9"),
			key.WithHelp("alt+1..9", "jump tab"),
		),
	}
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Cycle, k.NewTab, k.Export, k.ViewFormat, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Cycle, k.CyclePrev},
		{k.NewTab, k.CloseTab, k.NextTab, k.PrevTab, k.JumpTab},
		{k.Export, k.ViewFormat},
		{k.SaveFav, k.BrowseFav, k.BrowseSchema, k.Explain},
		{k.Command},
		{k.Help, k.Close},
		{k.Quit},
	}
}
