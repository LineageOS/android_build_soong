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

// Copies all the entries (APKs/APEXes) matching the target configuration from the given
// APK set into a zip file. Run it without arguments to see usage details.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"android/soong/cmd/extract_apks/bundle_proto"
	android_bundle_proto "android/soong/cmd/extract_apks/bundle_proto"
	"android/soong/third_party/zip"
)

type TargetConfig struct {
	sdkVersion int32
	screenDpi  map[android_bundle_proto.ScreenDensity_DensityAlias]bool
	// Map holding <ABI alias>:<its sequence number in the flag> info.
	abis             map[android_bundle_proto.Abi_AbiAlias]int
	allowPrereleased bool
	stem             string
	skipSdkCheck     bool
}

// An APK set is a zip archive. An entry 'toc.pb' describes its contents.
// It is a protobuf message BuildApkResult.
type Toc *android_bundle_proto.BuildApksResult

type ApkSet struct {
	path    string
	reader  *zip.ReadCloser
	entries map[string]*zip.File
}

func newApkSet(path string) (*ApkSet, error) {
	apkSet := &ApkSet{path: path, entries: make(map[string]*zip.File)}
	var err error
	if apkSet.reader, err = zip.OpenReader(apkSet.path); err != nil {
		return nil, err
	}
	for _, f := range apkSet.reader.File {
		apkSet.entries[f.Name] = f
	}
	return apkSet, nil
}

func (apkSet *ApkSet) getToc() (Toc, error) {
	var err error
	tocFile, ok := apkSet.entries["toc.pb"]
	if !ok {
		return nil, fmt.Errorf("%s: APK set should have toc.pb entry", apkSet.path)
	}
	rc, err := tocFile.Open()
	if err != nil {
		return nil, err
	}
	bytes, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	rc.Close()
	buildApksResult := new(android_bundle_proto.BuildApksResult)
	if err = proto.Unmarshal(bytes, buildApksResult); err != nil {
		return nil, err
	}
	return buildApksResult, nil
}

func (apkSet *ApkSet) close() {
	apkSet.reader.Close()
}

// Matchers for selection criteria

type abiTargetingMatcher struct {
	*android_bundle_proto.AbiTargeting
}

func (m abiTargetingMatcher) matches(config TargetConfig) bool {
	if m.AbiTargeting == nil {
		return true
	}
	if _, ok := config.abis[android_bundle_proto.Abi_UNSPECIFIED_CPU_ARCHITECTURE]; ok {
		return true
	}
	// Find the one that appears first in the abis flags.
	abiIdx := math.MaxInt32
	for _, v := range m.GetValue() {
		if i, ok := config.abis[v.Alias]; ok {
			if i < abiIdx {
				abiIdx = i
			}
		}
	}
	if abiIdx == math.MaxInt32 {
		return false
	}
	// See if any alternatives appear before the above one.
	for _, a := range m.GetAlternatives() {
		if i, ok := config.abis[a.Alias]; ok {
			if i < abiIdx {
				// There is a better alternative. Skip this one.
				return false
			}
		}
	}
	return true
}

type apkDescriptionMatcher struct {
	*android_bundle_proto.ApkDescription
}

func (m apkDescriptionMatcher) matches(config TargetConfig, allAbisMustMatch bool) bool {
	return m.ApkDescription == nil || (apkTargetingMatcher{m.Targeting}).matches(config, allAbisMustMatch)
}

type apkTargetingMatcher struct {
	*android_bundle_proto.ApkTargeting
}

func (m apkTargetingMatcher) matches(config TargetConfig, allAbisMustMatch bool) bool {
	return m.ApkTargeting == nil ||
		(abiTargetingMatcher{m.AbiTargeting}.matches(config) &&
			languageTargetingMatcher{m.LanguageTargeting}.matches(config) &&
			screenDensityTargetingMatcher{m.ScreenDensityTargeting}.matches(config) &&
			sdkVersionTargetingMatcher{m.SdkVersionTargeting}.matches(config) &&
			multiAbiTargetingMatcher{m.MultiAbiTargeting}.matches(config, allAbisMustMatch))
}

type languageTargetingMatcher struct {
	*android_bundle_proto.LanguageTargeting
}

func (m languageTargetingMatcher) matches(_ TargetConfig) bool {
	if m.LanguageTargeting == nil {
		return true
	}
	log.Fatal("language based entry selection is not implemented")
	return false
}

type moduleMetadataMatcher struct {
	*android_bundle_proto.ModuleMetadata
}

