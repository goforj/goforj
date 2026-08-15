package project

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestRenderConfigTracksExplicitAgentGuidance distinguishes durable selection from legacy omission.
func TestRenderConfigTracksExplicitAgentGuidance(t *testing.T) {
	for _, test := range []struct {
		name     string
		yaml     string
		value    AgentGuidance
		explicit bool
		wantErr  string
	}{
		{name: "baseline", yaml: "agent_guidance: baseline\n", value: AgentGuidanceBaseline, explicit: true},
		{name: "none", yaml: "agent_guidance: none\n", value: AgentGuidanceNone, explicit: true},
		{name: "legacy omission", yaml: "components: [cli]\n", explicit: false},
		{name: "invalid", yaml: "agent_guidance: recommended\n", wantErr: "unsupported agent_guidance"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var config RenderConfig
			err := yaml.Unmarshal([]byte(test.yaml), &config)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("Unmarshal() error = %v, want %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.AgentGuidance != test.value || config.HasAgentGuidance() != test.explicit {
				t.Fatalf("config = %#v, explicit = %t", config, config.HasAgentGuidance())
			}
		})
	}
}
