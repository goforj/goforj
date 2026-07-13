package forj

import (
	"github.com/alecthomas/kong"
	"github.com/goforj/goforj/internal/bench"
	"github.com/goforj/goforj/internal/build"
	"github.com/goforj/goforj/internal/forj/atlas"
	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
)

// RootCmd owns framework-level commands while app-owned commands are reached through delegation.
type RootCmd struct {
	Version                   kong.VersionFlag                `help:"Show version information" version:"${version}"`
	Dev                       bool                            `name:"dev" aliases:"x" env:"FORJ_DEV" help:"Show developer/maintainer commands in help output" hidden:""`
	BuildCmd                  build.Cmd                       `cmd:""`
	GenerateCmd               generate.Cmd                    `cmd:""`
	NewProjectCmd             NewProjectCmd                   `cmd:""`
	AtlasInstallCmd           atlas.InstallCmd                `cmd:""`
	AtlasUpdateCmd            atlas.UpdateCmd                 `cmd:""`
	AtlasDoctorCmd            atlas.DoctorCmd                 `cmd:""`
	AtlasListSkillsCmd        atlas.ListSkillsCmd             `cmd:""`
	AtlasMakeSkillCmd         atlas.MakeSkillCmd              `cmd:""`
	AtlasMCPCmd               atlas.MCPCmd                    `cmd:""`
	MakeAppCmd                makeapp.Cmd                     `cmd:""`
	DevCmd                    DevCmd                          `cmd:""`
	DownCmd                   DownCmd                         `cmd:""`
	BuildBinaryCmd            BuildBinaryCmd                  `cmd:""`
	TestRenderCmd             TestRenderCmd                   `cmd:""`
	TestRendersCmd            TestRendersCmd                  `cmd:""`
	TestIntegrationCmd        TestIntegrationCmd              `cmd:""`
	InspectOverheadMeasureCmd bench.InspectOverheadMeasureCmd `cmd:""`
	LoggerOverheadMeasureCmd  bench.LoggerOverheadMeasureCmd  `cmd:""`
	HTTPLiveProfileCmd        bench.HTTPLiveProfileCmd        `cmd:""`
	HTTPRuntimeProfileCmd     bench.HTTPRuntimeProfileCmd     `cmd:""`
	MetricsOverheadMeasureCmd bench.MetricsOverheadMeasureCmd `cmd:""`
	TestConsoleCmd            TestConsoleCmd                  `cmd:""`
	TestOpenAPICmd            TestOpenAPICmd                  `cmd:""`
	ScenarioListCmd           ScenarioListCmd                 `cmd:""`
	ScenarioGenerateCmd       ScenarioGenerateCmd             `cmd:""`
	ScenarioTestCmd           ScenarioTestCmd                 `cmd:""`
	RenderCmd                 RenderCmd                       `cmd:""`
	RunCmd                    build.RunCmd                    `cmd:""`
}

// NewRootCmd wires only native framework commands so generated app generators do not appear in forj help.
func NewRootCmd(
	buildCmd *build.Cmd,
	generateCmd *generate.Cmd,
	newProjectCmd *NewProjectCmd,
	atlasInstallCmd *atlas.InstallCmd,
	atlasUpdateCmd *atlas.UpdateCmd,
	atlasDoctorCmd *atlas.DoctorCmd,
	atlasListSkillsCmd *atlas.ListSkillsCmd,
	atlasMakeSkillCmd *atlas.MakeSkillCmd,
	atlasMCPCmd *atlas.MCPCmd,
	makeAppCmd *makeapp.Cmd,
	devCmd *DevCmd,
	downCmd *DownCmd,
	buildBinaryCmd *BuildBinaryCmd,
	testRenderCmd *TestRenderCmd,
	testRendersCmd *TestRendersCmd,
	testIntegrationCmd *TestIntegrationCmd,
	inspectOverheadMeasureCmd *bench.InspectOverheadMeasureCmd,
	loggerOverheadMeasureCmd *bench.LoggerOverheadMeasureCmd,
	httpLiveProfileCmd *bench.HTTPLiveProfileCmd,
	httpRuntimeProfileCmd *bench.HTTPRuntimeProfileCmd,
	metricsOverheadMeasureCmd *bench.MetricsOverheadMeasureCmd,
	testConsoleCmd *TestConsoleCmd,
	testOpenAPICmd *TestOpenAPICmd,
	scenarioListCmd *ScenarioListCmd,
	scenarioGenerateCmd *ScenarioGenerateCmd,
	scenarioTestCmd *ScenarioTestCmd,
	rendererCmd *RenderCmd,
	runCmd *build.RunCmd,
) *RootCmd {
	root := &RootCmd{
		BuildCmd:                  *buildCmd,
		GenerateCmd:               *generateCmd,
		NewProjectCmd:             *newProjectCmd,
		AtlasInstallCmd:           *atlasInstallCmd,
		AtlasUpdateCmd:            *atlasUpdateCmd,
		AtlasDoctorCmd:            *atlasDoctorCmd,
		AtlasListSkillsCmd:        *atlasListSkillsCmd,
		AtlasMakeSkillCmd:         *atlasMakeSkillCmd,
		AtlasMCPCmd:               *atlasMCPCmd,
		MakeAppCmd:                *makeAppCmd,
		DevCmd:                    *devCmd,
		DownCmd:                   *downCmd,
		BuildBinaryCmd:            *buildBinaryCmd,
		TestRenderCmd:             *testRenderCmd,
		TestRendersCmd:            *testRendersCmd,
		TestIntegrationCmd:        *testIntegrationCmd,
		InspectOverheadMeasureCmd: *inspectOverheadMeasureCmd,
		LoggerOverheadMeasureCmd:  *loggerOverheadMeasureCmd,
		HTTPLiveProfileCmd:        *httpLiveProfileCmd,
		HTTPRuntimeProfileCmd:     *httpRuntimeProfileCmd,
		MetricsOverheadMeasureCmd: *metricsOverheadMeasureCmd,
		TestConsoleCmd:            *testConsoleCmd,
		TestOpenAPICmd:            *testOpenAPICmd,
		ScenarioListCmd:           *scenarioListCmd,
		ScenarioGenerateCmd:       *scenarioGenerateCmd,
		ScenarioTestCmd:           *scenarioTestCmd,
		RenderCmd:                 *rendererCmd,
		RunCmd:                    *runCmd,
	}
	return root
}
