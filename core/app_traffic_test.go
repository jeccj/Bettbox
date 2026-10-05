package main

import (
	"testing"
	"time"

	"github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

func newAppTrafficTestInfo(process string, upload, download int64, direct bool) *statistic.TrackerInfo {
	info := &statistic.TrackerInfo{
		Metadata: &constant.Metadata{Process: process},
		IsDirect: direct,
	}
	info.UploadTotal.Store(upload)
	info.DownloadTotal.Store(download)
	return info
}

func TestAddAppTrafficAggregatesProcessAndProxyViews(t *testing.T) {
	counters := make(map[string]appTrafficCounter)
	addTrackerTraffic(counters, newAppTrafficTestInfo("browser", 10, 20, false), nil)
	addTrackerTraffic(counters, newAppTrafficTestInfo("browser", 3, 4, true), nil)

	got, ok := counters["browser"]
	if !ok {
		t.Fatal("expected browser traffic")
	}
	if got.upload != 13 || got.download != 24 {
		t.Fatalf("unexpected all traffic: upload=%d download=%d", got.upload, got.download)
	}
	if got.proxyUpload != 10 || got.proxyDownload != 20 {
		t.Fatalf("unexpected proxy traffic: upload=%d download=%d", got.proxyUpload, got.proxyDownload)
	}
}

func TestAppTrafficUsesUnknownForMissingProcess(t *testing.T) {
	info := newAppTrafficTestInfo("", 1, 2, false)
	if got := appTrafficProcess(info); got != unknownProcess {
		t.Fatalf("expected %q, got %q", unknownProcess, got)
	}
}

func TestAppTrafficKeepsCountedInnerProcess(t *testing.T) {
	counters := make(map[string]appTrafficCounter)
	info := newAppTrafficTestInfo("mihomo", 1, 2, false)
	info.Metadata.Type = constant.INNER
	addTrackerTraffic(counters, info, nil)

	if got := counters["mihomo"]; got.upload != 1 || got.download != 2 {
		t.Fatalf("unexpected inner traffic: upload=%d download=%d", got.upload, got.download)
	}
}

func TestAppTrafficSkipsUncountedInnerTracker(t *testing.T) {
	counters := make(map[string]appTrafficCounter)
	info := newAppTrafficTestInfo("", 1, 2, false)
	info.Metadata.Type = constant.INNER
	addTrackerTraffic(counters, info, nil)

	if len(counters) != 0 {
		t.Fatalf("unexpected uncounted inner traffic: %#v", counters)
	}
}

func TestAppTrafficBaselineLeavesPostResetBytes(t *testing.T) {
	counters := make(map[string]appTrafficCounter)
	info := newAppTrafficTestInfo("browser", 15, 25, false)
	baseline := &trackerTrafficBaseline{upload: 10, download: 20}
	addTrackerTraffic(counters, info, baseline)
	got := counters["browser"]
	if got.upload != 5 || got.download != 5 || got.proxyUpload != 5 || got.proxyDownload != 5 {
		t.Fatalf("unexpected post-reset traffic: %#v", got)
	}
}

func TestHandleResetTrafficClearsAppAggregation(t *testing.T) {
	appTrafficLock.Lock()
	appTraffic = map[string]appTrafficCounter{"browser": {upload: 1}}
	appTrafficBaselines = map[string]trackerTrafficBaseline{"tracker": {upload: 1}}
	appTrafficLock.Unlock()

	handleResetTraffic()

	appTrafficLock.Lock()
	defer appTrafficLock.Unlock()
	if len(appTraffic) != 0 || len(appTrafficBaselines) != 0 {
		t.Fatalf("app traffic was not reset: app=%#v baseline=%#v", appTraffic, appTrafficBaselines)
	}
}

func TestRecordAppTrafficIgnoresDeletedBeforeResetCallback(t *testing.T) {
	appTrafficLock.Lock()
	appTraffic = make(map[string]appTrafficCounter)
	appTrafficBaselines = make(map[string]trackerTrafficBaseline)
	appTrafficResetAt = time.Now()
	resetAt := appTrafficResetAt
	appTrafficLock.Unlock()

	info := newAppTrafficTestInfo("browser", 100, 200, false)
	info.Start = resetAt.Add(-time.Second)
	recordAppTraffic(info)

	appTrafficLock.Lock()
	defer appTrafficLock.Unlock()
	if len(appTraffic) != 0 {
		t.Fatalf("pre-reset callback was counted: %#v", appTraffic)
	}
}

func TestRecordAppTrafficUsesBaselineForDelayedCloseCallback(t *testing.T) {
	appTrafficLock.Lock()
	appTraffic = make(map[string]appTrafficCounter)
	appTrafficResetAt = time.Now()
	resetAt := appTrafficResetAt
	info := newAppTrafficTestInfo("browser", 15, 25, false)
	info.Start = resetAt.Add(-time.Second)
	appTrafficBaselines = map[string]trackerTrafficBaseline{
		info.UUID.String(): {upload: 10, download: 20},
	}
	appTrafficLock.Unlock()

	recordAppTraffic(info)

	appTrafficLock.Lock()
	defer appTrafficLock.Unlock()
	got := appTraffic["browser"]
	if got.upload != 5 || got.download != 5 {
		t.Fatalf("delayed callback used lifetime traffic: %#v", got)
	}
	if len(appTrafficBaselines) != 0 {
		t.Fatalf("tracker baseline was not consumed: %#v", appTrafficBaselines)
	}
}
