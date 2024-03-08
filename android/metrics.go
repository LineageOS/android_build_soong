// Copyright 2020 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package android

import (
	"bytes"
	"io/ioutil"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/google/blueprint/metrics"
	"google.golang.org/protobuf/proto"

	soong_metrics_proto "android/soong/ui/metrics/metrics_proto"
)

var soongMetricsOnceKey = NewOnceKey("soong metrics")

type soongMetrics struct {
	modules       int
	variants      int
	perfCollector perfCollector
}

type perfCollector struct {
	events []*soong_metrics_proto.PerfCounters
	stop   chan<- bool
}

func getSoongMetrics(config Config) *soongMetrics {
	return config.Once(soongMetricsOnceKey, func() interface{} {
		return &soongMetrics{}
	}).(*soongMetrics)
}

func init() {
	RegisterParallelSingletonType("soong_metrics", soongMetricsSingletonFactory)
}

func soongMetricsSingletonFactory() Singleton { return soongMetricsSingleton{} }

type soongMetricsSingleton struct{}

func (soongMetricsSingleton) GenerateBuildActions(ctx SingletonContext) {
	metrics := getSoongMetrics(ctx.Config())
	ctx.VisitAllModules(func(m Module) {
		if ctx.PrimaryModule(m) == m {
			metrics.modules++
		}
		metrics.variants++
	})
}

func collectMetrics(config Config, eventHandler *metrics.EventHandler) *soong_metrics_proto.SoongBuildMetrics {
	metrics := &soong_metrics_proto.SoongBuildMetrics{}

	soongMetrics := getSoongMetrics(config)
	if soongMetrics.modules > 0 {
		metrics.Modules = proto.Uint32(uint32(soongMetrics.modules))
		metrics.Variants = proto.Uint32(uint32(soongMetrics.variants))
	}

	soongMetrics.perfCollector.stop <- true
	metrics.PerfCounters = soongMetrics.perfCollector.events

	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	metrics.MaxHeapSize = proto.Uint64(memStats.HeapSys)
	metrics.TotalAllocCount = proto.Uint64(memStats.Mallocs)
	metrics.TotalAllocSize = proto.Uint64(memStats.TotalAlloc)

	for _, event := range eventHandler.CompletedEvents() {
		perfInfo := soong_metrics_proto.PerfInfo{
			Description: proto.String(event.Id),
			Name:        proto.String("soong_build"),
			StartTime:   proto.Uint64(uint64(event.Start.UnixNano())),
			RealTime:    proto.Uint64(event.RuntimeNanoseconds()),
		}
		metrics.Events = append(metrics.Events, &perfInfo)
	}

	return metrics
}

func StartBackgroundMetrics(config Config) {
	perfCollector := &getSoongMetrics(config).perfCollector
	stop := make(chan bool)
	perfCollector.stop = stop

	previousTime := time.Now()
	previousCpuTime := readCpuTime()

	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-stop:
				ticker.Stop()
				return
			case <-ticker.C:
				// carry on
			}

			currentTime := time.Now()

			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			currentCpuTime := readCpuTime()

			interval := currentTime.Sub(previousTime)
			intervalCpuTime := currentCpuTime - previousCpuTime
			intervalCpuPercent := intervalCpuTime * 100 / interval

			// heapAlloc is the memory that has been allocated on the heap but not yet GC'd.  It may be referenced,
			// or unrefenced but not yet GC'd.
			heapAlloc := memStats.HeapAlloc
			// heapUnused is the memory that was previously used by the heap, but is currently not used.  It does not
			// count memory that was used and then returned to the OS.
			heapUnused := memStats.HeapIdle - memStats.HeapReleased
			// heapOverhead is the memory used by the allocator and GC
			heapOverhead := memStats.MSpanSys + memStats.MCacheSys + memStats.GCSys
			// otherMem is the memory used outside of the heap.
			otherMem := memStats.Sys - memStats.HeapSys - heapOverhead

			perfCollector.events = append(perfCollector.events, &soong_metrics_proto.PerfCounters{
				Time: proto.Uint64(uint64(currentTime.UnixNano())),
				Groups: []*soong_metrics_proto.PerfCounterGroup{
					{
						Name: proto.String("cpu"),
						Counters: []*soong_metrics_proto.PerfCounter{
							{Name: proto.String("cpu_percent"), Value: proto.Int64(int64(intervalCpuPercent))},
						},
					}, {
						Name: proto.String("memory"),
						Counters: []*soong_metrics_proto.PerfCounter{
							{Name: proto.String("heap_alloc"), Value: proto.Int64(int64(heapAlloc))},
							{Name: proto.String("heap_unused"), Value: proto.Int64(int64(heapUnused))},
							{Name: proto.String("heap_overhead"), Value: proto.Int64(int64(heapOverhead))},
							{Name: proto.String("other"), Value: proto.Int64(int64(otherMem))},
						},
					},
				},
			})

			previousTime = currentTime
			previousCpuTime = currentCpuTime
		}
	}()
}

func readCpuTime() time.Duration {
	if runtime.GOOS != "linux" {
		return 0
	}

	stat, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0
	}

	endOfComm := bytes.LastIndexByte(stat, ')')
	if endOfComm < 0 || endOfComm > len(stat)-2 {
		return 0
	}

	stat = stat[endOfComm+2:]

	statFields := bytes.Split(stat, []byte{' '})
	// This should come from sysconf(_SC_CLK_TCK), but there's no way to call that from Go.  Assume it's 100,
	// which is the value for all platforms we support.
	const HZ = 100
	const MS_PER_HZ = 1e3 / HZ * time.Millisecond

	const STAT_UTIME_FIELD = 14 - 2
	const STAT_STIME_FIELD = 15 - 2
	if len(statFields) < STAT_STIME_FIELD {
		return 0
	}
	userCpuTicks, err := strconv.ParseUint(string(statFields[STAT_UTIME_FIELD]), 10, 64)
	if err != nil {
		return 0
	}
	kernelCpuTicks, _ := strconv.ParseUint(string(statFields[STAT_STIME_FIELD]), 10, 64)
	if err != nil {
		return 0
	}
	return time.Duration(userCpuTicks+kernelCpuTicks) * MS_PER_HZ
}

func WriteMetrics(config Config, eventHandler *metrics.EventHandler, metricsFile string) error {
	metrics := collectMetrics(config, eventHandler)

	buf, err := proto.Marshal(metrics)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(absolutePath(metricsFile), buf, 0666)
	if err != nil {
		return err
	}

	return nil
}
