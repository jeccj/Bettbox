package main

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"core/state"

	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

const unknownProcess = "Unknown"

// appTrafficCounter keeps both views so changing OnlyStatisticsProxy keeps the
// same semantics as the existing total traffic counters.
type appTrafficCounter struct {
	upload        int64
	download      int64
	proxyUpload   int64
	proxyDownload int64
}

type appTrafficItem struct {
	Process  string `json:"process"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Total    int64  `json:"total"`
}

type trackerTrafficBaseline struct {
	upload   int64
	download int64
}

var (
	appTrafficLock      sync.Mutex
	appTrafficResetAt   = time.Now()
	appTraffic          = make(map[string]appTrafficCounter)
	appTrafficBaselines = make(map[string]trackerTrafficBaseline)
)

func resetAppTraffic() {
	appTrafficLock.Lock()
	defer appTrafficLock.Unlock()
	appTrafficResetAt = time.Now()
	appTraffic = make(map[string]appTrafficCounter)
	appTrafficBaselines = make(map[string]trackerTrafficBaseline)
}

func resetAppTrafficForTraffic() {
	appTrafficLock.Lock()
	defer appTrafficLock.Unlock()
	appTrafficResetAt = time.Now()
	appTraffic = make(map[string]appTrafficCounter)
	appTrafficBaselines = make(map[string]trackerTrafficBaseline)

	// TrackerInfo counters are lifetime counters and survive Manager.ResetStatistic.
	// Keep active per-tracker values so the next response starts at zero while
	// retaining bytes written by those same connections afterwards.
	snapshot := statistic.DefaultManager.Snapshot()
	for index := range snapshot.Connections {
		info := &snapshot.Connections[index]
		if !isAppTrafficTracker(info) {
			continue
		}
		appTrafficBaselines[info.UUID.String()] = trackerTrafficBaseline{
			upload:   info.UploadTotal.Load(),
			download: info.DownloadTotal.Load(),
		}
	}
}

func appTrafficProcess(info *statistic.TrackerInfo) string {
	if info == nil || info.Metadata == nil {
		return unknownProcess
	}
	process := strings.TrimSpace(info.Metadata.Process)
	if process == "" {
		process = strings.TrimSpace(info.Metadata.ProcessPath)
	}
	if process == "" {
		return unknownProcess
	}
	return process
}

func isAppTrafficTracker(info *statistic.TrackerInfo) bool {
	if info == nil {
		return false
	}
	// The current pushToManager=false call sites use INNER metadata without a
	// process. INNER alone is not enough: listener/inner creates counted
	// trackers with process "mihomo" and pushToManager=true.
	if info.Metadata != nil &&
		info.Metadata.Type == constant.INNER &&
		strings.TrimSpace(info.Metadata.Process) == "" &&
		strings.TrimSpace(info.Metadata.ProcessPath) == "" {
		return false
	}
	return true
}

func addAppTraffic(counters map[string]appTrafficCounter, info *statistic.TrackerInfo, upload, download int64) {
	if !isAppTrafficTracker(info) || (upload == 0 && download == 0) {
		return
	}
	process := appTrafficProcess(info)
	counter := counters[process]
	counter.upload += upload
	counter.download += download
	if !info.IsDirect {
		counter.proxyUpload += upload
		counter.proxyDownload += download
	}
	counters[process] = counter
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func addTrackerTraffic(counters map[string]appTrafficCounter, info *statistic.TrackerInfo, baseline *trackerTrafficBaseline) {
	if !isAppTrafficTracker(info) {
		return
	}
	upload := info.UploadTotal.Load()
	download := info.DownloadTotal.Load()
	if baseline != nil {
		upload = maxInt64(0, upload-baseline.upload)
		download = maxInt64(0, download-baseline.download)
	}
	addAppTraffic(counters, info, upload, download)
}

func recordAppTraffic(info *statistic.TrackerInfo) {
	appTrafficLock.Lock()
	defer appTrafficLock.Unlock()
	if !isAppTrafficTracker(info) {
		return
	}
	trackerID := info.UUID.String()
	baseline, hasBaseline := appTrafficBaselines[trackerID]
	if hasBaseline {
		delete(appTrafficBaselines, trackerID)
		addTrackerTraffic(appTraffic, info, &baseline)
		return
	}
	if !info.Start.IsZero() && info.Start.Before(appTrafficResetAt) {
		return
	}
	addTrackerTraffic(appTraffic, info, nil)
}

func collectAppTraffic(onlyProxy bool) []appTrafficItem {
	appTrafficLock.Lock()
	counters := make(map[string]appTrafficCounter, len(appTraffic))
	for process, counter := range appTraffic {
		counters[process] = counter
	}
	// Keep the lock while taking the snapshot so a connection close cannot be
	// counted in both the closed and active portions of the result. Mihomo
	// removes a tracker before invoking the close notification, so an in-flight
	// close may briefly appear in neither portion and be visible on the next
	// query.
	snapshot := statistic.DefaultManager.Snapshot()
	for index := range snapshot.Connections {
		info := &snapshot.Connections[index]
		baseline, hasBaseline := appTrafficBaselines[info.UUID.String()]
		if hasBaseline {
			addTrackerTraffic(counters, info, &baseline)
		} else if info.Start.IsZero() || !info.Start.Before(appTrafficResetAt) {
			addTrackerTraffic(counters, info, nil)
		}
	}
	appTrafficLock.Unlock()

	items := make([]appTrafficItem, 0, len(counters))
	for process, counter := range counters {
		upload := counter.upload
		download := counter.download
		if onlyProxy {
			upload = counter.proxyUpload
			download = counter.proxyDownload
		}
		total := upload + download
		if total == 0 {
			continue
		}
		items = append(items, appTrafficItem{
			Process:  process,
			Upload:   upload,
			Download: download,
			Total:    total,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Total != items[j].Total {
			return items[i].Total > items[j].Total
		}
		return items[i].Process < items[j].Process
	})
	return items
}

func handleGetAppTraffic() string {
	items := collectAppTraffic(state.CurrentState.OnlyStatisticsProxy)
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}
