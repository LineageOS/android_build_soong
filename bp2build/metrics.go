package bp2build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/shared"
	"android/soong/ui/metrics/bp2build_metrics_proto"
	"google.golang.org/protobuf/proto"

	"github.com/google/blueprint"
)

// CodegenMetrics represents information about the Blueprint-to-BUILD
// conversion process.
// Use CreateCodegenMetrics() to get a properly initialized instance
type CodegenMetrics struct {
	serialized *bp2build_metrics_proto.Bp2BuildMetrics
	// List of modules with unconverted deps
	// NOTE: NOT in the .proto
	moduleWithUnconvertedDepsMsgs []string

	// List of modules with missing deps
	// NOTE: NOT in the .proto
	moduleWithMissingDepsMsgs []string

	// Map of converted modules and paths to call
	// NOTE: NOT in the .proto
	convertedModulePathMap map[string]string
}

func CreateCodegenMetrics() CodegenMetrics {
	return CodegenMetrics{
		serialized: &bp2build_metrics_proto.Bp2BuildMetrics{
			RuleClassCount:           make(map[string]uint64),
			ConvertedModuleTypeCount: make(map[string]uint64),
			TotalModuleTypeCount:     make(map[string]uint64),
		},
		convertedModulePathMap: make(map[string]string),
	}
}

// Serialize returns the protoized version of CodegenMetrics: bp2build_metrics_proto.Bp2BuildMetrics
func (metrics *CodegenMetrics) Serialize() *bp2build_metrics_proto.Bp2BuildMetrics {
	return metrics.serialized
}

// Print the codegen metrics to stdout.
func (metrics *CodegenMetrics) Print() {
	generatedTargetCount := uint64(0)
	for _, ruleClass := range android.SortedStringKeys(metrics.serialized.RuleClassCount) {
		count := metrics.serialized.RuleClassCount[ruleClass]
		fmt.Printf("[bp2build] %s: %d targets\n", ruleClass, count)
		generatedTargetCount += count
	}
	fmt.Printf(
		`[bp2build] Converted %d Android.bp modules to %d total generated BUILD targets. Included %d handcrafted BUILD targets. There are %d total Android.bp modules.
%d converted modules have unconverted deps:
	%s
%d converted modules have missing deps:
	%s
`,
		metrics.serialized.GeneratedModuleCount,
		generatedTargetCount,
		metrics.serialized.HandCraftedModuleCount,
		metrics.TotalModuleCount(),
		len(metrics.moduleWithUnconvertedDepsMsgs),
		strings.Join(metrics.moduleWithUnconvertedDepsMsgs, "\n\t"),
		len(metrics.moduleWithMissingDepsMsgs),
		strings.Join(metrics.moduleWithMissingDepsMsgs, "\n\t"),
	)
}

const bp2buildMetricsFilename = "bp2build_metrics.pb"

// fail prints $PWD to stderr, followed by the given printf string and args (vals),
// then the given alert, and then exits with 1 for failure
func fail(err error, alertFmt string, vals ...interface{}) {
	cwd, wderr := os.Getwd()
	if wderr != nil {
		cwd = "FAILED TO GET $PWD: " + wderr.Error()
	}
	fmt.Fprintf(os.Stderr, "\nIn "+cwd+":\n"+alertFmt+"\n"+err.Error()+"\n", vals...)
	os.Exit(1)
}

// Write the bp2build-protoized codegen metrics into the given directory
func (metrics *CodegenMetrics) Write(dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// The metrics dir doesn't already exist, so create it (and parents)
		if err := os.MkdirAll(dir, 0755); err != nil { // rx for all; w for user
			fail(err, "Failed to `mkdir -p` %s", dir)
		}
	} else if err != nil {
		fail(err, "Failed to `stat` %s", dir)
	}
	metricsFile := filepath.Join(dir, bp2buildMetricsFilename)
	if err := metrics.dump(metricsFile); err != nil {
		fail(err, "Error outputting %s", metricsFile)
	}
	if _, err := os.Stat(metricsFile); err != nil {
		if os.IsNotExist(err) {
			fail(err, "MISSING BP2BUILD METRICS OUTPUT: %s", metricsFile)
		} else {
			fail(err, "FAILED TO `stat` BP2BUILD METRICS OUTPUT: %s", metricsFile)
		}
	}
}

// ReadCodegenMetrics loads CodegenMetrics from `dir`
// returns a nil pointer if the file doesn't exist
func ReadCodegenMetrics(dir string) *CodegenMetrics {
	metricsFile := filepath.Join(dir, bp2buildMetricsFilename)
	if _, err := os.Stat(metricsFile); err != nil {
		if os.IsNotExist(err) {
			return nil
		} else {
			fail(err, "FAILED TO `stat` BP2BUILD METRICS OUTPUT: %s", metricsFile)
			panic("unreachable after fail")
		}
	}
	if buf, err := os.ReadFile(metricsFile); err != nil {
		fail(err, "FAILED TO READ BP2BUILD METRICS OUTPUT: %s", metricsFile)
		panic("unreachable after fail")
	} else {
		bp2BuildMetrics := bp2build_metrics_proto.Bp2BuildMetrics{
			RuleClassCount:           make(map[string]uint64),
			ConvertedModuleTypeCount: make(map[string]uint64),
			TotalModuleTypeCount:     make(map[string]uint64),
		}
		if err := proto.Unmarshal(buf, &bp2BuildMetrics); err != nil {
			fail(err, "FAILED TO PARSE BP2BUILD METRICS OUTPUT: %s", metricsFile)
		}
		return &CodegenMetrics{
			serialized:             &bp2BuildMetrics,
			convertedModulePathMap: make(map[string]string),
		}
	}
}

func (metrics *CodegenMetrics) IncrementRuleClassCount(ruleClass string) {
	metrics.serialized.RuleClassCount[ruleClass] += 1
}

func (metrics *CodegenMetrics) AddEvent(event *bp2build_metrics_proto.Event) {
	metrics.serialized.Events = append(metrics.serialized.Events, event)
}
func (metrics *CodegenMetrics) AddUnconvertedModule(moduleType string) {
	metrics.serialized.UnconvertedModuleCount += 1
	metrics.serialized.TotalModuleTypeCount[moduleType] += 1
}

func (metrics *CodegenMetrics) TotalModuleCount() uint64 {
	return metrics.serialized.HandCraftedModuleCount +
		metrics.serialized.GeneratedModuleCount +
		metrics.serialized.UnconvertedModuleCount
}

// Dump serializes the metrics to the given filename
func (metrics *CodegenMetrics) dump(filename string) (err error) {
	ser := metrics.Serialize()
	return shared.Save(ser, filename)
}

type ConversionType int

const (
	Generated ConversionType = iota
	Handcrafted
)

func (metrics *CodegenMetrics) AddConvertedModule(m blueprint.Module, moduleType string, dir string, conversionType ConversionType) {
	// Undo prebuilt_ module name prefix modifications
	moduleName := android.RemoveOptionalPrebuiltPrefix(m.Name())
	metrics.serialized.ConvertedModules = append(metrics.serialized.ConvertedModules, moduleName)
	metrics.convertedModulePathMap[moduleName] = "//" + dir
	metrics.serialized.ConvertedModuleTypeCount[moduleType] += 1
	metrics.serialized.TotalModuleTypeCount[moduleType] += 1

	if conversionType == Handcrafted {
		metrics.serialized.HandCraftedModuleCount += 1
	} else if conversionType == Generated {
		metrics.serialized.GeneratedModuleCount += 1
	}
}
