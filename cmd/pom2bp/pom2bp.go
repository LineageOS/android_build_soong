// Copyright 2017 Google Inc. All rights reserved.
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

package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/google/blueprint/proptools"

	"android/soong/bpfix/bpfix"
)

type RewriteNames []RewriteName
type RewriteName struct {
	regexp *regexp.Regexp
	repl   string
}

func (r *RewriteNames) String() string {
	return ""
}

func (r *RewriteNames) Set(v string) error {
	split := strings.SplitN(v, "=", 2)
	if len(split) != 2 {
		return fmt.Errorf("Must be in the form of <regex>=<replace>")
	}
	regex, err := regexp.Compile(split[0])
	if err != nil {
		return nil
	}
	*r = append(*r, RewriteName{
		regexp: regex,
		repl:   split[1],
	})
	return nil
}

func (r *RewriteNames) MavenToBp(groupId string, artifactId string) string {
	for _, r := range *r {
		if r.regexp.MatchString(groupId + ":" + artifactId) {
			return r.regexp.ReplaceAllString(groupId+":"+artifactId, r.repl)
		} else if r.regexp.MatchString(artifactId) {
			return r.regexp.ReplaceAllString(artifactId, r.repl)
		}
	}
	return artifactId
}

var rewriteNames = RewriteNames{}

type ExtraDeps map[string][]string

func (d ExtraDeps) String() string {
	return ""
}

func (d ExtraDeps) Set(v string) error {
	split := strings.SplitN(v, "=", 2)
	if len(split) != 2 {
		return fmt.Errorf("Must be in the form of <module>=<module>[,<module>]")
	}
	d[split[0]] = strings.Split(split[1], ",")
	return nil
}

var extraStaticLibs = make(ExtraDeps)

var extraLibs = make(ExtraDeps)

var optionalUsesLibs = make(ExtraDeps)

type Exclude map[string]bool

func (e Exclude) String() string {
	return ""
}

func (e Exclude) Set(v string) error {
	e[v] = true
	return nil
}

var excludes = make(Exclude)

type HostModuleNames map[string]bool

func (n HostModuleNames) IsHostModule(groupId string, artifactId string) bool {
	_, found := n[groupId+":"+artifactId]
	return found
}

func (n HostModuleNames) String() string {
	return ""
}

func (n HostModuleNames) Set(v string) error {
	n[v] = true
	return nil
}

var hostModuleNames = HostModuleNames{}

type HostAndDeviceModuleNames map[string]bool

func (n HostAndDeviceModuleNames) IsHostAndDeviceModule(groupId string, artifactId string) bool {
	_, found := n[groupId+":"+artifactId]

	return found
}

func (n HostAndDeviceModuleNames) String() string {
	return ""
}

func (n HostAndDeviceModuleNames) Set(v string) error {
	n[v] = true
	return nil
}

var hostAndDeviceModuleNames = HostAndDeviceModuleNames{}

var sdkVersion string
var defaultMinSdkVersion string
var useVersion string
var staticDeps bool
var writeCmd bool
var jetifier bool

func InList(s string, list []string) bool {
	for _, l := range list {
		if l == s {
			return true
		}
	}

	return false
}

type Dependency struct {
	XMLName xml.Name `xml:"dependency"`

	BpTarget    string `xml:"-"`
	BazelTarget string `xml:"-"`

	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
	Type       string `xml:"type"`
	Scope      string `xml:"scope"`
}

func (d Dependency) BpName() string {
	if d.BpTarget == "" {
		d.BpTarget = rewriteNames.MavenToBp(d.GroupId, d.ArtifactId)
	}
	return d.BpTarget
}

type Pom struct {
	XMLName xml.Name `xml:"http://maven.apache.org/POM/4.0.0 project"`

	PomFile       string `xml:"-"`
	ArtifactFile  string `xml:"-"`
	BpTarget      string `xml:"-"`
	MinSdkVersion string `xml:"-"`

	GroupId    string `xml:"groupId"`
	ArtifactId string `xml:"artifactId"`
	Version    string `xml:"version"`
	Packaging  string `xml:"packaging"`

	Dependencies []*Dependency `xml:"dependencies>dependency"`
}

