package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestJobsLighthouseHistoryUsesQueueHistory verifies queue history has one durable owner and no cache-backed duplicate timeline.
func TestJobsLighthouseHistoryUsesQueueHistory(t *testing.T) {
	components := project.Components{CLI: true, Cache: true, Jobs: true}
	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/jobs-history"},
		Components:        components,
		ProjectComponents: components,
	}

	runtimeSource := renderSharedTemplate(t, "internal/jobs/lighthouse.go.tmpl", data)
	queueSource := renderSharedTemplate(t, "internal/jobs/lighthouse_queue.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/jobs/lighthouse.go.tmpl", runtimeSource)
	assertFormattedGoTemplate(t, "internal/jobs/lighthouse_queue.go.tmpl", queueSource)

	for _, want := range []string{
		`c.agent.RegisterCommand("queue:history", queueHistoryHandler)`,
		`queueInstance.History(ctx, queueName, window)`,
	} {
		if !strings.Contains(queueSource, want) {
			t.Fatalf("Jobs Lighthouse queue source omitted %q:\n%s", want, queueSource)
		}
	}
	for _, forbidden := range []string{
		"queueTimeline",
		"recordSnapshot",
	} {
		if strings.Contains(queueSource, forbidden) || strings.Contains(runtimeSource, forbidden) {
			t.Fatalf("Jobs Lighthouse retained obsolete timeline marker %q", forbidden)
		}
	}
	if strings.Contains(queueSource, `"github.com/goforj/cache/cachecore"`) {
		t.Fatalf("Jobs queue history retained the obsolete Cache persistence dependency:\n%s", queueSource)
	}
}
