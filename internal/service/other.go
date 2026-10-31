//go:build !linux && !darwin && !windows

package service

import "context"

type unsupported struct{}

// New returns the manager of this computer.
func New() Manager { return unsupported{} }

func (unsupported) Install(Spec) (string, error) { return "", ErrUnsupported }
func (unsupported) Uninstall() error             { return ErrUnsupported }
func (unsupported) Start() error                 { return ErrUnsupported }
func (unsupported) Stop() error                  { return ErrUnsupported }
func (unsupported) Status() (string, error)      { return "", ErrUnsupported }

// Run is only used by Windows services.
func Run(func(ctx context.Context) int) error { return ErrUnsupported }
