// Package agent runs checks periodically, keeps the latest state and reports
// status changes.
package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"ipcheck/internal/checker"
	"ipcheck/internal/config"
	"ipcheck/internal/notify"
)

type State struct {
	OK       bool             `json:"ok"`
	Problems int              `json:"problems"`
	Updated  time.Time        `json:"updated"`
	Interval string           `json:"interval"`
	Results  []checker.Result `json:"results"`
}

type Agent struct {
	cfg     *config.Config
	checker *checker.Checker
	trigger chan struct{}

	mu      sync.Mutex
	results []checker.Result
	updated time.Time
	subs    map[chan struct{}]struct{}
}

func New(cfg *config.Config) *Agent {
	return &Agent{
		cfg:     cfg,
		checker: checker.New(cfg),
		trigger: make(chan struct{}, 1),
		subs:    map[chan struct{}]struct{}{},
	}
}

func (a *Agent) Run(ctx context.Context) {
	t := time.NewTicker(a.cfg.Interval)
	defer t.Stop()
	for {
		a.checkOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-a.trigger:
			t.Reset(a.cfg.Interval)
		}
	}
}

// CheckNow requests an immediate check.
func (a *Agent) CheckNow() {
	select {
	case a.trigger <- struct{}{}:
	default:
	}
}

func (a *Agent) Snapshot() State {
	a.mu.Lock()
	defer a.mu.Unlock()
	return MakeState(a.results, a.updated, a.cfg.Interval)
}

// MakeState summarizes results into the state served by the API.
func MakeState(results []checker.Result, updated time.Time, interval time.Duration) State {
	s := State{Updated: updated, Interval: interval.String(), Results: results}
	for _, r := range results {
		if r.Status != checker.OK {
			s.Problems++
		}
	}
	s.OK = len(results) > 0 && s.Problems == 0
	return s
}

// Subscribe returns a channel that receives a signal after every check.
func (a *Agent) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	a.mu.Lock()
	a.subs[ch] = struct{}{}
	a.mu.Unlock()
	return ch, func() {
		a.mu.Lock()
		delete(a.subs, ch)
		a.mu.Unlock()
	}
}

func (a *Agent) checkOnce(ctx context.Context) {
	results := a.checker.CheckAll(ctx)

	a.mu.Lock()
	prev := a.results
	a.results, a.updated = results, time.Now()
	for ch := range a.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	a.mu.Unlock()

	if a.cfg.NotifyEnabled() {
		reportChanges(prev, results)
	}
}

func reportChanges(prev, cur []checker.Result) {
	old := make(map[string]checker.Result, len(prev))
	for _, r := range prev {
		old[r.Group] = r
	}
	for _, r := range cur {
		p, seen := old[r.Group]
		switch {
		case !seen && r.Status == checker.OK:
			// first check and everything is fine — stay quiet
		case seen && p.Status == r.Status && p.IP == r.IP:
			// nothing changed
		case r.Status == checker.OK:
			notify.Send("🟢 "+r.Group+" восстановлен",
				fmt.Sprintf("%s %s · %s", checker.Flag(r.Country), r.Exit, r.IP), false)
		default:
			notify.Send(fmt.Sprintf("🔴 %s: %s", r.Group, Describe(r.Status)), r.Message, true)
		}
	}
}

func Describe(s checker.Status) string {
	switch s {
	case checker.OK:
		return "OK"
	case checker.Mismatch:
		return "не тот выход"
	case checker.Unknown:
		return "неизвестный выход"
	case checker.DNSBypass:
		return "DNS мимо podkop"
	default:
		return "ошибка"
	}
}
