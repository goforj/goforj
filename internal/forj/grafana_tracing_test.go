package forj

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestGrafanaTraceStackFollowsComponentSelection keeps optional developer Grafana independent from the provisioned tracing stack.
func TestGrafanaTraceStackFollowsComponentSelection(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional service", true: "generated stack"}[enabled], func(t *testing.T) {
			components := project.DefaultSelectedComponents()
			components.Grafana = enabled
			plan := defaultResourcePlanForTest(t, components)
			_, rendered := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
			type service struct {
				Profiles  []string `yaml:"profiles"`
				DependsOn []string `yaml:"depends_on"`
				Ports     []string `yaml:"ports"`
				Volumes   []string `yaml:"volumes"`
			}
			var compose struct {
				Services map[string]yaml.Node `yaml:"services"`
			}
			if err := yaml.Unmarshal([]byte(rendered), &compose); err != nil {
				t.Fatal(err)
			}
			tempoNode, exists := compose.Services["tempo"]
			var tempo, grafana service
			grafanaNode := compose.Services["grafana"]
			if err := grafanaNode.Decode(&grafana); err != nil {
				t.Fatal(err)
			}
			if exists {
				if err := tempoNode.Decode(&tempo); err != nil {
					t.Fatal(err)
				}
			}
			if exists != enabled || slices.Contains(grafana.DependsOn, "tempo") != enabled {
				t.Fatalf("Tempo provisioning and Grafana dependency must follow the Grafana component: %v", compose.Services)
			}
			if !enabled {
				return
			}
			if !slices.Equal(tempo.Profiles, []string{"grafana"}) || !slices.Contains(tempo.Volumes, "tempo:/var/tempo") {
				t.Fatalf("Tempo must share the Grafana profile and persist traces: %+v", tempo)
			}
			for _, port := range []string{"${IP_ADDRESS:-127.0.0.1}:${TEMPO_HTTP_PORT:-13200}:3200", "${IP_ADDRESS:-127.0.0.1}:${TEMPO_OTLP_GRPC_PORT:-14317}:4317", "${IP_ADDRESS:-127.0.0.1}:${TEMPO_OTLP_HTTP_PORT:-14318}:4318"} {
				if !slices.Contains(tempo.Ports, port) {
					t.Fatalf("missing local-only overridable port %q: %v", port, tempo.Ports)
				}
			}
		})
	}
}

// TestGrafanaTraceDashboardUsesProvisionedDatasource keeps service discovery and trace search on the same provisioned backend.
func TestGrafanaTraceDashboardUsesProvisionedDatasource(t *testing.T) {
	root := filepath.Join("..", "..", "templates", "containers", "observability")
	raw, err := os.ReadFile(filepath.Join(root, "grafana", "dashboards", "traces-overview.json.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	var dashboard struct {
		UID        string `json:"uid"`
		Templating struct {
			List []struct {
				Datasource struct{ UID string }   `json:"datasource"`
				Query      struct{ Label string } `json:"query"`
			} `json:"list"`
		} `json:"templating"`
		Panels []struct {
			Targets []struct {
				Datasource struct{ UID string } `json:"datasource"`
				Query      string               `json:"query"`
				QueryType  string               `json:"queryType"`
			} `json:"targets"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(raw, &dashboard); err != nil {
		t.Fatal(err)
	}
	if dashboard.UID != "goforj-traces-overview" || len(dashboard.Templating.List) != 1 {
		t.Fatalf("invalid trace dashboard identity or variables: %+v", dashboard)
	}
	variable := dashboard.Templating.List[0]
	if variable.Datasource.UID != "goforj-tempo" || variable.Query.Label != "resource.service.name" {
		t.Fatalf("invalid service discovery: %+v", variable)
	}
	queries := 0
	for _, panel := range dashboard.Panels {
		for _, target := range panel.Targets {
			queries++
			if target.Datasource.UID != "goforj-tempo" || target.QueryType != "traceql" || !strings.Contains(target.Query, "`${service:regex}`") {
				t.Fatalf("invalid trace search: %+v", target)
			}
		}
	}
	if queries == 0 {
		t.Fatal("dashboard must search traces")
	}
	for _, tc := range []struct{ file, want string }{
		{"grafana/provisioning/datasources/datasource.yml.tmpl", "uid: goforj-tempo"},
		{"grafana/seed-dashboards.sh.tmpl", dashboard.UID},
		{"tempo/Dockerfile.tmpl", "grafana/tempo:3.1.1@sha256:"},
		{"tempo/tempo.yaml.tmpl", "path: /var/tempo/wal"},
	} {
		data, err := os.ReadFile(filepath.Join(root, tc.file))
		if err != nil || !strings.Contains(string(data), tc.want) {
			t.Fatalf("trace provisioning %s missing %q: %v", tc.file, tc.want, err)
		}
	}
}
