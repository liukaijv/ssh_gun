// Package singleinstance ensures only one Feisuo UI process runs and can
// ask the primary instance to show its window.
package singleinstance

import "errors"

// ErrAlreadyRunning is returned by Acquire when another primary instance holds the lock.
var ErrAlreadyRunning = errors.New("another instance is already running")

// Guard is held by the primary instance until Close.
type Guard struct {
	impl guardImpl
}

// Acquire tries to become the primary instance.
// On success the caller must Close the Guard when the app exits.
// On ErrAlreadyRunning the caller should NotifyActivate and exit.
func Acquire() (*Guard, error) {
	impl, err := acquireImpl()
	if err != nil {
		return nil, err
	}
	return &Guard{impl: impl}, nil
}

// NotifyActivate asks the primary instance to show its window.
func NotifyActivate() error {
	return notifyActivateImpl()
}

// ListenActivate starts accepting activation requests until Close.
// onActivate may be called from a background goroutine.
func (g *Guard) ListenActivate(onActivate func()) error {
	if g == nil || g.impl == nil {
		return errors.New("nil guard")
	}
	return g.impl.listen(onActivate)
}

// Close releases the single-instance lock and stops the activate listener.
func (g *Guard) Close() {
	if g == nil || g.impl == nil {
		return
	}
	g.impl.close()
	g.impl = nil
}

type guardImpl interface {
	listen(onActivate func()) error
	close()
}