func (m moduleMetadataMatcher) matches(config TargetConfig) bool {
	return m.ModuleMetadata == nil ||
		(m.GetDeliveryType() == android_bundle_proto.DeliveryType_INSTALL_TIME &&
			moduleTargetingMatcher{m.Targeting}.matches(config) &&
			!m.IsInstant)
}

type moduleTargetingMatcher struct {
	*android_bundle_proto.ModuleTargeting
}

func (m moduleTargetingMatcher) matches(config TargetConfig) bool {
	return m.ModuleTargeting == nil ||
		(sdkVersionTargetingMatcher{m.SdkVersionTargeting}.matches(config) &&
			userCountriesTargetingMatcher{m.UserCountriesTargeting}.matches(config))
}

// A higher number means a higher priority.
// This order must be kept identical to bundletool's.
var multiAbiPriorities = map[android_bundle_proto.Abi_AbiAlias]int{
	android_bundle_proto.Abi_ARMEABI:     1,
	android_bundle_proto.Abi_ARMEABI_V7A: 2,
	android_bundle_proto.Abi_ARM64_V8A:   3,
	android_bundle_proto.Abi_X86:         4,
	android_bundle_proto.Abi_X86_64:      5,
	android_bundle_proto.Abi_MIPS:        6,
	android_bundle_proto.Abi_MIPS64:      7,
}

type multiAbiTargetingMatcher struct {
	*android_bundle_proto.MultiAbiTargeting
}

type multiAbiValue []*bundle_proto.Abi

func (m multiAbiValue) compare(other multiAbiValue) int {
	min := func(a, b int) int {
		if a < b {
			return a
		}
		return b
	}

	sortAbis := func(abiSlice multiAbiValue) func(i, j int) bool {
		return func(i, j int) bool {
			// sort priorities greatest to least
			return multiAbiPriorities[abiSlice[i].Alias] > multiAbiPriorities[abiSlice[j].Alias]
		}
	}

	sortedM := append(multiAbiValue{}, m...)
	sort.Slice(sortedM, sortAbis(sortedM))
	sortedOther := append(multiAbiValue{}, other...)
	sort.Slice(sortedOther, sortAbis(sortedOther))

	for i := 0; i < min(len(sortedM), len(sortedOther)); i++ {
		if multiAbiPriorities[sortedM[i].Alias] > multiAbiPriorities[sortedOther[i].Alias] {
			return 1
		}
		if multiAbiPriorities[sortedM[i].Alias] < multiAbiPriorities[sortedOther[i].Alias] {
			return -1
		}
	}

	return len(sortedM) - len(sortedOther)
}

// this logic should match the logic in bundletool at
// https://github.com/google/bundletool/blob/ae0fc0162fd80d92ef8f4ef4527c066f0106942f/src/main/java/com/android/tools/build/bundletool/device/MultiAbiMatcher.java#L43
// (note link is the commit at time of writing; but logic should always match the latest)
func (t multiAbiTargetingMatcher) matches(config TargetConfig, allAbisMustMatch bool) bool {
	if t.MultiAbiTargeting == nil {
		return true
	}
	if _, ok := config.abis[android_bundle_proto.Abi_UNSPECIFIED_CPU_ARCHITECTURE]; ok {
		return true
	}

	multiAbiIsValid := func(m multiAbiValue) bool {
		numValid := 0
		for _, abi := range m {
			if _, ok := config.abis[abi.Alias]; ok {
				numValid += 1
			}
		}
		if numValid == 0 {
			return false
		} else if numValid > 0 && !allAbisMustMatch {
			return true
		} else {
			return numValid == len(m)
		}
	}

	// ensure that the current value is valid for our config
	valueSetContainsViableAbi := false
	multiAbiSet := t.GetValue()
	for _, multiAbi := range multiAbiSet {
		if multiAbiIsValid(multiAbi.GetAbi()) {
			valueSetContainsViableAbi = true
			break
		}
	}

	if !valueSetContainsViableAbi {
		return false
	}

	// See if there are any matching alternatives with a higher priority.
	for _, altMultiAbi := range t.GetAlternatives() {
		if !multiAbiIsValid(altMultiAbi.GetAbi()) {
			continue
		}

		for _, multiAbi := range multiAbiSet {
			valueAbis := multiAbiValue(multiAbi.GetAbi())
			altAbis := multiAbiValue(altMultiAbi.GetAbi())
			if valueAbis.compare(altAbis) < 0 {
				// An alternative has a higher priority, don't use this one
				return false
			}
		}
	}

	return true
}

