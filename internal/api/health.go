package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"downaria-api/internal/api/middleware"
	"downaria-api/internal/extract"
	"downaria-api/internal/outbound"
	"downaria-api/internal/storage"
	"github.com/shirou/gopsutil/v4/host"
	psnet "github.com/shirou/gopsutil/v4/net"
)

type CacheStatsProvider interface{ Stats() extract.CacheStats }

type HealthOptions struct {
	Dependencies map[string]string
	Cache        CacheStatsProvider
	TempDir      string
	ActiveCount  func() int
	ActiveJobs   func() []*storage.Job
}

type SecurityOptions struct {
	Guard          URLGuard
	MaxOutputBytes int64
}

type RouterOptions struct {
	Health         HealthOptions
	Security       SecurityOptions
	Jobs           JobService
	ArtifactStore  ArtifactStore
	OutboundClient *outbound.Client
	Stats          *statsStore
	ReadyFn        func() bool
	MediaLimiter   *middleware.RateLimiter
	JobLimiter     *middleware.RateLimiter
}

type healthData struct {
	Status       string            `json:"status"`
	StartedAt    time.Time         `json:"started_at"`
	Uptime       string            `json:"uptime"`
	Process      processState      `json:"process"`
	System       systemState       `json:"system"`
	Runtime      runtimeState      `json:"runtime"`
	Jobs         jobsState         `json:"jobs"`
	Network      networkState      `json:"network"`
	Cache        cacheState        `json:"cache"`
	TempStorage  tempStorageState  `json:"temp_storage"`
	Dependencies map[string]string `json:"dependencies"`
}

type processState struct {
	Memory memoryState `json:"memory"`
}

type systemState struct {
	RAM     storageState `json:"ram"`
	Storage storageState `json:"storage"`
	CPU     cpuState     `json:"cpu"`
	Host    hostState    `json:"host"`
}

type runtimeState struct {
	GOOS       string `json:"goos"`
	GOARCH     string `json:"goarch"`
	Goroutines int    `json:"goroutines"`
	GOMAXPROCS int    `json:"gomaxprocs"`
	Version    string `json:"go_version"`
}

type hostState struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Platform string `json:"platform"`
	Kernel   string `json:"kernel"`
	Uptime   uint64 `json:"uptime"`
	Cores    int    `json:"cores"`
	RAMTotal string `json:"ram_total_human"`
}

type memoryState struct {
	HeapAllocBytes uint64 `json:"heap_alloc_bytes"`
	HeapAlloc      string `json:"heap_alloc"`
	SysBytes       uint64 `json:"sys_bytes"`
	Sys            string `json:"sys"`
	HeapObjects    uint64 `json:"heap_objects"`
}

type storageState struct {
	TotalBytes uint64 `json:"total_bytes"`
	Total      string `json:"total"`
	FreeBytes  uint64 `json:"free_bytes"`
	Free       string `json:"free"`
	UsedBytes  uint64 `json:"used_bytes"`
	Used       string `json:"used"`

	// App specific footprints
	AppUsed      string `json:"app_used,omitempty"`
	AppUsedBytes int64  `json:"app_used_bytes,omitempty"`
	AppLimit     string `json:"app_limit,omitempty"`

	// Global machine stats
	GlobalTotal string `json:"global_total,omitempty"`
	GlobalUsed  string `json:"global_used,omitempty"`
}

type cpuState struct {
	Cores     int    `json:"cores"`
	Name      string `json:"name"`
	Arch      string `json:"arch"`
	UsageNote string `json:"usage_note,omitempty"`
}

type jobsState struct {
	Active  int      `json:"active"`
	States  []string `json:"states,omitempty"`
	Summary string   `json:"summary,omitempty"`
}

type networkState struct {
	InBytesTotal  uint64  `json:"in_bytes_total"`
	OutBytesTotal uint64  `json:"out_bytes_total"`
	InBPS         float64 `json:"in_bps"`
	OutBPS        float64 `json:"out_bps"`
	InHuman       string  `json:"in_human"`
	OutHuman      string  `json:"out_human"`
	InterfaceMode string  `json:"interface_mode"`
	UpdatedAt     string  `json:"updated_at"`
}

type cacheState struct {
	Entries        int    `json:"entries"`
	BytesUsed      int64  `json:"bytes_used"`
	BytesUsedHuman string `json:"bytes_used_human"`
}

type tempStorageState struct {
	Root           string `json:"root,omitempty"`
	Entries        int    `json:"entries"`
	BytesUsed      int64  `json:"bytes_used"`
	BytesUsedHuman string `json:"bytes_used_human"`
}

type healthCacheEntry struct {
	mu        sync.Mutex
	root      string
	expiresAt time.Time
	state     tempStorageState
}

var tempStorageCache healthCacheEntry
var memStatsCache cachedValue[runtime.MemStats]
var storageCache cachedValue[storageState]
var ramCache cachedValue[storageState]
var networkStateCache cachedValue[networkState]

