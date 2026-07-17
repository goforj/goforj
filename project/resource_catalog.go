package project

import (
	"fmt"

	"github.com/goforj/str"
)

// ResourceKey identifies an App resource whose implementation is selected by environment configuration.
type ResourceKey string

const (
	// ResourceDatabase identifies the App database.
	ResourceDatabase ResourceKey = "database"
	// ResourceCache identifies the root cache.
	ResourceCache ResourceKey = "cache"
	// ResourceQueue identifies the root queue used when Jobs is enabled.
	ResourceQueue ResourceKey = "queue"
	// ResourceEvents identifies the root event bus.
	ResourceEvents ResourceKey = "events"
	// ResourceStorage identifies the root storage disk.
	ResourceStorage ResourceKey = "storage"
	// ResourceMail identifies the root mailer used when Mail is enabled.
	ResourceMail ResourceKey = "mail"
)

// ServiceKey identifies infrastructure that may be shared by multiple active resources.
type ServiceKey string

const (
	// ServiceRedis identifies a Redis endpoint or generated Redis service.
	ServiceRedis ServiceKey = "redis"
	// ServiceMySQL identifies a MySQL endpoint or generated MariaDB service.
	ServiceMySQL ServiceKey = "mysql"
	// ServicePostgres identifies a Postgres endpoint or generated Postgres service.
	ServicePostgres ServiceKey = "postgres"
	// ServiceCacheMemcached identifies the root cache's external Memcached endpoint.
	ServiceCacheMemcached ServiceKey = "cache_memcached"
	// ServiceCacheDynamoDB identifies the root cache's DynamoDB configuration.
	ServiceCacheDynamoDB ServiceKey = "cache_dynamodb"
	// ServiceCachePostgres identifies the root cache's resource-specific Postgres endpoint.
	ServiceCachePostgres ServiceKey = "cache_postgres"
	// ServiceCacheMySQL identifies the root cache's resource-specific MySQL endpoint.
	ServiceCacheMySQL ServiceKey = "cache_mysql"
	// ServiceCacheNATS identifies the root cache's resource-specific NATS endpoint.
	ServiceCacheNATS ServiceKey = "cache_nats"
	// ServiceQueueNATS identifies the root queue's resource-specific NATS endpoint.
	ServiceQueueNATS ServiceKey = "queue_nats"
	// ServiceQueueSQS identifies the root queue's AWS SQS configuration.
	ServiceQueueSQS ServiceKey = "queue_sqs"
	// ServiceQueueRabbitMQ identifies the root queue's RabbitMQ endpoint.
	ServiceQueueRabbitMQ ServiceKey = "queue_rabbitmq"
	// ServiceQueuePostgres identifies the root queue's resource-specific Postgres endpoint.
	ServiceQueuePostgres ServiceKey = "queue_postgres"
	// ServiceQueueMySQL identifies the root queue's resource-specific MySQL endpoint.
	ServiceQueueMySQL ServiceKey = "queue_mysql"
	// ServiceEventsNATS identifies the root event bus's resource-specific NATS endpoint.
	ServiceEventsNATS ServiceKey = "events_nats"
	// ServiceEventsKafka identifies the root event bus's Kafka brokers.
	ServiceEventsKafka ServiceKey = "events_kafka"
	// ServiceEventsGCPPubSub identifies the root event bus's Google Pub/Sub configuration.
	ServiceEventsGCPPubSub ServiceKey = "events_gcp_pubsub"
	// ServiceEventsSNS identifies the root event bus's AWS SNS configuration.
	ServiceEventsSNS ServiceKey = "events_sns"
	// ServiceStorageFTP identifies the root storage disk's FTP endpoint.
	ServiceStorageFTP ServiceKey = "storage_ftp"
	// ServiceStorageSFTP identifies the root storage disk's SFTP endpoint.
	ServiceStorageSFTP ServiceKey = "storage_sftp"
	// ServiceStorageS3 identifies the root storage disk's S3 configuration.
	ServiceStorageS3 ServiceKey = "storage_s3"
	// ServiceStorageGCS identifies the root storage disk's Google Cloud Storage configuration.
	ServiceStorageGCS ServiceKey = "storage_gcs"
	// ServiceStorageDropbox identifies the root storage disk's Dropbox configuration.
	ServiceStorageDropbox ServiceKey = "storage_dropbox"
	// ServiceStorageRclone identifies the root storage disk's rclone configuration.
	ServiceStorageRclone ServiceKey = "storage_rclone"
	// ServiceMailSMTP identifies an SMTP endpoint when Mailpit is unavailable.
	ServiceMailSMTP ServiceKey = "mail_smtp"
	// ServiceMailResend identifies the App mailer's Resend configuration.
	ServiceMailResend ServiceKey = "mail_resend"
	// ServiceMailPostmark identifies the App mailer's Postmark configuration.
	ServiceMailPostmark ServiceKey = "mail_postmark"
	// ServiceMailMailgun identifies the App mailer's Mailgun configuration.
	ServiceMailMailgun ServiceKey = "mail_mailgun"
	// ServiceMailSendGrid identifies the App mailer's SendGrid configuration.
	ServiceMailSendGrid ServiceKey = "mail_sendgrid"
	// ServiceMailSES identifies the App mailer's AWS SES configuration.
	ServiceMailSES ServiceKey = "mail_ses"
)

