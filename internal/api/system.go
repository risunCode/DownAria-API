package api

import (
	"os"
	"runtime"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

func diskUsage(path string) (uint64, uint64) {
	if path == "" {
		if runtime.GOOS == "windows" {
			path = "C:\\"
		} else {
			path = "/"
		}
	}
	
	// Use the directory of the path if it's a file
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		path = "."
	}

	usage, err := disk.Usage(path)
	if err != nil {
		usage, err = disk.Usage(".")
		if err != nil {
			return 0, 0
		}
	}
	return usage.Total, usage.Free
}

func rootDiskUsage() (uint64, uint64) {
	root := "/"
	if runtime.GOOS == "windows" {
		root = "C:\\"
	}
	usage, err := disk.Usage(root)
	if err != nil {
		return 0, 0
	}
	return usage.Total, usage.Free
}

func memoryStatus() (uint64, uint64) {
	v, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0
	}
	return v.Total, v.Available
}
