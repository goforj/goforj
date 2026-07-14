package project

import (
	"strings"
	"testing"
)

// TestResourceCatalogExternalDriversDeclareOperationalMetadata protects the Advanced inventory from silent infrastructure requirements.
func TestResourceCatalogExternalDriversDeclareOperationalMetadata(t *testing.T) {
	for _, resource := range ResourceCatalog() {
		for _, driver := range resource.Drivers {
			if !resourceDriverRequiresInfrastructure(driver) {
				continue
			}
			if driver.Service == "" {
				t.Errorf("%s %s has no stable service key", resource.Key, driver.Name)
			}
			if strings.TrimSpace(driver.ServiceLabel) == "" {
				t.Errorf("%s %s has no readable service label", resource.Key, driver.Name)
			}
			if len(driver.Environment) == 0 && len(driver.EndpointEnvironment) == 0 && !resourceDriverUsesBaseEnvironment(driver.Service) {
				t.Errorf("%s %s has no endpoint or credential placeholders", resource.Key, driver.Name)
			}
		}
	}
}

// TestResourceCatalogExternalEndpointAffinity prevents similarly named drivers from accidentally sharing resource-specific connections.
func TestResourceCatalogExternalEndpointAffinity(t *testing.T) {
	checks := []struct {
		resource ResourceKey
		driver   string
		want     ServiceKey
	}{
		{resource: ResourceCache, driver: "nats", want: ServiceCacheNATS},
		{resource: ResourceQueue, driver: "nats", want: ServiceQueueNATS},
		{resource: ResourceEvents, driver: "nats", want: ServiceEventsNATS},
		{resource: ResourceCache, driver: "postgres", want: ServiceCachePostgres},
		{resource: ResourceQueue, driver: "postgres", want: ServiceQueuePostgres},
	}
	for _, check := range checks {
		definition, _ := ResourceDefinitionByKey(check.resource)
		driver, ok := definition.Driver(check.driver)
		if !ok {
			t.Fatalf("%s driver %s is missing", check.resource, check.driver)
		}
		if driver.Service != check.want {
			t.Errorf("%s %s service = %q, want %q", check.resource, check.driver, driver.Service, check.want)
		}
	}
	if ServiceCacheNATS == ServiceQueueNATS || ServiceQueueNATS == ServiceEventsNATS || ServiceCachePostgres == ServiceQueuePostgres {
		t.Fatal("resource-specific endpoints were assigned a shared service identity")
	}
}

// TestResourceCatalogReturnsDefensiveEnvironmentMetadata verifies callers cannot mutate process-wide placeholder policy.
func TestResourceCatalogReturnsDefensiveEnvironmentMetadata(t *testing.T) {
	definition, _ := ResourceDefinitionByKey(ResourceStorage)
	driver, _ := definition.Driver("s3")
	if len(driver.Environment) == 0 {
		t.Fatal("S3 placeholder metadata is missing")
	}
	driver.Environment[0].Key = "CHANGED"

	second, _ := ResourceDefinitionByKey(ResourceStorage)
	retained, _ := second.Driver("s3")
	if retained.Environment[0].Key == "CHANGED" {
		t.Fatal("ResourceDefinitionByKey returned aliased environment metadata")
	}
}

// TestResourceCatalogSMTPDeclaresEndpointMetadata keeps Mailpit affinity separate from external SMTP endpoints.
func TestResourceCatalogSMTPDeclaresEndpointMetadata(t *testing.T) {
	definition, _ := ResourceDefinitionByKey(ResourceMail)
	driver, _ := definition.Driver("smtp")
	if len(driver.EndpointEnvironment) != 2 {
		t.Fatalf("SMTP endpoint metadata = %#v, want host and port", driver.EndpointEnvironment)
	}
	if driver.EndpointEnvironment[0].Key != "MAIL_SMTP_HOST" || driver.EndpointEnvironment[1].Key != "MAIL_SMTP_PORT" {
		t.Fatalf("SMTP endpoint metadata = %#v, want MAIL_SMTP_HOST and MAIL_SMTP_PORT", driver.EndpointEnvironment)
	}
}

// resourceDriverRequiresInfrastructure identifies catalog groups that cannot run solely in-process or from a local file.
func resourceDriverRequiresInfrastructure(driver DriverDefinition) bool {
	if driver.Group == DriverGroupShared || driver.Group == DriverGroupCloud {
		return true
	}
	return driver.Group == DriverGroupSQL && driver.Name != "sqlite"
}

// resourceDriverUsesBaseEnvironment identifies infrastructure already configured by the concise template contract.
func resourceDriverUsesBaseEnvironment(service ServiceKey) bool {
	switch service {
	case ServiceRedis, ServiceMySQL, ServicePostgres:
		return true
	default:
		return false
	}
}