// DriverEnvironmentPlaceholder describes one safe dotenv hint required by an opt-in driver.
type DriverEnvironmentPlaceholder struct {
	Key         string
	Example     string
	Description string
}

// DriverDefinition describes one generated implementation and its operational effect.
type DriverDefinition struct {
	Name                 string
	Label                string
	Service              ServiceKey
	ServiceLabel         string
	LocallyProvisionable bool
	EndpointEnvironment  []DriverEnvironmentPlaceholder
	Environment          []DriverEnvironmentPlaceholder
	Order                int
}

// GeneratedNamedResourceDefinition describes a framework-owned named resource seeded during generation.
type GeneratedNamedResourceDefinition struct {
	Name              string
	Label             string
	EnvironmentKey    string
	RequiredComponent ComponentKey
	DefaultDriver     string
}

// ResourceDefinition describes one resource's environment scope, fallback policy, and driver inventory.
type ResourceDefinition struct {
	Key                     ResourceKey
	Label                   string
	EnvironmentPrefix       string
	DefaultDriver           string
	DefaultSupportedDrivers []string
	NamedDriversInheritRoot bool
	Drivers                 []DriverDefinition
	NamedResources          []GeneratedNamedResourceDefinition
	applicable              func(Components) bool
}

var resourceCatalog = []ResourceDefinition{
	{
		Key:                     ResourceDatabase,
		Label:                   "Database",
		EnvironmentPrefix:       "DB",
		DefaultDriver:           "sqlite",
		DefaultSupportedDrivers: []string{"sqlite"},
		NamedDriversInheritRoot: true,
		Drivers: []DriverDefinition{
			{Name: "mysql", Label: "MySQL", Service: ServiceMySQL, ServiceLabel: "MySQL", LocallyProvisionable: true, Order: 20},
			{Name: "postgres", Label: "Postgres", Service: ServicePostgres, ServiceLabel: "Postgres", LocallyProvisionable: true, Order: 30},
			{Name: "sqlite", Label: "SQLite", Order: 10},
		},
		applicable: func(components Components) bool { return components.HasDatabase() },
	},
	{
		Key:                     ResourceCache,
		Label:                   "Cache",
		EnvironmentPrefix:       "CACHE",
		DefaultDriver:           "memory",
		DefaultSupportedDrivers: []string{"memory", "redis"},
		Drivers: []DriverDefinition{
			{Name: "memory", Label: "Memory", Order: 10},
			{Name: "file", Label: "File", Order: 20},
			{Name: "null", Label: "Null", Order: 30},
			{Name: "redis", Label: "Redis", Service: ServiceRedis, ServiceLabel: "Redis", LocallyProvisionable: true, Order: 40},
			{Name: "memcached", Label: "Memcached", Service: ServiceCacheMemcached, ServiceLabel: "Memcached for cache", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("CACHE_ADDRESSES", "", "comma-separated Memcached addresses"),
			}, Order: 50},
			{Name: "dynamodb", Label: "DynamoDB", Service: ServiceCacheDynamoDB, ServiceLabel: "DynamoDB for cache", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("CACHE_REGION", "us-east-1", "AWS region"),
				driverEnvironmentPlaceholder("CACHE_ENDPOINT", "", "optional DynamoDB endpoint override"),
			}, Order: 60},
			{Name: "sqlite", Label: "SQLite", Order: 70},
			{Name: "postgres", Label: "Postgres", Service: ServiceCachePostgres, ServiceLabel: "Postgres for cache", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("CACHE_DSN", "", "resource-specific Postgres DSN; DB_* is not inherited"),
			}, Order: 80},
			{Name: "mysql", Label: "MySQL", Service: ServiceCacheMySQL, ServiceLabel: "MySQL for cache", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("CACHE_DSN", "", "resource-specific MySQL DSN; DB_* is not inherited"),
			}, Order: 90},
			{Name: "nats", Label: "NATS", Service: ServiceCacheNATS, ServiceLabel: "NATS for cache", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("CACHE_URL", "nats://127.0.0.1:4222", "resource-specific NATS URL"),
			}, Order: 100},
		},
		NamedResources: []GeneratedNamedResourceDefinition{
			{Name: "settings", Label: "Demo settings cache", EnvironmentKey: "CACHE_SETTINGS_DRIVER", RequiredComponent: ComponentDemoApp, DefaultDriver: "memory"},
			{Name: "sessions", Label: "Auth sessions", EnvironmentKey: "CACHE_SESSIONS_DRIVER", RequiredComponent: ComponentAuth, DefaultDriver: "memory"},
		},
		applicable: func(components Components) bool { return components.Cache },
	},
	{
		Key:                     ResourceQueue,
		Label:                   "Queue",
		EnvironmentPrefix:       "QUEUE",
		DefaultDriver:           "workerpool",
		DefaultSupportedDrivers: []string{"workerpool", "redis"},
		NamedDriversInheritRoot: true,
		Drivers: []DriverDefinition{
			{Name: "null", Label: "Null", Order: 10},
			{Name: "sync", Label: "Sync", Order: 20},
			{Name: "workerpool", Label: "Workerpool", Order: 30},
			{Name: "redis", Label: "Redis", Service: ServiceRedis, ServiceLabel: "Redis", LocallyProvisionable: true, Order: 40},
			{Name: "nats", Label: "NATS", Service: ServiceQueueNATS, ServiceLabel: "NATS for queue", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("QUEUE_URL", "nats://127.0.0.1:4222", "resource-specific NATS URL"),
			}, Order: 50},
			{Name: "sqs", Label: "SQS", Service: ServiceQueueSQS, ServiceLabel: "SQS for queue", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("QUEUE_REGION", "us-east-1", "AWS region"),
				driverEnvironmentPlaceholder("QUEUE_ENDPOINT", "", "optional SQS endpoint override"),
				driverEnvironmentPlaceholder("QUEUE_ACCESS_KEY", "", "AWS access key when the default credential chain is unavailable"),
				driverEnvironmentPlaceholder("QUEUE_SECRET_KEY", "", "AWS secret key when the default credential chain is unavailable"),
			}, Order: 60},
			{Name: "rabbitmq", Label: "RabbitMQ", Service: ServiceQueueRabbitMQ, ServiceLabel: "RabbitMQ for queue", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("QUEUE_URL", "amqp://guest:guest@127.0.0.1:5672/", "resource-specific RabbitMQ URL"),
			}, Order: 70},
			{Name: "sqlite", Label: "SQLite", Order: 80},
			{Name: "postgres", Label: "Postgres", Service: ServiceQueuePostgres, ServiceLabel: "Postgres for queue", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("QUEUE_DSN", "", "resource-specific Postgres DSN; DB_* is not inherited"),
			}, Order: 90},
			{Name: "mysql", Label: "MySQL", Service: ServiceQueueMySQL, ServiceLabel: "MySQL for queue", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("QUEUE_DSN", "", "resource-specific MySQL DSN; DB_* is not inherited"),
			}, Order: 100},
		},
		applicable: func(components Components) bool { return components.Jobs },
	},
	{
		Key:                     ResourceEvents,
		Label:                   "Events",
		EnvironmentPrefix:       "EVENTS",
		DefaultDriver:           "inproc",
		DefaultSupportedDrivers: []string{"inproc", "redis"},
		Drivers: []DriverDefinition{
			{Name: "inproc", Label: "In-process", Order: 10},
			{Name: "null", Label: "Null", Order: 20},
			{Name: "redis", Label: "Redis", Service: ServiceRedis, ServiceLabel: "Redis", LocallyProvisionable: true, Order: 30},
			{Name: "nats", Label: "NATS", Service: ServiceEventsNATS, ServiceLabel: "NATS for events", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("EVENTS_URL", "nats://127.0.0.1:4222", "resource-specific NATS URL"),
			}, Order: 40},
			{Name: "natsjetstream", Label: "NATS JetStream", Service: ServiceEventsNATS, ServiceLabel: "NATS JetStream for events", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("EVENTS_URL", "nats://127.0.0.1:4222", "resource-specific NATS URL"),
			}, Order: 50},
			{Name: "kafka", Label: "Kafka", Service: ServiceEventsKafka, ServiceLabel: "Kafka for events", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("EVENTS_BROKERS", "127.0.0.1:9092", "comma-separated Kafka brokers"),
			}, Order: 60},
			{Name: "gcppubsub", Label: "Google Pub/Sub", Service: ServiceEventsGCPPubSub, ServiceLabel: "Google Pub/Sub for events", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("EVENTS_PROJECT_ID", "", "Google Cloud project ID"),
				driverEnvironmentPlaceholder("EVENTS_URI", "", "optional Pub/Sub emulator or endpoint URI"),
			}, Order: 70},
			{Name: "sns", Label: "SNS", Service: ServiceEventsSNS, ServiceLabel: "SNS for events", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("EVENTS_REGION", "us-east-1", "AWS region"),
				driverEnvironmentPlaceholder("EVENTS_ENDPOINT", "", "optional SNS endpoint override"),
			}, Order: 80},
		},
		applicable: func(components Components) bool { return components.Events },
	},
	{
		Key:                     ResourceStorage,
		Label:                   "Storage",
		EnvironmentPrefix:       "STORAGE",
		DefaultDriver:           "local",
		DefaultSupportedDrivers: []string{"local"},
		Drivers: []DriverDefinition{
			{Name: "local", Label: "Local", Order: 10},
			{Name: "memory", Label: "Memory", Order: 20},
			{Name: "redis", Label: "Redis", Service: ServiceRedis, ServiceLabel: "Redis", LocallyProvisionable: true, Order: 30},
			{Name: "ftp", Label: "FTP", Service: ServiceStorageFTP, ServiceLabel: "FTP for storage", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("STORAGE_HOST", "", "FTP host"),
				driverEnvironmentPlaceholder("STORAGE_USER", "", "FTP user"),
				driverEnvironmentPlaceholder("STORAGE_PASSWORD", "", "FTP password"),
			}, Order: 40},
			{Name: "sftp", Label: "SFTP", Service: ServiceStorageSFTP, ServiceLabel: "SFTP for storage", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("STORAGE_HOST", "", "SFTP host"),
				driverEnvironmentPlaceholder("STORAGE_USER", "root", "SFTP user"),
				driverEnvironmentPlaceholder("STORAGE_PASSWORD", "", "SFTP password when key authentication is unavailable"),
				driverEnvironmentPlaceholder("STORAGE_KEY_PATH", "", "SFTP private-key path"),
				driverEnvironmentPlaceholder("STORAGE_KNOWN_HOSTS_PATH", "", "known_hosts file used to verify the SFTP server"),
				driverEnvironmentPlaceholder("STORAGE_INSECURE_IGNORE_HOST_KEY", "false", "explicitly disable SFTP host-key verification for isolated development only"),
			}, Order: 50},
			{Name: "s3", Label: "S3", Service: ServiceStorageS3, ServiceLabel: "S3 for storage", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("STORAGE_BUCKET", "", "S3 bucket"),
				driverEnvironmentPlaceholder("STORAGE_REGION", "us-east-1", "AWS region"),
				driverEnvironmentPlaceholder("STORAGE_ENDPOINT", "", "optional S3-compatible endpoint"),
				driverEnvironmentPlaceholder("STORAGE_ACCESS_KEY_ID", "", "AWS access key when the default credential chain is unavailable"),
				driverEnvironmentPlaceholder("STORAGE_SECRET_ACCESS_KEY", "", "AWS secret key when the default credential chain is unavailable"),
			}, Order: 60},
			{Name: "gcs", Label: "Google Cloud Storage", Service: ServiceStorageGCS, ServiceLabel: "Google Cloud Storage", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("STORAGE_BUCKET", "", "Google Cloud Storage bucket"),
				driverEnvironmentPlaceholder("STORAGE_CREDENTIALS_JSON", "", "Google Cloud credentials JSON"),
				driverEnvironmentPlaceholder("STORAGE_ENDPOINT", "", "optional Google Cloud Storage endpoint"),
			}, Order: 70},
			{Name: "dropbox", Label: "Dropbox", Service: ServiceStorageDropbox, ServiceLabel: "Dropbox for storage", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("STORAGE_TOKEN", "", "Dropbox access token"),
			}, Order: 80},
			{Name: "rclone", Label: "rclone", Service: ServiceStorageRclone, ServiceLabel: "rclone for storage", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("STORAGE_REMOTE", "", "rclone remote name"),
				driverEnvironmentPlaceholder("STORAGE_RCLONE_CONFIG_PATH", "", "rclone config path"),
				driverEnvironmentPlaceholder("STORAGE_RCLONE_CONFIG_DATA", "", "inline rclone config when a path is unavailable"),
			}, Order: 90},
		},
		NamedResources: []GeneratedNamedResourceDefinition{
			{Name: "public", Label: "Public storage", EnvironmentKey: "STORAGE_PUBLIC_DRIVER", DefaultDriver: "local"},
			{Name: "favicons", Label: "Demo favicon storage", EnvironmentKey: "STORAGE_FAVICONS_DRIVER", RequiredComponent: ComponentDemoApp, DefaultDriver: "local"},
		},
		applicable: func(components Components) bool { return components.Storage },
	},
	{
		Key:                     ResourceMail,
		Label:                   "Mail",
		EnvironmentPrefix:       "MAIL",
		DefaultDriver:           "log",
		DefaultSupportedDrivers: []string{"log", "smtp"},
		Drivers: []DriverDefinition{
			{Name: "log", Label: "Log", Order: 10},
			{Name: "smtp", Label: "SMTP", Service: ServiceMailSMTP, ServiceLabel: "SMTP for mail", LocallyProvisionable: true, EndpointEnvironment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("MAIL_SMTP_HOST", "localhost", "SMTP hostname"),
				driverEnvironmentPlaceholder("MAIL_SMTP_PORT", "1025", "SMTP port"),
			}, Order: 20},
			{Name: "resend", Label: "Resend", Service: ServiceMailResend, ServiceLabel: "Resend for mail", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("MAIL_RESEND_API_KEY", "", "Resend API key"),
				driverEnvironmentPlaceholder("MAIL_RESEND_ENDPOINT", "", "optional Resend endpoint override"),
			}, Order: 30},
			{Name: "postmark", Label: "Postmark", Service: ServiceMailPostmark, ServiceLabel: "Postmark for mail", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("MAIL_POSTMARK_SERVER_TOKEN", "", "Postmark server token"),
				driverEnvironmentPlaceholder("MAIL_POSTMARK_ENDPOINT", "", "optional Postmark endpoint override"),
			}, Order: 40},
			{Name: "mailgun", Label: "Mailgun", Service: ServiceMailMailgun, ServiceLabel: "Mailgun for mail", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("MAIL_MAILGUN_DOMAIN", "", "Mailgun domain"),
				driverEnvironmentPlaceholder("MAIL_MAILGUN_API_KEY", "", "Mailgun API key"),
				driverEnvironmentPlaceholder("MAIL_MAILGUN_ENDPOINT", "", "optional Mailgun endpoint override"),
			}, Order: 50},
			{Name: "sendgrid", Label: "SendGrid", Service: ServiceMailSendGrid, ServiceLabel: "SendGrid for mail", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("MAIL_SENDGRID_API_KEY", "", "SendGrid API key"),
				driverEnvironmentPlaceholder("MAIL_SENDGRID_ENDPOINT", "", "optional SendGrid endpoint override"),
			}, Order: 60},
			{Name: "ses", Label: "SES", Service: ServiceMailSES, ServiceLabel: "SES for mail", Environment: []DriverEnvironmentPlaceholder{
				driverEnvironmentPlaceholder("MAIL_SES_REGION", "us-east-1", "AWS region"),
				driverEnvironmentPlaceholder("MAIL_SES_ACCESS_KEY_ID", "", "AWS access key when the default credential chain is unavailable"),
				driverEnvironmentPlaceholder("MAIL_SES_SECRET_ACCESS_KEY", "", "AWS secret key when the default credential chain is unavailable"),
				driverEnvironmentPlaceholder("MAIL_SES_ENDPOINT", "", "optional SES endpoint override"),
			}, Order: 70},
		},
		applicable: func(components Components) bool { return components.Mail },
	},
}