func (p Pom) IsAar() bool {
	return p.Packaging == "aar"
}

func (p Pom) IsJar() bool {
	return p.Packaging == "jar"
}

func (p Pom) IsApk() bool {
	return p.Packaging == "apk"
}

func (p Pom) IsHostModule() bool {
	return hostModuleNames.IsHostModule(p.GroupId, p.ArtifactId)
}

func (p Pom) IsDeviceModule() bool {
	return !p.IsHostModule()
}

func (p Pom) IsHostAndDeviceModule() bool {
	return hostAndDeviceModuleNames.IsHostAndDeviceModule(p.GroupId, p.ArtifactId)
}

func (p Pom) IsHostOnly() bool {
	return p.IsHostModule() && !p.IsHostAndDeviceModule()
}

func (p Pom) ModuleType() string {
	if p.IsAar() {
		return "android_library"
	} else if p.IsHostOnly() {
		return "java_library_host"
	} else {
		return "java_library_static"
	}
}

func (p Pom) BazelTargetType() string {
	if p.IsAar() {
		return "android_library"
	} else {
		return "java_library"
	}
}

func (p Pom) ImportModuleType() string {
	if p.IsAar() {
		return "android_library_import"
	} else if p.IsApk() {
		return "android_app_import"
	} else if p.IsHostOnly() {
		return "java_import_host"
	} else {
		return "java_import"
	}
}

func (p Pom) BazelImportTargetType() string {
	if p.IsAar() {
		return "aar_import"
	} else if p.IsApk() {
		return "apk_import"
	} else {
		return "java_import"
	}
}

func (p Pom) ImportProperty() string {
	if p.IsAar() {
		return "aars"
	} else if p.IsApk() {
		return "apk"
	} else {
		return "jars"
	}
}

func (p Pom) BazelImportProperty() string {
	if p.IsAar() {
		return "aar"
	} else if p.IsApk() {
		return "apk"
	} else {
		return "jars"
	}
}

func (p Pom) BpName() string {
	if p.BpTarget == "" {
		p.BpTarget = rewriteNames.MavenToBp(p.GroupId, p.ArtifactId)
	}
	return p.BpTarget
}

func (p Pom) BpJarDeps() []string {
	return p.BpDeps("jar", []string{"compile", "runtime"})
}

func (p Pom) BpAarDeps() []string {
	return p.BpDeps("aar", []string{"compile", "runtime"})
}

func (p Pom) BazelJarDeps() []string {
	return p.BazelDeps("jar", []string{"compile", "runtime"})
}

func (p Pom) BazelAarDeps() []string {
	return p.BazelDeps("aar", []string{"compile", "runtime"})
}

func (p Pom) BpExtraStaticLibs() []string {
	return extraStaticLibs[p.BpName()]
}

func (p Pom) BpExtraLibs() []string {
	return extraLibs[p.BpName()]
}

func (p Pom) BpOptionalUsesLibs() []string {
	return optionalUsesLibs[p.BpName()]
}

// BpDeps obtains dependencies filtered by type and scope. The results of this
// method are formatted as Android.bp targets, e.g. run through MavenToBp rules.
func (p Pom) BpDeps(typeExt string, scopes []string) []string {
	var ret []string
	for _, d := range p.Dependencies {
		if d.Type != typeExt || !InList(d.Scope, scopes) {
			continue
		}
		name := rewriteNames.MavenToBp(d.GroupId, d.ArtifactId)
		ret = append(ret, name)
	}
	return ret
}

// BazelDeps obtains dependencies filtered by type and scope. The results of this
// method are formatted as Bazel BUILD targets.
func (p Pom) BazelDeps(typeExt string, scopes []string) []string {
	var ret []string
	for _, d := range p.Dependencies {
		if d.Type != typeExt || !InList(d.Scope, scopes) {
			continue
		}
		ret = append(ret, d.BazelTarget)
	}
	return ret
}