type screenDensityTargetingMatcher struct {
	*android_bundle_proto.ScreenDensityTargeting
}

func (m screenDensityTargetingMatcher) matches(config TargetConfig) bool {
	if m.ScreenDensityTargeting == nil {
		return true
	}
	if _, ok := config.screenDpi[android_bundle_proto.ScreenDensity_DENSITY_UNSPECIFIED]; ok {
		return true
	}
	for _, v := range m.GetValue() {
		switch x := v.GetDensityOneof().(type) {
		case *android_bundle_proto.ScreenDensity_DensityAlias_:
			if _, ok := config.screenDpi[x.DensityAlias]; ok {
				return true
			}
		default:
			log.Fatal("For screen density, only DPI name based entry selection (e.g. HDPI, XHDPI) is implemented")
		}
	}
	return false
}

type sdkVersionTargetingMatcher struct {
	*android_bundle_proto.SdkVersionTargeting
}

func (m sdkVersionTargetingMatcher) matches(config TargetConfig) bool {
	const preReleaseVersion = 10000
	// TODO (b274518686) This check should only be used while SHA based targeting is active
	// Once we have switched to an SDK version, this can be changed to throw an error if
	// it was accidentally set
	if config.skipSdkCheck == true {
		return true
	}
	if m.SdkVersionTargeting == nil {
		return true
	}
	if len(m.Value) > 1 {
		log.Fatal(fmt.Sprintf("sdk_version_targeting should not have multiple values:%#v", m.Value))
	}
	// Inspect only sdkVersionTargeting.Value.
	// Even though one of the SdkVersionTargeting.Alternatives values may be
	// better matching, we will select all of them
	return m.Value[0].Min == nil ||
		m.Value[0].Min.Value <= config.sdkVersion ||
		(config.allowPrereleased && m.Value[0].Min.Value == preReleaseVersion)
}

type textureCompressionFormatTargetingMatcher struct {
	*android_bundle_proto.TextureCompressionFormatTargeting
}

func (m textureCompressionFormatTargetingMatcher) matches(_ TargetConfig) bool {
	if m.TextureCompressionFormatTargeting == nil {
		return true
	}
	log.Fatal("texture based entry selection is not implemented")
	return false
}

type userCountriesTargetingMatcher struct {
	*android_bundle_proto.UserCountriesTargeting
}

func (m userCountriesTargetingMatcher) matches(_ TargetConfig) bool {
	if m.UserCountriesTargeting == nil {
		return true
	}
	log.Fatal("country based entry selection is not implemented")
	return false
}

type variantTargetingMatcher struct {
	*android_bundle_proto.VariantTargeting
}

func (m variantTargetingMatcher) matches(config TargetConfig, allAbisMustMatch bool) bool {
	if m.VariantTargeting == nil {
		return true
	}
	return sdkVersionTargetingMatcher{m.SdkVersionTargeting}.matches(config) &&
		abiTargetingMatcher{m.AbiTargeting}.matches(config) &&
		multiAbiTargetingMatcher{m.MultiAbiTargeting}.matches(config, allAbisMustMatch) &&
		screenDensityTargetingMatcher{m.ScreenDensityTargeting}.matches(config) &&
		textureCompressionFormatTargetingMatcher{m.TextureCompressionFormatTargeting}.matches(config)
}

type SelectionResult struct {
	moduleName string
	entries    []string
}

// Return all entries matching target configuration
func selectApks(toc Toc, targetConfig TargetConfig) SelectionResult {
	checkMatching := func(allAbisMustMatch bool) SelectionResult {
		var result SelectionResult
		for _, variant := range (*toc).GetVariant() {
			if !(variantTargetingMatcher{variant.GetTargeting()}.matches(targetConfig, allAbisMustMatch)) {
				continue
			}
			for _, as := range variant.GetApkSet() {
				if !(moduleMetadataMatcher{as.ModuleMetadata}.matches(targetConfig)) {
					continue
				}
				for _, apkdesc := range as.GetApkDescription() {
					if (apkDescriptionMatcher{apkdesc}).matches(targetConfig, allAbisMustMatch) {
						result.entries = append(result.entries, apkdesc.GetPath())
						// TODO(asmundak): As it turns out, moduleName which we get from
						// the ModuleMetadata matches the module names of the generated
						// entry paths just by coincidence, only for the split APKs. We
						// need to discuss this with bundletool folks.
						result.moduleName = as.GetModuleMetadata().GetName()
					}
				}
				// we allow only a single module, so bail out here if we found one
				if result.moduleName != "" {
					return result
				}
			}
		}
		return result
	}
	result := checkMatching(true)
	if result.moduleName == "" {
		// if there are no matches where all of the ABIs are available in the
		// TargetConfig, then search again with a looser requirement of at
		// least one matching ABI
		// NOTE(b/260130686): this logic diverges from the logic in bundletool
		// https://github.com/google/bundletool/blob/ae0fc0162fd80d92ef8f4ef4527c066f0106942f/src/main/java/com/android/tools/build/bundletool/device/MultiAbiMatcher.java#L43
		result = checkMatching(false)
	}
	return result
}

