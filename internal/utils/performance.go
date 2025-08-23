package utils

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// PerformanceMonitor tracks operation timings
type PerformanceMonitor struct {
	timings    map[string]time.Duration
	startTimes map[string]time.Time
	mu         sync.RWMutex
}

var (
	instance *PerformanceMonitor
	once     sync.Once
)

// GetInstance returns the singleton performance monitor
func GetInstance() *PerformanceMonitor {
	once.Do(func() {
		instance = &PerformanceMonitor{
			timings:    make(map[string]time.Duration),
			startTimes: make(map[string]time.Time),
		}
	})
	return instance
}

// StartTimer starts a timer for the given label
func (pm *PerformanceMonitor) StartTimer(label string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.startTimes[label] = time.Now()
}

// EndTimer ends a timer for the given label and returns the duration
func (pm *PerformanceMonitor) EndTimer(label string) time.Duration {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	startTime, exists := pm.startTimes[label]
	if !exists {
		panic(fmt.Sprintf("Timer \"%s\" was not started", label))
	}

	duration := time.Since(startTime)
	pm.timings[label] = duration
	delete(pm.startTimes, label)
	return duration
}

// GetTiming returns the timing for the given label
func (pm *PerformanceMonitor) GetTiming(label string) (time.Duration, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	timing, exists := pm.timings[label]
	return timing, exists
}

// LogTimings logs all recorded timings
func (pm *PerformanceMonitor) LogTimings() {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.timings) == 0 {
		return
	}

	fmt.Println("\nPerformance timings:")
	for label, duration := range pm.timings {
		fmt.Printf("  %s: %.2fms\n", label, float64(duration.Nanoseconds())/1e6)
	}
	fmt.Println()
}

// Clear clears all timings and start times
func (pm *PerformanceMonitor) Clear() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.timings = make(map[string]time.Duration)
	pm.startTimes = make(map[string]time.Time)
}

// TimeOperation times an operation and returns its result
func TimeOperation[T any](label string, operation func() (T, error)) (T, error) {
	monitor := GetInstance()
	monitor.StartTimer(label)

	defer func() {
		duration := monitor.EndTimer(label)
		if os.Getenv("DEBUG") != "" {
			fmt.Printf("[DEBUG] %s: %.2fms\n", label, float64(duration.Nanoseconds())/1e6)
		}
	}()

	return operation()
}