func PathModVars() (string, string, string) {
	cmd := "/bin/bash"
	androidTop := os.Getenv("ANDROID_BUILD_TOP")
	envSetupSh := path.Join(androidTop, "build/envsetup.sh")
	return cmd, androidTop, envSetupSh
}

func InitRefreshMod(poms []*Pom) error {
	cmd, _, envSetupSh := PathModVars()
	// refreshmod is expensive, so if pathmod is already working we can skip it.
	_, err := exec.Command(cmd, "-c", ". "+envSetupSh+" && pathmod "+poms[0].BpName()).Output()
	if exitErr, _ := err.(*exec.ExitError); exitErr != nil || err != nil {
		_, err := exec.Command(cmd, "-c", ". "+envSetupSh+" && refreshmod").Output()
		if exitErr, _ := err.(*exec.ExitError); exitErr != nil {
			return fmt.Errorf("failed to run %s\n%s\ntry running lunch.", cmd, string(exitErr.Stderr))
		} else if err != nil {
			return err
		}
	}
	return nil
}

func BazelifyExtraDeps(extraDeps ExtraDeps, modules map[string]*Pom) error {
	for _, deps := range extraDeps {
		for _, dep := range deps {
			bazelName, err := BpNameToBazelTarget(dep, modules)
			if err != nil {
				return err
			}
			dep = bazelName
		}

	}
	return nil
}

func (p *Pom) GetBazelDepNames(modules map[string]*Pom) error {
	for _, d := range p.Dependencies {
		bazelName, err := BpNameToBazelTarget(d.BpName(), modules)
		if err != nil {
			return err
		}
		d.BazelTarget = bazelName
	}
	return nil
}

func BpNameToBazelTarget(bpName string, modules map[string]*Pom) (string, error) {
	cmd, androidTop, envSetupSh := PathModVars()

	if _, ok := modules[bpName]; ok {
		// We've seen the POM for this dependency, it will be local to the output BUILD file
		return ":" + bpName, nil
	} else {
		// we don't have the POM for this artifact, find and use the fully qualified target name.
		output, err := exec.Command(cmd, "-c", ". "+envSetupSh+" && pathmod "+bpName).Output()
		if exitErr, _ := err.(*exec.ExitError); exitErr != nil {
			return "", fmt.Errorf("failed to run %s %s\n%s", cmd, bpName, string(exitErr.Stderr))
		} else if err != nil {
			return "", err
		}
		relPath := ""
		for _, line := range strings.Fields(string(output)) {
			if strings.Contains(line, androidTop) {
				relPath = strings.TrimPrefix(line, androidTop)
				relPath = strings.TrimLeft(relPath, "/")
			}
		}
		return "//" + relPath + ":" + bpName, nil
	}
}

func (p Pom) SdkVersion() string {
	return sdkVersion
}

func (p Pom) DefaultMinSdkVersion() string {
	return defaultMinSdkVersion
}

func (p Pom) Jetifier() bool {
	return jetifier
}

func (p *Pom) FixDeps(modules map[string]*Pom) {
	for _, d := range p.Dependencies {
		if d.Type == "" {
			if depPom, ok := modules[d.BpName()]; ok {
				// We've seen the POM for this dependency, use its packaging
				// as the dependency type rather than Maven spec default.
				d.Type = depPom.Packaging
			} else {
				// Dependency type was not specified and we don't have the POM
				// for this artifact, use the default from Maven spec.
				d.Type = "jar"
			}
		}
		if d.Scope == "" {
			// Scope was not specified, use the default from Maven spec.
			d.Scope = "compile"
		}
	}
}

