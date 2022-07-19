package bp2build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/shared"
	"android/soong/ui/metrics/bp2build_metrics_proto"
	"github.com/google/blueprint"
)

// Simple metrics struct to collect information about a Blueprint to BUILD
// conversion process.
type CodegenMetrics struct {
	// Total number of Soong modules converted to generated targets
	generatedModuleCount uint64

	// Total number of Soong modules converted to handcrafted targets
	handCraftedModuleCount uint64

	// Total number of unconverted Soong modules
	unconvertedModuleCount uint64

	// Counts of generated Bazel targets per Bazel rule class
	ruleClassCount map[string]uint64

	// List of modules with unconverted deps
	// NOTE: NOT in the .proto
	moduleWithUnconvertedDepsMsgs []string

	// List of modules with missing deps
	// NOTE: NOT in the .proto
	moduleWithMissingDepsMsgs []string

	// List of converted modules
	convertedModules []string

	// Counts of converted modules by module type.
	convertedModuleTypeCount map[string]uint64

	// Counts of total modules by module type.
	totalModuleTypeCount map[string]uint64

	Events []*bp2build_metrics_proto.Event
}

// Serialize returns the protoized version of CodegenMetrics: bp2build_metrics_proto.Bp2BuildMetrics
func (metrics *CodegenMetrics) Serialize() bp2build_metrics_proto.Bp2BuildMetrics {
	return bp2build_metrics_proto.Bp2BuildMetrics{
		GeneratedModuleCount:     metrics.generatedModuleCount,
		HandCraftedModuleCount:   metrics.handCraftedModuleCount,
		UnconvertedModuleCount:   metrics.unconvertedModuleCount,
		RuleClassCount:           metrics.ruleClassCount,
		ConvertedModules:         metrics.convertedModules,
		ConvertedModuleTypeCount: metrics.convertedModuleTypeCount,
		TotalModuleTypeCount:     metrics.totalModuleTypeCount,
		Events:                   metrics.Events,
	}
}

// Print the codegen metrics to stdout.
func (metrics *CodegenMetrics) Print() {
	generatedTargetCount := uint64(0)
	for _, ruleClass := range android.SortedStringKeys(metrics.ruleClassCount) {
		count := metrics.ruleClassCount[ruleClass]
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
		metrics.generatedModuleCount,
		generatedTargetCount,
		metrics.handCraftedModuleCount,
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
		fail(err, "MISSING BP2BUILD METRICS OUTPUT: Failed to `stat` %s", metricsFile)
	}
}

func (metrics *CodegenMetrics) IncrementRuleClassCount(ruleClass string) {
	metrics.ruleClassCount[ruleClass] += 1
}

func (metrics *CodegenMetrics) AddUnconvertedModule(moduleType string) {
	metrics.unconvertedModuleCount += 1
	metrics.totalModuleTypeCount[moduleType] += 1
}

func (metrics *CodegenMetrics) TotalModuleCount() uint64 {
	return metrics.handCraftedModuleCount +
		metrics.generatedModuleCount +
		metrics.unconvertedModuleCount
}

// Dump serializes the metrics to the given filename
func (metrics *CodegenMetrics) dump(filename string) (err error) {
	ser := metrics.Serialize()
	return shared.Save(&ser, filename)
}

type ConversionType int

const (
	Generated ConversionType = iota
	Handcrafted
)

func (metrics *CodegenMetrics) AddConvertedModule(m blueprint.Module, moduleType string, conversionType ConversionType) {
	// Undo prebuilt_ module name prefix modifications
	moduleName := android.RemoveOptionalPrebuiltPrefix(m.Name())
	metrics.convertedModules = append(metrics.convertedModules, moduleName)
	metrics.convertedModuleTypeCount[moduleType] += 1
	metrics.totalModuleTypeCount[moduleType] += 1

	if conversionType == Handcrafted {
		metrics.handCraftedModuleCount += 1
	} else if conversionType == Generated {
		metrics.generatedModuleCount += 1
	}
}
