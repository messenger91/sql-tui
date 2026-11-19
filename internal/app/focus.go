package app

// Focus identifies the pane that currently receives keys.
type Focus int

const (
	FocusWorkspace Focus = iota
	FocusQuery
	FocusResult
)

// String returns the label shown in the status bar.
func (f Focus) String() string {
	switch f {
	case FocusWorkspace:
		return "workspace"
	case FocusQuery:
		return "query"
	case FocusResult:
		return "results"
	default:
		return "unknown"
	}
}

// Next cycles focus forwards: workspace -> query -> results -> workspace.
func (f Focus) Next() Focus {
	switch f {
	case FocusWorkspace:
		return FocusQuery
	case FocusQuery:
		return FocusResult
	default:
		return FocusWorkspace
	}
}

// Prev cycles focus backwards.
func (f Focus) Prev() Focus {
	switch f {
	case FocusWorkspace:
		return FocusResult
	case FocusQuery:
		return FocusWorkspace
	default:
		return FocusQuery
	}
}
