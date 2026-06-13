package tui

import (
	"fmt"
	"strconv"
	"strings"

	"codeberg.org/2ug/stormdrain/internal/manager"
	"codeberg.org/2ug/stormdrain/internal/util"
)

// Produces "<HOST1>:<CONTAINER1>, <HOST2>:<CONTAINER2>, ..." style string.
func formatPorts(ports []manager.PortMap) string {
	var sb strings.Builder
	for i, p := range ports {
		if i != 0 {
			fmt.Fprintf(&sb, ", ")
		}
		fmt.Fprintf(&sb, "%d:%d", p.Host, p.Container)
	}
	return sb.String()
}

func parsePorts(rawPorts string) ([]manager.PortMap, error) {
	src := util.StripAllWhitespace(rawPorts)
	if src == "" {
		return nil, nil
	}

	segments := strings.Split(src, ",")
	ports := make([]manager.PortMap, 0, len(segments))
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		parts := strings.Split(seg, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid port mapping %q, expected host:container", seg)
		}
		host, err := strconv.Atoi(parts[0])
		if err != nil || host < 1 || host > 65535 {
			return nil, fmt.Errorf("invalid host port %q in mapping %q", parts[0], seg)
		}
		container, err := strconv.Atoi(parts[1])
		if err != nil || container < 1 || container > 65535 {
			return nil, fmt.Errorf("invalid container port %q in mapping %q", parts[1], seg)
		}
		ports = append(ports, manager.PortMap{Host: host, Container: container})
	}
	return ports, nil
}

// Produces "<NAME1>:<PATH1>, <NAME2>:<PATH2>, ..." style string.
func formatVirtualVolumes(volumes []manager.VirtualVolume) string {
	var sb strings.Builder
	for i, v := range volumes {
		if i != 0 {
			fmt.Fprintf(&sb, ", ")
		}
		fmt.Fprintf(&sb, "%s:%s", v.Name, v.Path)
	}
	return sb.String()
}

func parseVirtualVolumes(rawVolumes string) ([]manager.VirtualVolume, error) {
	src := util.StripAllWhitespace(rawVolumes)
	if src == "" {
		return nil, nil
	}

	segments := strings.Split(src, ",")
	volumes := make([]manager.VirtualVolume, 0, len(segments))
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		parts := strings.SplitN(seg, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("invalid virtual volume %q, expected name:path", seg)
		}
		volumes = append(volumes, manager.VirtualVolume{Name: parts[0], Path: parts[1]})
	}
	return volumes, nil
}

// Produces "<ENVPATH1>, <ENVPATH2>, ..." style string.
func formatEnvFiles(envs []string) string {
	var sb strings.Builder
	for i, e := range envs {
		if i != 0 {
			fmt.Fprintf(&sb, ", ")
		}
		fmt.Fprintf(&sb, "%s", e)
	}
	return sb.String()
}

func parseEnvFiles(rawEnvs string) ([]string, error) {
	src := util.StripAllWhitespace(rawEnvs)
	if src == "" {
		return nil, nil
	}

	segments := strings.Split(src, ",")
	envs := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		envs = append(envs, seg)
	}
	if len(envs) == 0 {
		return nil, nil
	}
	return envs, nil
}