// ExtractMinSdkVersion extracts the minSdkVersion from the AndroidManifest.xml file inside an aar file, or sets it
// to "current" if it is not present.
func (p *Pom) ExtractMinSdkVersion() error {
	aar, err := zip.OpenReader(p.ArtifactFile)
	if err != nil {
		return err
	}
	defer aar.Close()

	var manifest *zip.File
	for _, f := range aar.File {
		if f.Name == "AndroidManifest.xml" {
			manifest = f
			break
		}
	}

	if manifest == nil {
		return fmt.Errorf("failed to find AndroidManifest.xml in %s", p.ArtifactFile)
	}

	r, err := manifest.Open()
	if err != nil {
		return err
	}
	defer r.Close()

	decoder := xml.NewDecoder(r)

	manifestData := struct {
		XMLName  xml.Name `xml:"manifest"`
		Uses_sdk struct {
			MinSdkVersion string `xml:"http://schemas.android.com/apk/res/android minSdkVersion,attr"`
		} `xml:"uses-sdk"`
	}{}

	err = decoder.Decode(&manifestData)
	if err != nil {
		return err
	}

	p.MinSdkVersion = manifestData.Uses_sdk.MinSdkVersion
	if p.MinSdkVersion == "" {
		p.MinSdkVersion = "current"
	}

	return nil
}

var bpTemplate = template.Must(template.New("bp").Parse(`
{{.ImportModuleType}} {
    name: "{{.BpName}}",
    {{- if .IsApk}}
    {{.ImportProperty}}: "{{.ArtifactFile}}",
    {{- else}}
    {{.ImportProperty}}: ["{{.ArtifactFile}}"],
    sdk_version: "{{.SdkVersion}}",
    {{- end}}
    {{- if .Jetifier}}
    jetifier: true,
    {{- end}}
    {{- if .IsHostAndDeviceModule}}
    host_supported: true,
    {{- end}}
    {{- if not .IsHostOnly}}
    apex_available: [
        "//apex_available:platform",
        "//apex_available:anyapex",
    ],
    {{- end}}
    {{- if .IsAar}}
    min_sdk_version: "{{.MinSdkVersion}}",
    static_libs: [
        {{- range .BpJarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpAarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpExtraStaticLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- if .BpExtraLibs}}
    libs: [
        {{- range .BpExtraLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
    {{- if .BpOptionalUsesLibs}}
    optional_uses_libs: [
        {{- range .BpOptionalUsesLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
    {{- else if not .IsHostOnly}}
    {{- if not .IsApk}}
    min_sdk_version: "{{.DefaultMinSdkVersion}}",
    {{- end}}
    {{- end}}
    {{- if .IsApk}}
    preprocessed: true,
    presigned: true,
    {{- end}}

}
`))

var bpDepsTemplate = template.Must(template.New("bp").Parse(`
{{.ImportModuleType}} {
    name: "{{.BpName}}-nodeps",
    {{.ImportProperty}}: ["{{.ArtifactFile}}"],
    sdk_version: "{{.SdkVersion}}",
    {{- if .Jetifier}}
    jetifier: true,
    {{- end}}
    {{- if .IsHostAndDeviceModule}}
    host_supported: true,
    {{- end}}
    {{- if not .IsHostOnly}}
    apex_available: [
        "//apex_available:platform",
        "//apex_available:anyapex",
    ],
    {{- end}}
    {{- if .IsAar}}
    min_sdk_version: "{{.MinSdkVersion}}",
    static_libs: [
        {{- range .BpJarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpAarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpExtraStaticLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- if .BpExtraLibs}}
    libs: [
        {{- range .BpExtraLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
    {{- else if not .IsHostOnly}}
    min_sdk_version: "{{.DefaultMinSdkVersion}}",
    {{- end}}
}

{{.ModuleType}} {
    name: "{{.BpName}}",
    {{- if .IsDeviceModule}}
    sdk_version: "{{.SdkVersion}}",
    {{- if .IsHostAndDeviceModule}}
    host_supported: true,
    {{- end}}
    {{- if not .IsHostOnly}}
    apex_available: [
        "//apex_available:platform",
        "//apex_available:anyapex",
    ],
    {{- end}}
    {{- if .IsAar}}
    min_sdk_version: "{{.MinSdkVersion}}",
    manifest: "manifests/{{.BpName}}/AndroidManifest.xml",
    {{- else if not .IsHostOnly}}
    min_sdk_version: "{{.DefaultMinSdkVersion}}",
    {{- end}}
    {{- end}}
    static_libs: [
        "{{.BpName}}-nodeps",
        {{- range .BpJarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpAarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpExtraStaticLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- if .BpExtraLibs}}
    libs: [
        {{- range .BpExtraLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
    {{- if .BpOptionalUsesLibs}}
    optional_uses_libs: [
        {{- range .BpOptionalUsesLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
    java_version: "1.8",
}
`))

