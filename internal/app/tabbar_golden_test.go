package app

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func checkGolden(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(want) != content {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, content)
	}
}

func TestGoldenTabBarMixedStates(t *testing.T) {
	m := newSized(t, 80, 24)

	first := m.activeSession()
	first.Title = "SELECT 1"
	first.MarkDone()

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	second := m.activeSession()
	second.Title = "UPDATE orders"
	second.MarkRunning(func() {})

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	third := m.activeSession()
	third.Title = "untitled"

	checkGolden(t, "tab_bar", ansi.Strip(m.tabBar(80)))
}
