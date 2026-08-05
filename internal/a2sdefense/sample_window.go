package a2sdefense

import (
	"net/netip"
	"sort"
	"sync"
	"time"
)

type sampleSeries struct {
	latest     time.Time
	timestamps []time.Time
}

type SampleWindow struct {
	duration time.Duration
	mu       sync.Mutex
	bySource map[netip.Addr]sampleSeries
}

func NewSampleWindow(duration time.Duration) *SampleWindow {
	return &SampleWindow{duration: duration, bySource: make(map[netip.Addr]sampleSeries)}
}

func (w *SampleWindow) Observe(source netip.Addr, timestamp time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	series := w.bySource[source]
	if series.latest.IsZero() || timestamp.After(series.latest) {
		series.latest = timestamp
	}
	cutoff := series.latest.Add(-w.duration)
	first := 0
	for first < len(series.timestamps) && series.timestamps[first].Before(cutoff) {
		first++
	}
	series.timestamps = series.timestamps[first:]
	if !timestamp.Before(cutoff) {
		index := sort.Search(len(series.timestamps), func(index int) bool {
			return !series.timestamps[index].Before(timestamp)
		})
		series.timestamps = append(series.timestamps, time.Time{})
		copy(series.timestamps[index+1:], series.timestamps[index:])
		series.timestamps[index] = timestamp
	}
	w.bySource[source] = series
	for candidate, other := range w.bySource {
		if candidate != source && other.latest.Before(cutoff) {
			delete(w.bySource, candidate)
		}
	}
	return len(series.timestamps)
}
