//go:build !windows

package game

import (
	"context"
	"time"
)

var DefaultTargetGames = []string{
	"cs2.exe",
	"dota2.exe",
}

func FindRunningGames(targets []string) ([]string, error) {
	return nil, nil
}

type Watcher struct{}

func NewWatcher(targets []string, interval time.Duration, onStart, onStop func(string)) *Watcher {
	return &Watcher{}
}

func (w *Watcher) Start(ctx context.Context) {}
func (w *Watcher) Stop()                     {}
func (w *Watcher) RunningGames() []string    { return []string{} }
