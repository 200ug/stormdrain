package manager

import (
	"fmt"
	"strings"
	"time"
)

// Structure that holds all necessary container information. Should be updated
// with Podman's batch commands instead of querying containers one by one.
type Container struct {
	Name      string
	UptimeSec int    // -1 if down
	CPU       string // "<dir_perc>% / <avg_perc>%"
	Memory    string // "<perc>%"
	NetIO     string // "<total_sent> / <total_received>"

	ImageTag    string
	ProjectPath string // from label
	ProfileName string // from label
	Mounts      []string
	Ports       []portPs
}

func NewContainer(ps containerPs, stats *containerStats) Container {
	c := Container{
		Name:        "-",
		ImageTag:    ps.ImageTag,
		ProjectPath: ps.Labels.ProjectPath,
		ProfileName: ps.Labels.ProfileName,
		Mounts:      ps.Mounts,
		Ports:       ps.Ports,
	}
	if ps.State != "running" {
		c.UptimeSec = -1
	} else {
		c.UptimeSec = computeUptime(ps.StartedAt)
	}
	if stats != nil {
		c.Name = stats.Name
		c.CPU = fmt.Sprintf("%s / %s", stats.CPUDirectPercentage, stats.CPUAveragePercentage)
		c.Memory = stats.MemoryPercentage
		c.NetIO = stats.NetworkIO
	}
	return c
}

func (c *Container) FormatDetails() string {
	b := strings.Builder{}
	fmt.Fprintf(&b, "Image: %s\n", c.ImageTag)
	fmt.Fprintf(&b, "Project: %s\n", c.ProjectPath)
	if len(c.Ports) > 0 {
		var ports []string
		for _, p := range c.Ports {
			ports = append(ports, fmt.Sprintf("%d:%d/%s", p.HostPort, p.ContainerPort, p.Protocol))
		}
		fmt.Fprintf(&b, "Ports: %s\n", strings.Join(ports, ", "))
	} else {
		fmt.Fprintf(&b, "Ports: -\n")
	}
	if len(c.Mounts) > 0 {
		fmt.Fprintf(&b, "Mounts: %s\n", strings.Join(c.Mounts, ", "))
	} else {
		fmt.Fprintf(&b, "Mounts: -\n")
	}
	if c.NetIO != "" {
		fmt.Fprintf(&b, "Net I/O: %s\n", c.NetIO)
	} else {
		fmt.Fprintf(&b, "Net I/O: -\n")
	}
	return b.String()
}

func (c *Container) StatusString() string {
	if c.UptimeSec < 0 {
		return "down"
	}
	d := time.Duration(c.UptimeSec) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds up", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm up", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh up", int(d.Hours()))
	}
	return fmt.Sprintf("%dd up", int(d.Hours()/24))
}

func computeUptime(startedAt int) int {
	if startedAt == 0 {
		return -1
	}
	return int(time.Now().Unix()) - startedAt
}
