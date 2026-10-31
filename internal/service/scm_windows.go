//go:build windows

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// scm: a Windows service, started automatically, restarted after a crash.
// It runs `mediaserver service run <WorkDir> <flags>`.
type scm struct{}

// New returns the manager of this computer.
func New() Manager { return scm{} }

func connect() (*mgr.Mgr, error) {
	m, err := mgr.Connect()
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return nil, errors.New("open an administrator terminal (Run as administrator) to manage the service")
	}
	if err != nil {
		return nil, fmt.Errorf("service manager: %w", err)
	}
	return m, nil
}

// open finds the installed service.
func open(m *mgr.Mgr) (*mgr.Service, error) {
	s, err := m.OpenService(Name)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil, ErrNotInstalled
	}
	if err != nil {
		return nil, fmt.Errorf("open service: %w", err)
	}
	return s, nil
}

func (scm) Install(sp Spec) (string, error) {
	m, err := connect()
	if err != nil {
		return "", err
	}
	defer m.Disconnect() //nolint:errcheck // nothing to do about it
	if s, err := m.OpenService(Name); err == nil {
		_ = s.Close()
		return "", errors.New("the service is already installed: run `mediaserver service uninstall` first")
	}
	args := append([]string{"service", "run", sp.WorkDir}, sp.Args...)
	s, err := m.CreateService(Name, sp.Exe, mgr.Config{
		DisplayName:      "mediaserver",
		Description:      Description,
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true, // after the network
	}, args...)
	if err != nil {
		return "", fmt.Errorf("create service: %w", err)
	}
	defer s.Close()
	restart := mgr.RecoveryAction{Type: mgr.ServiceRestart, Delay: 5 * time.Second}
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{restart, restart, restart}, uint32((24 * time.Hour).Seconds())); err != nil {
		return "", fmt.Errorf("service recovery: %w", err)
	}
	if err := s.Start(); err != nil {
		return "Windows service " + Name, fmt.Errorf("start service: %w", err)
	}
	return "Windows service " + Name, nil
}

func (scm) Uninstall() error {
	m, err := connect()
	if err != nil {
		return err
	}
	defer m.Disconnect() //nolint:errcheck // nothing to do about it
	s, err := open(m)
	if err != nil {
		return err
	}
	defer s.Close()
	_ = stop(s) // already stopped is fine
	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

func (scm) Start() error {
	return withService(func(s *mgr.Service) error { return s.Start() })
}

func (scm) Stop() error { return withService(stop) }

func (scm) Status() (string, error) {
	state := "not installed"
	err := withService(func(s *mgr.Service) error {
		st, err := s.Query()
		if err != nil {
			return err
		}
		state = map[svc.State]string{
			svc.Stopped: "stopped", svc.StartPending: "starting", svc.StopPending: "stopping",
			svc.Running: "running", svc.Paused: "paused",
		}[st.State]
		return nil
	})
	if errors.Is(err, ErrNotInstalled) {
		return state, nil
	}
	return state, err
}

func withService(f func(s *mgr.Service) error) error {
	m, err := connect()
	if err != nil {
		return err
	}
	defer m.Disconnect() //nolint:errcheck // nothing to do about it
	s, err := open(m)
	if err != nil {
		return err
	}
	defer s.Close()
	return f(s)
}

// stop asks the service to stop and waits for it (the server takes up to
// its shutdown timeout).
func stop(s *mgr.Service) error {
	st, err := s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("stop service: %w", err)
	}
	for deadline := time.Now().Add(30 * time.Second); st.State != svc.Stopped; {
		if time.Now().After(deadline) {
			return errors.New("the service did not stop in 30 s")
		}
		time.Sleep(300 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return fmt.Errorf("stop service: %w", err)
		}
	}
	return nil
}

// Run runs the server as a Windows service: Stop and Shutdown from the
// service manager cancel ctx (the server's graceful shutdown).
func Run(run func(ctx context.Context) int) error {
	return svc.Run(Name, handler{run})
}

type handler struct{ run func(ctx context.Context) int }

func (h handler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan int, 1)
	go func() { done <- h.run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case code := <-done:
			// Exited by itself (bad config, port in use): non-zero makes the
			// service manager apply the recovery actions.
			return code != 0, uint32(code) //nolint:gosec // exit codes are small
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 30_000}
				cancel()
				return false, uint32(<-done) //nolint:gosec // exit codes are small
			default:
			}
		}
	}
}