var bazelTemplate = template.Must(template.New("bp").Parse(`
{{.BazelImportTargetType}} (
    name = "{{.BpName}}",
    {{.BazelImportProperty}}: {{- if not .IsAar}}[{{- end}}"{{.ArtifactFile}}"{{- if not .IsAar}}]{{- end}},
    visibility = ["//visibility:public"],
    {{- if .IsAar}}
    deps = [
        {{- range .BazelJarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BazelAarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpExtraStaticLibs}}
        "{{.}}",
        {{- end}}
        {{- range .BpExtraLibs}}
        "{{.}}",
        {{- end}}
        {{- range .BpOptionalUsesLibs}}
        "{{.}}",
        {{- end}}
    ],
    {{- end}}
)
`))

var bazelDepsTemplate = template.Must(template.New("bp").Parse(`
{{.BazelImportTargetType}} (
    name = "{{.BpName}}",
    {{.BazelImportProperty}} = {{- if not .IsAar}}[{{- end}}"{{.ArtifactFile}}"{{- if not .IsAar}}]{{- end}},
    visibility = ["//visibility:public"],
    exports = [
        {{- range .BazelJarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BazelAarDeps}}
        "{{.}}",
        {{- end}}
        {{- range .BpExtraStaticLibs}}
        "{{.}}",
        {{- end}}
        {{- range .BpExtraLibs}}
        "{{.}}",
        {{- end}}
        {{- range .BpOptionalUsesLibs}}
        "{{.}}",
        {{- end}}
    ],
)
`))

func parse(filename string) (*Pom, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var pom Pom
	err = xml.Unmarshal(data, &pom)
	if err != nil {
		return nil, err
	}

	if useVersion != "" && pom.Version != useVersion {
		return nil, nil
	}

	if pom.Packaging == "" {
		pom.Packaging = "jar"
	}

	pom.PomFile = filename
	pom.ArtifactFile = strings.TrimSuffix(filename, ".pom") + "." + pom.Packaging

	return &pom, nil
}

