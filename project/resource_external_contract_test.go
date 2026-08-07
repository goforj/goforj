package project

import (
	"strings"
	"testing"
)

func TestResourceCatalogExternalDriversDeclareOperationalMetadata(t *testing.T) {
	for _, resource := range ResourceCatalog() {
		for _, driver := range resource.Drivers {
			hasEnvironment := len(driver.Environment) > 0 || len(driver.EndpointEnvironment) > 0
			if driver.Service == "" {
				if hasEnvironment {
					t.Errorf("%s %s declares infrastructure configuration without a stable service key", resource.Key, driver.Name)
				}
				continue
			}
			if strings.TrimSpace(driver.ServiceLabel) == "" {
				t.Errorf("%s %s has no readable service label", resource.Key, driver.Name)
			}
			if !hasEnvironment && !resourceDriverUsesBaseEnvironment(driver.Service) {
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
	definition.DefaultSupportedDrivers[0] = "changed"
	driver, _ := definition.Driver("s3")
	if len(driver.Environment) == 0 {
		t.Fatal("S3 placeholder metadata is missing")
	}
	driver.Environment[0].Key = "CHANGED"

	second, _ := ResourceDefinitionByKey(ResourceStorage)
	if second.DefaultSupportedDrivers[0] == "changed" {
		t.Fatal("ResourceDefinitionByKey returned aliased default-driver metadata")
	}
	retained, _ := second.Driver("s3")
	if retained.Environment[0].Key == "CHANGED" {
		t.Fatal("ResourceDefinitionByKey returned aliased environment metadata")
	}
}

// TestResourceCatalogDefinesEnvironmentAndFallbackPolicy keeps generator and renderer defaults on one shared contract.
func TestResourceCatalogDefinesEnvironmentAndFallbackPolicy(t *testing.T) {
	tests := []struct {
		resource      ResourceKey
		prefix        string
		defaultDriver string
		rootDriver    string
		namedDriver   string
	}{
		{resource: ResourceDatabase, prefix: "DB", defaultDriver: "sqlite", rootDriver: "mysql", namedDriver: "mysql"},
		{resource: ResourceCache, prefix: "CACHE", defaultDriver: "memory", rootDriver: "redis", namedDriver: "memory"},
		{resource: ResourceQueue, prefix: "QUEUE", defaultDriver: "workerpool", rootDriver: "redis", namedDriver: "redis"},
		{resource: ResourceEvents, prefix: "EVENTS", defaultDriver: "inproc", rootDriver: "redis", namedDriver: "inproc"},
		{resource: ResourceStorage, prefix: "STORAGE", defaultDriver: "local", rootDriver: "s3", namedDriver: "local"},
		{resource: ResourceMail, prefix: "MAIL", defaultDriver: "log", rootDriver: "smtp", namedDriver: "log"},
	}

	for _, test := range tests {
		t.Run(string(test.resource), func(t *testing.T) {
			definition, ok := ResourceDefinitionByKey(test.resource)
			if !ok {
				t.Fatalf("resource %q is missing", test.resource)
			}
			if definition.EnvironmentPrefix != test.prefix {
				t.Errorf("EnvironmentPrefix = %q, want %q", definition.EnvironmentPrefix, test.prefix)
			}
			if got := definition.EnvironmentKey("driver"); got != test.prefix+"_DRIVER" {
				t.Errorf("EnvironmentKey(driver) = %q, want %q", got, test.prefix+"_DRIVER")
			}
			if definition.DefaultDriver != test.defaultDriver {
				t.Errorf("DefaultDriver = %q, want %q", definition.DefaultDriver, test.defaultDriver)
			}
			if got := definition.NamedDriverDefault(test.rootDriver); got != test.namedDriver {
				t.Errorf("NamedDriverDefault(%q) = %q, want %q", test.rootDriver, got, test.namedDriver)
			}
		})
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

// TestResourceCatalogSFTPDeclaresHostVerification prevents generated guidance from implying unverified SSH is a default.
func TestResourceCatalogSFTPDeclaresHostVerification(t *testing.T) {
	definition, _ := ResourceDefinitionByKey(ResourceStorage)
	driver, _ := definition.Driver("sftp")
	placeholders := make(map[string]DriverEnvironmentPlaceholder, len(driver.Environment))
	for _, placeholder := range driver.Environment {
		placeholders[placeholder.Key] = placeholder
	}
	if _, ok := placeholders["STORAGE_KNOWN_HOSTS_PATH"]; !ok {
		t.Fatal("SFTP known_hosts placeholder is missing")
	}
	insecure, ok := placeholders["STORAGE_INSECURE_IGNORE_HOST_KEY"]
	if !ok {
		t.Fatal("SFTP insecure host-key placeholder is missing")
	}
	if insecure.Example != "false" {
		t.Fatalf("SFTP insecure host-key example = %q, want false", insecure.Example)
	}
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
