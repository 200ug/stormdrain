package manager

import (
	"fmt"
)

type CommandType int

const (
	Create CommandType = iota
	Attach
	Stop
	Remove
	Purge
)

// Structure that holds incoming commands initiated from the user interface.
type Command struct {
	Type  CommandType
	Spec  Spec // notably this doesn't need to be "full" spec
	Force bool // only applies to the stop command
}

func (c *Command) Execute() error {
	switch c.Type {
	case Create:
		// NOTE: at this point we should've already loaded the profile config,
		//		 handled substitution into Dockerfile, and staged the user configs
		defer CleanupStagedConfigs(c.Spec.ProjectPath, c.Spec.ContainerName)
		return c.Spec.CreateContainer() // builds, starts, and persists
	case Attach:
		return c.Spec.AttachIntoContainer()
	case Stop:
		return stopContainer(c.Spec.ContainerName, c.Force)
	case Remove:
		return c.Spec.RemoveContainer()
	case Purge:
		return fmt.Errorf("command fallthrough")
	default:
		return fmt.Errorf("unknown command type")
	}
}

func (c *Command) NotificationPrint() string {
	switch c.Type {
	case Create:
		return "Container created successfully"
	case Attach:
		// NOTE: this case should never happend, as attaching is handled
		//		 completely TUI-side (due to app.Suspend)
		return "Restored previous state successfully"
	case Stop:
		if c.Force {
			return "Container killed successfully"
		}
		return "Container stopped successfully"
	case Remove:
		return "Container removed successfully"
	case Purge:
		return "You shouldn't see this message"
	default:
		return "Something went wrong"
	}
}