type Zip2ZipWriter interface {
	CopyFrom(file *zip.File, name string) error
}

// Writes out selected entries, renaming them as needed
func (apkSet *ApkSet) writeApks(selected SelectionResult, config TargetConfig,
	outFile io.Writer, zipWriter Zip2ZipWriter, partition string) ([]string, error) {
	// Renaming rules:
	//  splits/MODULE-master.apk to STEM.apk
	// else
	//  splits/MODULE-*.apk to STEM>-$1.apk
	// TODO(asmundak):
	//  add more rules, for .apex files
	renameRules := []struct {
		rex  *regexp.Regexp
		repl string
	}{
		{
			regexp.MustCompile(`^.*/` + selected.moduleName + `-master\.apk$`),
			config.stem + `.apk`,
		},
		{
			regexp.MustCompile(`^.*/` + selected.moduleName + `(-.*\.apk)$`),
			config.stem + `$1`,
		},
		{
			regexp.MustCompile(`^universal\.apk$`),
			config.stem + ".apk",
		},
	}
	renamer := func(path string) (string, bool) {
		for _, rr := range renameRules {
			if rr.rex.MatchString(path) {
				return rr.rex.ReplaceAllString(path, rr.repl), true
			}
		}
		return "", false
	}

	entryOrigin := make(map[string]string) // output entry to input entry
	var apkcerts []string
	for _, apk := range selected.entries {
		apkFile, ok := apkSet.entries[apk]
		if !ok {
			return nil, fmt.Errorf("TOC refers to an entry %s which does not exist", apk)
		}
		inName := apkFile.Name
		outName, ok := renamer(inName)
		if !ok {
			log.Fatalf("selected an entry with unexpected name %s", inName)
		}
		if origin, ok := entryOrigin[inName]; ok {
			log.Fatalf("selected entries %s and %s will have the same output name %s",
				origin, inName, outName)
		}
		entryOrigin[outName] = inName
		if outName == config.stem+".apk" {
			if err := writeZipEntryToFile(outFile, apkFile); err != nil {
				return nil, err
			}
		} else {
			if err := zipWriter.CopyFrom(apkFile, outName); err != nil {
				return nil, err
			}
		}
		if partition != "" {
			apkcerts = append(apkcerts, fmt.Sprintf(
				`name="%s" certificate="PRESIGNED" private_key="" partition="%s"`, outName, partition))
		}
	}
	sort.Strings(apkcerts)
	return apkcerts, nil
}

func (apkSet *ApkSet) extractAndCopySingle(selected SelectionResult, outFile *os.File) error {
	if len(selected.entries) != 1 {
		return fmt.Errorf("Too many matching entries for extract-single:\n%v", selected.entries)
	}
	apk, ok := apkSet.entries[selected.entries[0]]
	if !ok {
		return fmt.Errorf("Couldn't find apk path %s", selected.entries[0])
	}
	return writeZipEntryToFile(outFile, apk)
}

// Arguments parsing
var (
	outputFile   = flag.String("o", "", "output file for primary entry")
	zipFile      = flag.String("zip", "", "output file containing additional extracted entries")
	targetConfig = TargetConfig{
		screenDpi: map[android_bundle_proto.ScreenDensity_DensityAlias]bool{},
		abis:      map[android_bundle_proto.Abi_AbiAlias]int{},
	}
	extractSingle = flag.Bool("extract-single", false,
		"extract a single target and output it uncompressed. only available for standalone apks and apexes.")
	apkcertsOutput = flag.String("apkcerts", "",
		"optional apkcerts.txt output file containing signing info of all outputted apks")
	partition = flag.String("partition", "", "partition string. required when -apkcerts is used.")
)

// Parse abi values
type abiFlagValue struct {
	targetConfig *TargetConfig
}