var networkCache = struct {
	mu          sync.Mutex
	initialized bool
	lastAt      time.Time
	lastIn      uint64
	lastOut     uint64
	snapshot    networkState
}{}

func buildHealthData(options HealthOptions) healthData {
	mem := cachedMemStats()
	ram := cachedSystemRAM()
	storage := cachedStorage(options.TempDir)
	hostInfo, _ := host.Info()

	// 892MB Default limit for App footprint context
	appStorageLimit := "892.0 MB"

	// Sync folder scan usage to storage state
	tempStats := readTempStorage(options.TempDir)
	storage.AppUsed = tempStats.BytesUsedHuman
	storage.AppUsedBytes = tempStats.BytesUsed
	storage.AppLimit = appStorageLimit

	// Update host state with additional info
	hState := hostState{
		Hostname: hostInfo.Hostname,
		OS:       hostInfo.OS,
		Platform: hostInfo.Platform,
		Kernel:   hostInfo.KernelVersion,
		Uptime:   hostInfo.Uptime,
		Cores:    runtime.NumCPU(),
		RAMTotal: humanBytes(ram.TotalBytes),
	}

	cache := cacheState{}
	if options.Cache != nil {
		stats := options.Cache.Stats()
		cache = cacheState{Entries: stats.Entries, BytesUsed: stats.BytesUsed, BytesUsedHuman: humanBytesInt64(stats.BytesUsed)}
	}
	jobs := jobsState{}
	if options.ActiveCount != nil {
		jobs.Active = options.ActiveCount()
	}
	if options.ActiveJobs != nil {
		active := options.ActiveJobs()
		typeCounts := make(map[string]int)
		for _, j := range active {
			typeCounts[j.State]++
			jobs.States = append(jobs.States, j.State)
		}
		if len(typeCounts) > 0 {
			var summary []string
			for state, count := range typeCounts {
				summary = append(summary, fmt.Sprintf("%d %s", count, state))
			}
			jobs.Summary = strings.Join(summary, ", ")
		}
	}
	deps := map[string]string{}
	for key, value := range options.Dependencies {
		deps[key] = value
	}
	return healthData{
		Status:    "ok",
		StartedAt: startedAt,
		Uptime:    time.Since(startedAt).Round(time.Second).String(),
		Process:   processState{Memory: memoryState{HeapAllocBytes: mem.HeapAlloc, HeapAlloc: humanBytes(mem.HeapAlloc), SysBytes: mem.Sys, Sys: humanBytes(mem.Sys), HeapObjects: mem.HeapObjects}},
		System: systemState{
			RAM:     ram,
			Storage: storage,
			CPU:     cpuState{Cores: hState.Cores, Name: os.Getenv("PROCESSOR_IDENTIFIER"), Arch: runtime.GOARCH, UsageNote: "cpu usage percentage is not collected in the minimal build"},
			Host:    hState,
		},
		Runtime:      runtimeState{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Goroutines: runtime.NumGoroutine(), GOMAXPROCS: runtime.GOMAXPROCS(0), Version: runtime.Version()},
		Jobs:         jobs,
		Network:      cachedNetworkState(),
		Cache:        cache,
		TempStorage:  readTempStorage(options.TempDir),
		Dependencies: deps,
	}
}

func cachedMemStats() runtime.MemStats {
	return memStatsCache.get(10*time.Second, func() runtime.MemStats {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		return mem
	})
}

func cachedStorage(tempDir string) storageState {
	return storageCache.get(5*time.Second, func() storageState {
		return readStorage(tempDir)
	})
}

func cachedSystemRAM() storageState {
	return ramCache.get(5*time.Second, readSystemRAM)
}

func cachedNetworkState() networkState {
	return networkStateCache.get(5*time.Second, snapshotNetworkState)
}

func snapshotNetworkState() networkState {
	now := time.Now()

	loopback := map[string]struct{}{}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		if (iface.Flags & net.FlagLoopback) != 0 {
			loopback[iface.Name] = struct{}{}
		}
	}

	counters, err := psnet.IOCounters(true)
	totalIn := uint64(0)
	totalOut := uint64(0)
	if err == nil {
		for _, item := range counters {
			if _, isLoopback := loopback[item.Name]; isLoopback {
				continue
			}
			totalIn += item.BytesRecv
			totalOut += item.BytesSent
		}
	}

	networkCache.mu.Lock()
	defer networkCache.mu.Unlock()

	state := networkCache.snapshot
	state.InBytesTotal = totalIn
	state.OutBytesTotal = totalOut
	state.InterfaceMode = "all_non_loopback_including_docker"
	state.UpdatedAt = now.Format(time.RFC3339)

	if networkCache.initialized {
		deltaSeconds := now.Sub(networkCache.lastAt).Seconds()
		if deltaSeconds > 0 {
			if totalIn >= networkCache.lastIn {
				state.InBPS = float64(totalIn-networkCache.lastIn) / deltaSeconds
			}
			if totalOut >= networkCache.lastOut {
				state.OutBPS = float64(totalOut-networkCache.lastOut) / deltaSeconds
			}
		}
	}

	state.InHuman = formatRate(state.InBPS)
	state.OutHuman = formatRate(state.OutBPS)

	networkCache.initialized = true
	networkCache.lastAt = now
	networkCache.lastIn = totalIn
	networkCache.lastOut = totalOut
	networkCache.snapshot = state

	return state
}