// ResourceCatalog returns a defensive copy of the canonical resource definitions.
func ResourceCatalog() []ResourceDefinition {
	definitions := make([]ResourceDefinition, len(resourceCatalog))
	for index, definition := range resourceCatalog {
		definitions[index] = cloneResourceDefinition(definition)
	}
	return definitions
}

// ResourceDefinitionByKey returns a defensive copy of one resource definition.
func ResourceDefinitionByKey(key ResourceKey) (ResourceDefinition, bool) {
	for _, definition := range resourceCatalog {
		if definition.Key == key {
			return cloneResourceDefinition(definition), true
		}
	}
	return ResourceDefinition{}, false
}

// AppliesTo reports whether the resource participates in a project with the supplied capabilities.
func (d ResourceDefinition) AppliesTo(components Components) bool {
	return d.applicable != nil && d.applicable(components)
}

// Driver returns one driver definition by its normalized name.
func (d ResourceDefinition) Driver(name string) (DriverDefinition, bool) {
	name = CanonicalResourceDriver(d.Key, name)
	for _, driver := range d.Drivers {
		if driver.Name == name {
			return driver, true
		}
	}
	return DriverDefinition{}, false
}

// DefaultSelection keeps new-project choices aligned with the fallbacks generated into resource managers.
func (d ResourceDefinition) DefaultSelection(components Components) (DriverSelection, error) {
	active := d.DefaultDriver
	supported := append([]string(nil), d.DefaultSupportedDrivers...)
	switch d.Key {
	case ResourceDatabase:
		active = components.DatabaseDriver()
		if components.DemoApp {
			active = "mysql"
			supported = []string{"sqlite", "mysql"}
		} else {
			supported = []string{active}
		}
		if active == "" {
			return DriverSelection{}, fmt.Errorf("database capability requires a selected database driver")
		}
	case ResourceMail:
		if components.Docker {
			active = "smtp"
		}
	}
	return DriverSelection{Active: active, Supported: supported}, nil
}

