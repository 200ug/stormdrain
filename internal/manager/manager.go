package manager

import (
	"fmt"
	"sync"
	"time"
)

// Handles all podman related actions, e.g. fetching the container count,
// querying runtime statistics, and so on. This means Manager.Run() should be
// started in a goroutine alongside the actual user interface.
type Manager struct {
	PodmanStats PodmanStats
	Containers  []Container

	CmdChan   chan Command
	NotifChan chan string
	ErrChan   chan error
	Mu        sync.RWMutex
}

func NewManager(fullInit bool) (*Manager, error) {
	if !podmanInPath() {
		return nil, fmt.Errorf("podman not in PATH")
	}

	var podmanStats PodmanStats
	if IsDarwin {
		rawMachineStats, err := ensurePodmanMachineIsRunning()
		if err != nil {
			return nil, err
		}
		podmanStats = NewPodmanStats(rawMachineStats)
	} else {
		podmanStats = newLinuxStats()
	}
	if !fullInit {
		// return early if we just want to perform the check(s), but don't need stats
		return nil, nil
	}

	// look up stormdrain related containers
	containers, err := getStormdrainContainers()
	if err != nil {
		return nil, err
	}

	return &Manager{
		PodmanStats: podmanStats,
		Containers:  containers,
		CmdChan:     make(chan Command), // notably no buffering
		NotifChan:   make(chan string, 10),
		ErrChan:     make(chan error, 10),
	}, nil
}

func (m *Manager) Run(stopChan <-chan any) {
	var err error
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case cmd := <-m.CmdChan:
			m.handleCommand(cmd)
		case <-t.C:
			m.Mu.Lock()
			m.Containers, err = getStormdrainContainers()
			m.Mu.Unlock()
			if err != nil {
				m.ErrChan <- err
			}
		case <-stopChan:
			return
		}
	}
}

func (m *Manager) handleCommand(cmd Command) {
	var err error
	if cmd.Type == Purge {
		m.handlePurgeCommand(cmd)
	} else {
		if err = cmd.Execute(); err != nil {
			m.ErrChan <- err
		} else {
			m.NotifChan <- cmd.NotificationPrint()
		}
	}
	// update our knowledge of containers after every command
	m.Mu.Lock()
	m.Containers, err = getStormdrainContainers()
	m.Mu.Unlock()
	if err != nil {
		m.ErrChan <- err // might overwrite our previous errors on ui, who knows
	}
}

func (m *Manager) handlePurgeCommand(cmd Command) {
	var err error
	m.Mu.RLock()
	containers := m.Containers
	m.Mu.RUnlock()

	var purgeErr error
	volumeNames := make(map[string]any)
	for _, c := range containers {
		spec, err := LoadSpec(c.ProjectPath)
		if err != nil {
			purgeErr = err
			break
		}
		if err = spec.RemoveContainer(); err != nil {
			purgeErr = err
			break
		}
		for _, v := range spec.VirtualVolumes {
			volumeNames[v.Name] = struct{}{}
		}
	}
	if purgeErr != nil {
		m.ErrChan <- fmt.Errorf("purge aborted: %w", purgeErr)
		return
	}

	for vn := range volumeNames {
		// let's reuse the same error holder, because why not
		if err = removeVolume(vn); err != nil && purgeErr == nil {
			purgeErr = err
		}
	}
	if purgeErr != nil {
		m.ErrChan <- purgeErr
	} else {
		m.NotifChan <- fmt.Sprintf("Purged %d containers and %d volumes", len(containers), len(volumeNames))
	}
}
