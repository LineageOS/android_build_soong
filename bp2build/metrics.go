package bp2build

import (
	"fmt"
	"strings"

	"android/soong/android"
)

// Simple metrics struct to collect information about a Blueprint to BUILD
// conversion process.
type CodegenMetrics struct {
	// Total number of Soong modules converted to generated targets
	generatedModuleCount int

	// Total number of Soong modules converted to handcrafted targets
	handCraftedModuleCount int

	// Total number of unconverted Soong modules
	unconvertedModuleCount int

	// Counts of generated Bazel targets per Bazel rule class
	ruleClassCount map[string]int

	moduleWithUnconvertedDepsMsgs []string

	convertedModules []string
}

// Print the codegen metrics to stdout.
func (metrics *CodegenMetrics) Print() {
	generatedTargetCount := 0
	for _, ruleClass := range android.SortedStringKeys(metrics.ruleClassCount) {
		count := metrics.ruleClassCount[ruleClass]
		fmt.Printf("[bp2build] %s: %d targets\n", ruleClass, count)
		generatedTargetCount += count
	}
	fmt.Printf(
		"[bp2build] Converted %d Android.bp modules to %d total generated BUILD targets. Included %d handcrafted BUILD targets. There are %d total Android.bp modules.\n%d converted modules have unconverted deps: \n\t%s",
		metrics.generatedModuleCount,
		generatedTargetCount,
		metrics.handCraftedModuleCount,
		metrics.TotalModuleCount(),
		len(metrics.moduleWithUnconvertedDepsMsgs),
		strings.Join(metrics.moduleWithUnconvertedDepsMsgs, "\n\t"))
}

func (metrics *CodegenMetrics) IncrementRuleClassCount(ruleClass string) {
	metrics.ruleClassCount[ruleClass] += 1
}

func (metrics *CodegenMetrics) IncrementUnconvertedCount() {
	metrics.unconvertedModuleCount += 1
}

func (metrics *CodegenMetrics) TotalModuleCount() int {
	return metrics.handCraftedModuleCount +
		metrics.generatedModuleCount +
		metrics.unconvertedModuleCount
}

type ConversionType int

const (
	Generated ConversionType = iota
	Handcrafted
)

func (metrics *CodegenMetrics) AddConvertedModule(moduleName string, conversionType ConversionType) {
	// Undo prebuilt_ module name prefix modifications
	moduleName = android.RemoveOptionalPrebuiltPrefix(moduleName)
	metrics.convertedModules = append(metrics.convertedModules, moduleName)

	if conversionType == Handcrafted {
		metrics.handCraftedModuleCount += 1
	} else if conversionType == Generated {
		metrics.generatedModuleCount += 1
	}
}