func (a abiFlagValue) String() string {
	return "all"
}

func (a abiFlagValue) Set(abiList string) error {
	for i, abi := range strings.Split(abiList, ",") {
		v, ok := android_bundle_proto.Abi_AbiAlias_value[abi]
		if !ok {
			return fmt.Errorf("bad ABI value: %q", abi)
		}
		targetConfig.abis[android_bundle_proto.Abi_AbiAlias(v)] = i
	}
	return nil
}

// Parse screen density values
type screenDensityFlagValue struct {
	targetConfig *TargetConfig
}

func (s screenDensityFlagValue) String() string {
	return "none"
}

func (s screenDensityFlagValue) Set(densityList string) error {
	if densityList == "none" {
		return nil
	}
	if densityList == "all" {
		targetConfig.screenDpi[android_bundle_proto.ScreenDensity_DENSITY_UNSPECIFIED] = true
		return nil
	}
	for _, density := range strings.Split(densityList, ",") {
		v, found := android_bundle_proto.ScreenDensity_DensityAlias_value[density]
		if !found {
			return fmt.Errorf("bad screen density value: %q", density)
		}
		targetConfig.screenDpi[android_bundle_proto.ScreenDensity_DensityAlias(v)] = true
	}
	return nil
}

func processArgs() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: extract_apks -o <output-file> [-zip <output-zip-file>] `+
			`-sdk-version value -abis value [-skip-sdk-check]`+
			`-screen-densities value {-stem value | -extract-single} [-allow-prereleased] `+
			`[-apkcerts <apkcerts output file> -partition <partition>] <APK set>`)
		flag.PrintDefaults()
		os.Exit(2)
	}
	version := flag.Uint("sdk-version", 0, "SDK version")
	flag.Var(abiFlagValue{&targetConfig}, "abis",
		"comma-separated ABIs list of ARMEABI ARMEABI_V7A ARM64_V8A X86 X86_64 MIPS MIPS64")
	flag.Var(screenDensityFlagValue{&targetConfig}, "screen-densities",
		"'all' or comma-separated list of screen density names (NODPI LDPI MDPI TVDPI HDPI XHDPI XXHDPI XXXHDPI)")
	flag.BoolVar(&targetConfig.allowPrereleased, "allow-prereleased", false,
		"allow prereleased")
	flag.BoolVar(&targetConfig.skipSdkCheck, "skip-sdk-check", false, "Skip the SDK version check")
	flag.StringVar(&targetConfig.stem, "stem", "", "output entries base name in the output zip file")
	flag.Parse()
	if (*outputFile == "") || len(flag.Args()) != 1 || *version == 0 ||
		((targetConfig.stem == "" || *zipFile == "") && !*extractSingle) ||
		(*apkcertsOutput != "" && *partition == "") {
		flag.Usage()
	}
	targetConfig.sdkVersion = int32(*version)

}

func main() {
	processArgs()
	var toc Toc
	apkSet, err := newApkSet(flag.Arg(0))
	if err == nil {
		defer apkSet.close()
		toc, err = apkSet.getToc()
	}
	if err != nil {
		log.Fatal(err)
	}
	sel := selectApks(toc, targetConfig)
	if len(sel.entries) == 0 {
		log.Fatalf("there are no entries for the target configuration: %#v", targetConfig)
	}

	outFile, err := os.Create(*outputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer outFile.Close()

	if *extractSingle {
		err = apkSet.extractAndCopySingle(sel, outFile)
	} else {
		zipOutputFile, err := os.Create(*zipFile)
		if err != nil {
			log.Fatal(err)
		}
		defer zipOutputFile.Close()

		zipWriter := zip.NewWriter(zipOutputFile)
		defer func() {
			if err := zipWriter.Close(); err != nil {
				log.Fatal(err)
			}
		}()

		apkcerts, err := apkSet.writeApks(sel, targetConfig, outFile, zipWriter, *partition)
		if err == nil && *apkcertsOutput != "" {
			apkcertsFile, err := os.Create(*apkcertsOutput)
			if err != nil {
				log.Fatal(err)
			}
			defer apkcertsFile.Close()
			for _, a := range apkcerts {
				_, err = apkcertsFile.WriteString(a + "\n")
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}
	if err != nil {
		log.Fatal(err)
	}
}

func writeZipEntryToFile(outFile io.Writer, zipEntry *zip.File) error {
	reader, err := zipEntry.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = io.Copy(outFile, reader)
	return err
}