func formatRate(value float64) string {
	if value <= 0 {
		return "0 B/s"
	}
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	idx := 0
	for value >= 1024 && idx < len(units)-1 {
		value /= 1024
		idx++
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}

func shouldRenderHealthHTML(r *http.Request) bool {
	if r == nil {
		return false
	}
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "json" {
		return false
	}
	if format == "html" {
		return true
	}
	accept := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	return strings.Contains(accept, "text/html")
}

func writeHealth(w http.ResponseWriter, r *http.Request, options HealthOptions) {
	if shouldRenderHealthHTML(r) {
		writeHealthHTML(w)
		return
	}
	writeSuccess(w, r, http.StatusOK, buildHealthData(options))
}

func writeHealthHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(healthDashboardHTML))
}

func readTempStorage(root string) tempStorageState {
	root = filepath.Clean(root)
	if root == "." || root == "" {
		root = os.TempDir()
	}
	tempStorageCache.mu.Lock()
	if tempStorageCache.root == root && time.Now().Before(tempStorageCache.expiresAt) {
		state := tempStorageCache.state
		tempStorageCache.mu.Unlock()
		return state
	}
	tempStorageCache.mu.Unlock()
	info, err := os.Stat(root)
	if err != nil {
		return tempStorageState{Root: root}
	}
	if !info.IsDir() {
		return tempStorageState{Root: root}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return tempStorageState{Root: root}
	}
	state := tempStorageState{Root: root}
	for _, entry := range entries {
		state.Entries++
		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}
		if entryInfo.IsDir() {
			continue
		}
		state.BytesUsed += entryInfo.Size()
	}
	state.BytesUsedHuman = humanBytesInt64(state.BytesUsed)
	tempStorageCache.mu.Lock()
	tempStorageCache.root = root
	tempStorageCache.expiresAt = time.Now().Add(5 * time.Second)
	tempStorageCache.state = state
	tempStorageCache.mu.Unlock()
	return state
}

func readStorage(tempDir string) storageState {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	total, free := diskUsage(tempDir)
	used := uint64(0)
	if total >= free {
		used = total - free
	}

	state := storageState{
		TotalBytes: total,
		Total:      humanBytes(total),
		FreeBytes:  free,
		Free:       humanBytes(free),
		UsedBytes:  used,
		Used:       humanBytes(used),
	}

	// Add global root stats
	gTotal, gFree := rootDiskUsage()
	if gTotal > 0 {
		gUsed := uint64(0)
		if gTotal >= gFree {
			gUsed = gTotal - gFree
		}
		state.GlobalTotal = humanBytes(gTotal)
		state.GlobalUsed = humanBytes(gUsed)
	}

	return state
}

func readSystemRAM() storageState {
	total, free := memoryStatus()
	used := uint64(0)
	if total >= free {
		used = total - free
	}

	state := storageState{
		TotalBytes: total,
		Total:      humanBytes(total),
		FreeBytes:  free,
		Free:       humanBytes(free),
		UsedBytes:  used,
		Used:       humanBytes(used),
	}

	// Local Process Specific
	mem := cachedMemStats()
	pUsed := mem.Sys // Total memory obtained from OS

	state.AppUsed = humanBytes(pUsed)
	state.AppUsedBytes = int64(pUsed)

	// Try to detect local limit (e.g. Docker/Railway cgroup)
	localLimit := total
	if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
			if limit, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil && limit < total {
				localLimit = limit
			}
		} else if data, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
			// Cgroup v2
			if limit, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil && limit < total {
				localLimit = limit
			}
		}
	}
	
	// Check for Railway specific limit env if cgroup failed or returned host total
	if envLimit := os.Getenv("RAILWAY_MEMORY_LIMIT_MB"); envLimit != "" {
		if limitMB, err := strconv.ParseUint(envLimit, 10, 64); err == nil {
			localLimit = limitMB * 1024 * 1024
		}
	}
	
	state.AppLimit = humanBytes(localLimit)

	state.GlobalTotal = state.Total
	state.GlobalUsed = state.Used

	return state
}

func humanBytes(value uint64) string {
	return humanBytesInt64(int64(value))
}

func humanBytesInt64(value int64) string {
	if value < 1024 {
		return formatBytes(value, "B")
	}
	units := []string{"KB", "MB", "GB", "TB"}
	size := float64(value)
	unit := "B"
	for _, candidate := range units {
		size /= 1024
		unit = candidate
		if size < 1024 {
			break
		}
	}
	return formatFloatBytes(size, unit)
}

func formatBytes(value int64, unit string) string {
	return fmt.Sprintf("%d %s", value, unit)
}

func formatFloatBytes(value float64, unit string) string {
	return fmt.Sprintf("%.1f %s", value, unit)
}