func rerunForRegen(filename string) error {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(bytes.NewBuffer(buf))

	// Skip the first line in the file
	for i := 0; i < 2; i++ {
		if !scanner.Scan() {
			if scanner.Err() != nil {
				return scanner.Err()
			} else {
				return fmt.Errorf("unexpected EOF")
			}
		}
	}

	// Extract the old args from the file
	line := scanner.Text()
	if strings.HasPrefix(line, "// pom2bp ") { // .bp file
		line = strings.TrimPrefix(line, "// pom2bp ")
	} else if strings.HasPrefix(line, "// pom2mk ") { // .bp file converted from .mk file
		line = strings.TrimPrefix(line, "// pom2mk ")
	} else if strings.HasPrefix(line, "# pom2mk ") { // .mk file
		line = strings.TrimPrefix(line, "# pom2mk ")
	} else if strings.HasPrefix(line, "# pom2bp ") { // Bazel BUILD file
		line = strings.TrimPrefix(line, "# pom2bp ")
	} else {
		return fmt.Errorf("unexpected second line: %q", line)
	}
	args := strings.Split(line, " ")
	lastArg := args[len(args)-1]
	args = args[:len(args)-1]

	// Append all current command line args except -regen <file> to the ones from the file
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-regen" || os.Args[i] == "--regen" {
			i++
		} else {
			args = append(args, os.Args[i])
		}
	}
	args = append(args, lastArg)

	cmd := os.Args[0] + " " + strings.Join(args, " ")
	// Re-exec pom2bp with the new arguments
	output, err := exec.Command("/bin/sh", "-c", cmd).Output()
	if exitErr, _ := err.(*exec.ExitError); exitErr != nil {
		return fmt.Errorf("failed to run %s\n%s", cmd, string(exitErr.Stderr))
	} else if err != nil {
		return err
	}

	// If the old file was a .mk file, replace it with a .bp file
	if filepath.Ext(filename) == ".mk" {
		os.Remove(filename)
		filename = strings.TrimSuffix(filename, ".mk") + ".bp"
	}

	return ioutil.WriteFile(filename, output, 0666)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `pom2bp, a tool to create Android.bp files from maven repos

The tool will extract the necessary information from *.pom files to create an Android.bp whose
aar libraries can be linked against when using AAPT2.

Usage: %s [--rewrite <regex>=<replace>] [--exclude <module>] [--extra-static-libs <module>=<module>[,<module>]] [--extra-libs <module>=<module>[,<module>]] [--optional-uses-libs <module>=<module>[,<module>]] [<dir>] [-regen <file>]

  -rewrite <regex>=<replace>
     rewrite can be used to specify mappings between Maven projects and Android.bp modules. The -rewrite
     option can be specified multiple times. When determining the Android.bp module for a given Maven
     project, mappings are searched in the order they were specified. The first <regex> matching
     either the Maven project's <groupId>:<artifactId> or <artifactId> will be used to generate
     the Android.bp module name using <replace>. If no matches are found, <artifactId> is used.
  -exclude <module>
     Don't put the specified module in the Android.bp file.
  -extra-static-libs <module>=<module>[,<module>]
     Some Android.bp modules have transitive static dependencies that must be specified when they
     are depended upon (like android-support-v7-mediarouter requires android-support-v7-appcompat).
     This may be specified multiple times to declare these dependencies.
  -extra-libs <module>=<module>[,<module>]
     Some Android.bp modules have transitive runtime dependencies that must be specified when they
     are depended upon (like androidx.test.rules requires android.test.base).
     This may be specified multiple times to declare these dependencies.
  -optional-uses-libs <module>=<module>[,<module>]
     Some Android.bp modules have optional dependencies (typically specified with <uses-library> in
     the module's AndroidManifest.xml) that must be specified when they are depended upon (like
     androidx.window:window optionally requires androidx.window:window-extensions).
     This may be specified multiple times to declare these dependencies.
  -sdk-version <version>
     Sets sdk_version: "<version>" for all modules.
  -default-min-sdk-version
     The default min_sdk_version to use for a module if one cannot be mined from AndroidManifest.xml
  -use-version <version>
     If the maven directory contains multiple versions of artifacts and their pom files,
     -use-version can be used to only write Android.bp files for a specific version of those artifacts.
  -write-cmd
     Whether to write the command line arguments used to generate the build file as a comment at
     the top of the build file itself.
  -jetifier
     Sets jetifier: true for all modules.
  <dir>
     The directory to search for *.pom files under.
     The contents are written to stdout, to be put in the current directory (often as Android.bp)
  -regen <file>
     Read arguments from <file> and overwrite it (if it ends with .bp) or move it to .bp (if it
     ends with .mk).

`, os.Args[0])
	}

	var regen string
	var pom2build bool
	var prepend string

	flag.Var(&excludes, "exclude", "Exclude module")
	flag.Var(&extraStaticLibs, "extra-static-libs", "Extra static dependencies needed when depending on a module")
	flag.Var(&extraLibs, "extra-libs", "Extra runtime dependencies needed when depending on a module")
	flag.Var(&optionalUsesLibs, "optional-uses-libs", "Extra optional dependencies needed when depending on a module")
	flag.Var(&rewriteNames, "rewrite", "Regex(es) to rewrite artifact names")
	flag.Var(&hostModuleNames, "host", "Specifies that the corresponding module (specified in the form 'module.group:module.artifact') is a host module")
	flag.Var(&hostAndDeviceModuleNames, "host-and-device", "Specifies that the corresponding module (specified in the form 'module.group:module.artifact') is both a host and device module.")
	flag.StringVar(&sdkVersion, "sdk-version", "", "What to write to sdk_version")
	flag.StringVar(&defaultMinSdkVersion, "default-min-sdk-version", "24", "Default min_sdk_version to use, if one is not available from AndroidManifest.xml. Default: 24")
	flag.StringVar(&useVersion, "use-version", "", "Only read artifacts of a specific version")
	flag.BoolVar(&staticDeps, "static-deps", false, "Statically include direct dependencies")
	flag.BoolVar(&writeCmd, "write-cmd", true, "Write command line arguments as a comment")
	flag.BoolVar(&jetifier, "jetifier", false, "Sets jetifier: true on all modules")
	flag.StringVar(&regen, "regen", "", "Rewrite specified file")
	flag.BoolVar(&pom2build, "pom2build", false, "If true, will generate a Bazel BUILD file *instead* of a .bp file")
	flag.StringVar(&prepend, "prepend", "", "Path to a file containing text to insert at the beginning of the generated build file")
	flag.Parse()

	if regen != "" {
		err := rerunForRegen(regen)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "Directory argument is required")
		os.Exit(1)
	} else if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "Multiple directories provided:", strings.Join(flag.Args(), " "))
		os.Exit(1)
	}

	dir := flag.Arg(0)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to get absolute directory:", err)
		os.Exit(1)
	}

	var filenames []string
	err = filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasPrefix(name, ".") {
			return nil
		}

		if strings.HasSuffix(name, ".pom") {
			path, err = filepath.Rel(absDir, path)
			if err != nil {
				return err
			}
			filenames = append(filenames, filepath.Join(dir, path))
		}
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error walking files:", err)
		os.Exit(1)
	}

	if len(filenames) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no *.pom files found under", dir)
		os.Exit(1)
	}

	sort.Strings(filenames)

	poms := []*Pom{}
	modules := make(map[string]*Pom)
	duplicate := false
	for _, filename := range filenames {
		pom, err := parse(filename)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error converting", filename, err)
			os.Exit(1)
		}

		if pom != nil {
			key := pom.BpName()
			if excludes[key] {
				continue
			}

			if old, ok := modules[key]; ok {
				fmt.Fprintln(os.Stderr, "Module", key, "defined twice:", old.PomFile, pom.PomFile)
				duplicate = true
			}

			poms = append(poms, pom)
			modules[key] = pom
		}
	}
	if duplicate {
		os.Exit(1)
	}

	if pom2build {
		if err := InitRefreshMod(poms); err != nil {
			fmt.Fprintf(os.Stderr, "Error in refreshmod: %s", err)
			os.Exit(1)
		}
		BazelifyExtraDeps(extraStaticLibs, modules)
		BazelifyExtraDeps(extraLibs, modules)
		BazelifyExtraDeps(optionalUsesLibs, modules)
	}

	for _, pom := range poms {
		if pom.IsAar() {
			err := pom.ExtractMinSdkVersion()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading manifest for %s: %s", pom.ArtifactFile, err)
				os.Exit(1)
			}
		}
		pom.FixDeps(modules)
		if pom2build {
			pom.GetBazelDepNames(modules)
		}
	}

	buf := &bytes.Buffer{}
	commentString := "//"
	if pom2build {
		commentString = "#"
	}

	fmt.Fprintln(buf, commentString, "This is a generated file. Do not modify directly.")

	if writeCmd {
		fmt.Fprintln(buf, commentString, "Automatically generated with:")
		fmt.Fprintln(buf, commentString, "pom2bp", strings.Join(proptools.ShellEscapeList(os.Args[1:]), " "))
	}

	if prepend != "" {
		contents, err := ioutil.ReadFile(prepend)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error reading", prepend, err)
			os.Exit(1)
		}
		fmt.Fprintln(buf, string(contents))
	}

	depsTemplate := bpDepsTemplate
	template := bpTemplate
	if pom2build {
		depsTemplate = bazelDepsTemplate
		template = bazelTemplate
	}

	for _, pom := range poms {
		var err error
		if staticDeps && !pom.IsApk() {
			err = depsTemplate.Execute(buf, pom)
		} else {
			err = template.Execute(buf, pom)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error writing", pom.PomFile, pom.BpName(), err)
			os.Exit(1)
		}
	}

	if pom2build {
		os.Stdout.WriteString(buf.String())
	} else {
		out, err := bpfix.Reformat(buf.String())
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error formatting output", err)
			os.Exit(1)
		}
		os.Stdout.WriteString(out)
	}

}
