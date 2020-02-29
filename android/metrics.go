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
	"io/ioutil"
	"runtime"

	"github.com/golang/protobuf/proto"

	soong_metrics_proto "android/soong/ui/metrics/metrics_proto"
)

var soongMetricsOnceKey = NewOnceKey("soong metrics")

type SoongMetrics struct {
	Modules  int
	Variants int
}

func ReadSoongMetrics(config Config) SoongMetrics {
	return config.Get(soongMetricsOnceKey).(SoongMetrics)
}

func init() {
	RegisterSingletonType("soong_metrics", soongMetricsSingletonFactory)
}

func soongMetricsSingletonFactory() Singleton { return soongMetricsSingleton{} }

type soongMetricsSingleton struct{}

func (soongMetricsSingleton) GenerateBuildActions(ctx SingletonContext) {
	metrics := SoongMetrics{}
	ctx.VisitAllModules(func(m Module) {
		if ctx.PrimaryModule(m) == m {
			metrics.Modules++
		}
		metrics.Variants++
	})
	ctx.Config().Once(soongMetricsOnceKey, func() interface{} {
		return metrics
	})
}

func collectMetrics(config Config) *soong_metrics_proto.SoongBuildMetrics {
	metrics := &soong_metrics_proto.SoongBuildMetrics{}

	soongMetrics := ReadSoongMetrics(config)
	metrics.Modules = proto.Uint32(uint32(soongMetrics.Modules))
	metrics.Variants = proto.Uint32(uint32(soongMetrics.Variants))

	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)
	metrics.MaxHeapSize = proto.Uint64(memStats.HeapSys)
	metrics.TotalAllocCount = proto.Uint64(memStats.Mallocs)
	metrics.TotalAllocSize = proto.Uint64(memStats.TotalAlloc)

	return metrics
}

func WriteMetrics(config Config, metricsFile string) error {
	metrics := collectMetrics(config)

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
