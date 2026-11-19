package app

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// senderHolder lets background runners call tea.Program.Send. main injects the
// program's Send after construction; the pointer is shared with the program's
// model copy.
type senderHolder struct {
	mu   sync.RWMutex
	send func(tea.Msg)
}

func newSenderHolder() *senderHolder { return &senderHolder{} }

// SetSender installs the program sender.
func (m Model) SetSender(send func(tea.Msg)) { m.sender.set(send) }

func (s *senderHolder) set(send func(tea.Msg)) {
	s.mu.Lock()
	s.send = send
	s.mu.Unlock()
}

func (s *senderHolder) clear() {
	s.mu.Lock()
	s.send = nil
	s.mu.Unlock()
}

func (s *senderHolder) publish(msg tea.Msg) {
	s.mu.RLock()
	send := s.send
	s.mu.RUnlock()
	if send != nil {
		send(msg)
	}
}

// runnerGroup tracks background runner and forwarder goroutines so shutdown
// can wait for them. It must live behind a pointer (models are copied).
type runnerGroup struct {
	wg     sync.WaitGroup
	mu     sync.Mutex
	active int
}

func newRunnerGroup() *runnerGroup { return &runnerGroup{} }

func (g *runnerGroup) begin(count int) {
	g.mu.Lock()
	g.active += count
	g.mu.Unlock()
	g.wg.Add(count)
}

func (g *runnerGroup) done() {
	g.mu.Lock()
	g.active--
	g.mu.Unlock()
	g.wg.Done()
}

func (g *runnerGroup) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.active
}

func (g *runnerGroup) wait() { g.wg.Wait() }
