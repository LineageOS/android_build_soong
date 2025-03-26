package android

import (
	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type ModuleProxy struct {
	module blueprint.ModuleProxy
}

var _ Module = (*ModuleProxy)(nil)

func (m ModuleProxy) IsNil() bool {
	return m.module.IsNil()
}

func (m ModuleProxy) Name() string {
	return m.module.Name()
}

func (m ModuleProxy) GenerateBuildActions(context blueprint.ModuleContext) {
	m.module.GenerateBuildActions(context)
}

func (m ModuleProxy) GenerateAndroidBuildActions(context ModuleContext) {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) CleanupAfterBuildActions() {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) ComponentDepsMutator(ctx BottomUpMutatorContext) {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) DepsMutator(context BottomUpMutatorContext) {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) base() *ModuleBase {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) Disable() {

	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) Enabled(ctx ConfigurableEvaluatorContext) bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) Target() Target {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) MultiTargets() []Target {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) ImageVariation() blueprint.Variation {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) Owner() string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInData() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInTestcases() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInSanitizerDir() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInRamdisk() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInVendorRamdisk() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInDebugRamdisk() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInRecovery() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInRoot() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInOdm() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInProduct() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInVendor() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInSystemExt() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInSystemDlkm() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInVendorDlkm() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallInOdmDlkm() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) InstallForceOS() (*OsType, *ArchType) {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) PartitionTag(d DeviceConfig) string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) HideFromMake() {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) IsHideFromMake() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) SkipInstall() {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) IsSkipInstall() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) MakeUninstallable() {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) ReplacedByPrebuilt() {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) IsReplacedByPrebuilt() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) ExportedToMake() bool {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) EffectiveLicenseFiles() Paths {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) AddProperties(props ...interface{}) {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) GetProperties() []interface{} {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) BuildParamsForTests() []BuildParams {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) VariablesForTests() map[string]string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) String() string {
	return m.module.String()
}

func (m ModuleProxy) qualifiedModuleId(ctx BaseModuleContext) qualifiedModuleName {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) visibilityProperties() []visibilityProperty {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) RequiredModuleNames(ctx ConfigurableEvaluatorContext) []string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) HostRequiredModuleNames() []string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) TargetRequiredModuleNames() []string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) VintfFragmentModuleNames(ctx ConfigurableEvaluatorContext) []string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) ConfigurableEvaluator(ctx ConfigurableEvaluatorContext) proptools.ConfigurableEvaluator {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) DecodeMultilib(ctx ConfigContext) (string, string) {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) Overrides() []string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) VintfFragments(ctx ConfigurableEvaluatorContext) []string {
	panic("method is not implemented on ModuleProxy")
}

func (m ModuleProxy) UseGenericConfig() bool {
	panic("method is not implemented on ModuleProxy")
}
