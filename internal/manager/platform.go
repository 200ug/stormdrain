package manager

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

var IsDarwin bool

func init() {
	IsDarwin = runtime.GOOS == "darwin"
}

func neWLinuxStats() PodmanStats {
	return PodmanStats{
		IsNative:               true,
		MachineName:            "native",
		AvailableTotalCPUs:     runtime.NumCPU(),
		AvailableTotalMemoryGB: totalMemoryGBFromProc(),
		AvailableDiskSizeGB:    -1,
	}
}

func totalMemoryGBFromProc() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return -1
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return -1
			}
			kb, err := strconv.Atoi(fields[1])
			if err != nil {
				return -1
			}
			return kb / (1024 * 1024)
		}
	}
	return -1
}