// EnvironmentKey keeps the database's DB prefix and the other resource prefixes out of consumer-specific switches.
func (d ResourceDefinition) EnvironmentKey(suffix string) string {
	suffix = str.Of(suffix).TrimSpace().ToUpper().String()
	if suffix == "" {
		return d.EnvironmentPrefix
	}
	return d.EnvironmentPrefix + "_" + suffix
}

// NamedDriverDefault preserves the resources that deliberately inherit their root driver while defaulting the rest locally.
func (d ResourceDefinition) NamedDriverDefault(rootDriver string) string {
	if d.NamedDriversInheritRoot {
		return CanonicalResourceDriver(d.Key, rootDriver)
	}
	return d.DefaultDriver
}

// CanonicalResourceDriver maps supported compatibility aliases onto the catalog identity used by plans and service discovery.
func CanonicalResourceDriver(resource ResourceKey, name string) string {
	name = normalizeDriverName(name)
	if resource != ResourceDatabase {
		return name
	}
	switch name {
	case "mariadb":
		return "mysql"
	case "postgresql":
		return "postgres"
	case "sqlite3":
		return "sqlite"
	default:
		return name
	}
}

// cloneResourceDefinition prevents callers from mutating the process-wide catalog.
func cloneResourceDefinition(definition ResourceDefinition) ResourceDefinition {
	cloned := definition
	cloned.DefaultSupportedDrivers = append([]string(nil), definition.DefaultSupportedDrivers...)
	cloned.Drivers = make([]DriverDefinition, len(definition.Drivers))
	for index, driver := range definition.Drivers {
		cloned.Drivers[index] = driver
		cloned.Drivers[index].EndpointEnvironment = append([]DriverEnvironmentPlaceholder(nil), driver.EndpointEnvironment...)
		cloned.Drivers[index].Environment = append([]DriverEnvironmentPlaceholder(nil), driver.Environment...)
	}
	cloned.NamedResources = append([]GeneratedNamedResourceDefinition(nil), definition.NamedResources...)
	return cloned
}

// driverEnvironmentPlaceholder keeps catalog declarations concise while retaining explicit safe examples.
func driverEnvironmentPlaceholder(key string, example string, description string) DriverEnvironmentPlaceholder {
	return DriverEnvironmentPlaceholder{Key: key, Example: example, Description: description}
}
