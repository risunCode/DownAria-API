package ports

import "time"

// StatsSnapshot represents the public statistics data
type StatsSnapshot struct {
	TodayVisits      int64 `json:"todayVisits"`
	TotalVisits      int64 `json:"totalVisits"`
	TotalExtractions int64 `json:"totalExtractions"`
	TotalDownloads   int64 `json:"totalDownloads"`
}

// StatsStore defines the interface for statistics storage operations
type StatsStore interface {
	// RecordVisitor records a unique visitor for the current day
	RecordVisitor(visitorKey string, now time.Time)
	
	// RecordExtraction increments the extraction counter
	RecordExtraction(now time.Time)
	
	// RecordDownload increments the download counter
	RecordDownload(now time.Time)
	
	// Snapshot returns the current statistics snapshot
	Snapshot(now time.Time) StatsSnapshot
}
