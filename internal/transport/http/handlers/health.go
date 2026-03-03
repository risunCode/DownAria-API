package handlers

import (
	"bufio"
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"downaria-api/pkg/response"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response.WriteSuccessRequest(w, r, http.StatusOK, h.buildStatusPayload())
}

func (h *Handler) Root(w http.ResponseWriter, r *http.Request) {
	response.WriteSuccessRequest(w, r, http.StatusOK, h.buildStatusPayload())
}

func (h *Handler) buildStatusPayload() map[string]any {
	now := time.Now().UTC()
	uptime := now.Sub(h.startedAt.UTC())

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	totalRAM, availableRAM := readLinuxRAMInfo()
	rootDisk := readDiskUsage("/")
	tempDisk := readDiskUsage(os.TempDir())
	hostname, _ := os.Hostname()

	return map[string]any{
		"status":       "ok",
		"message":      "DownAria-API is running",
		"timestamp":    now.Format(time.RFC3339),
		"startedAt":    h.startedAt.UTC().Format(time.RFC3339),
		"uptime":       formatUptime(uptime),
		"uptimeSecond": int64(uptime.Seconds()),
		"system": map[string]any{
			"hostname":  hostname,
			"os":        runtime.GOOS,
			"arch":      runtime.GOARCH,
			"goVersion": runtime.Version(),
			"cpuCores":  runtime.NumCPU(),
		},
		"memory": map[string]any{
			"total":              formatBytesAuto(totalRAM),
			"totalBytes":         totalRAM,
			"available":          formatBytesAuto(availableRAM),
			"availableBytes":     availableRAM,
			"processAlloc":       formatBytesAuto(mem.Alloc),
			"processAllocBytes":  mem.Alloc,
			"processSystem":      formatBytesAuto(mem.Sys),
			"processSystemBytes": mem.Sys,
		},
		"storage": map[string]any{
			"root": rootDisk,
			"temp": tempDisk,
		},
	}
}

func formatBytesAuto[T ~uint64 | ~uint](v T) string {
	bytes := float64(v)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	idx := 0
	for bytes >= 1024 && idx < len(units)-1 {
		bytes /= 1024
		idx++
	}
	if idx == 0 {
		return strconv.FormatUint(uint64(v), 10) + " " + units[idx]
	}
	return strconv.FormatFloat(bytes, 'f', 2, 64) + " " + units[idx]
}

func formatUptime(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d.Seconds())
	days := total / 86400
	hours := (total % 86400) / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if days > 0 {
		return strconv.FormatInt(days, 10) + "d " + strconv.FormatInt(hours, 10) + "h " + strconv.FormatInt(minutes, 10) + "m " + strconv.FormatInt(seconds, 10) + "s"
	}
	if hours > 0 {
		return strconv.FormatInt(hours, 10) + "h " + strconv.FormatInt(minutes, 10) + "m " + strconv.FormatInt(seconds, 10) + "s"
	}
	if minutes > 0 {
		return strconv.FormatInt(minutes, 10) + "m " + strconv.FormatInt(seconds, 10) + "s"
	}
	return strconv.FormatInt(seconds, 10) + "s"
}

func readLinuxRAMInfo() (totalBytes uint64, availableBytes uint64) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "MemTotal:") {
			totalBytes = parseMemInfoLine(line)
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			availableBytes = parseMemInfoLine(line)
		}
	}

	return totalBytes, availableBytes
}

func parseMemInfoLine(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return v * 1024
}

func readDiskUsage(path string) map[string]any {
	result := map[string]any{
		"path":       path,
		"total":      "0 B",
		"totalBytes": uint64(0),
		"used":       "0 B",
		"usedBytes":  uint64(0),
		"free":       "0 B",
		"freeBytes":  uint64(0),
	}

	out, err := exec.Command("df", "-kP", path).Output()
	if err != nil {
		return result
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return result
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return result
	}

	totalKB, errTotal := strconv.ParseUint(fields[1], 10, 64)
	usedKB, errUsed := strconv.ParseUint(fields[2], 10, 64)
	freeKB, errFree := strconv.ParseUint(fields[3], 10, 64)
	if errTotal != nil || errUsed != nil || errFree != nil {
		return result
	}

	totalBytes := totalKB * 1024
	usedBytes := usedKB * 1024
	freeBytes := freeKB * 1024

	result["total"] = formatBytesAuto(totalBytes)
	result["totalBytes"] = totalBytes
	result["used"] = formatBytesAuto(usedBytes)
	result["usedBytes"] = usedBytes
	result["free"] = formatBytesAuto(freeBytes)
	result["freeBytes"] = freeBytes
	return result
}
