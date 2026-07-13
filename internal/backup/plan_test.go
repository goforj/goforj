package backup

import "testing"

func TestConnectionFromEnvUsesNamedFallbacks(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_ANALYTICS_DATABASE", "analytics")
	got := ConnectionFromEnv("analytics")
	if got.Driver != "postgres" || got.Host != "localhost" || got.Password != "secret" || got.Database != "analytics" {
		t.Fatalf("unexpected connection: %#v", got)
	}
}

func TestBuildPlanDiscoversConfiguredConnections(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_CONNECTIONS", "reporting,audit")
	plan, err := BuildPlan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Resources) != 3 {
		t.Fatalf("resource count = %d, want 3", len(plan.Resources))
	}
	if plan.Resources[0].Connection.Name != "default" || plan.Resources[1].Connection.Name != "audit" || plan.Resources[2].Connection.Name != "reporting" {
		t.Fatalf("unexpected plan order: %#v", plan.Resources)
	}
}

func TestBuildPlanDiscoversNamedDriverEnvironmentKeys(t *testing.T) {
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_REPORTING_DRIVER", "postgres")
	plan, err := BuildPlan()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, resource := range plan.Resources {
		if resource.Connection.Name == "reporting" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected reporting connection in plan: %#v", plan.Resources)
	}
}

func TestBuildPlanClassifiesS3StorageAsObjectManifest(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "s3")
	t.Setenv("STORAGE_BUCKET", "uploads")
	t.Setenv("STORAGE_REGION", "us-east-1")
	plan, err := BuildPlan()
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Storage) != 1 || plan.Storage[0].Driver != "s3" || plan.Storage[0].Status != "external-managed" {
		t.Fatalf("unexpected S3 storage plan: %#v", plan.Storage)
	}
}
