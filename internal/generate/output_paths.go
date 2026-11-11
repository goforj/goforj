package generate

const (
	// CacheManagerPath identifies the manager source owned by cache generation.
	CacheManagerPath = "internal/caches/manager_gen.go"
	// CacheAccessorsPath identifies accessors derived from application cache declarations.
	CacheAccessorsPath = "internal/caches/accessors_gen.go"
	// QueueManagerPath identifies the manager source owned by queue generation.
	QueueManagerPath = "internal/queues/manager_gen.go"
	// QueueAccessorsPath identifies accessors derived from application queue declarations.
	QueueAccessorsPath = "internal/queues/accessors_gen.go"
	// StorageManagerPath identifies the manager source owned by storage generation.
	StorageManagerPath = "internal/storages/manager_gen.go"
	// StorageAccessorsPath identifies accessors derived from application storage declarations.
	StorageAccessorsPath = "internal/storages/accessors_gen.go"
	// EventsManagerPath identifies the manager source owned by event generation.
	EventsManagerPath = "internal/events/manager_gen.go"
	// EventsAccessorsPath identifies accessors derived from application event declarations.
	EventsAccessorsPath = "internal/events/accessors_gen.go"
	// MailManagerPath identifies the manager source owned by mail generation.
	MailManagerPath = "internal/mail/manager_gen.go"
	// MailAccessorsPath identifies accessors derived from application mail declarations.
	MailAccessorsPath = "internal/mail/accessors_gen.go"
	// DatabaseConnectionsPath identifies accessors derived from application database declarations.
	DatabaseConnectionsPath = "internal/database/connections_gen.go"
	// ObservabilityTargetsPath identifies scrape discovery whose management is delegated by runtime observability policy.
	ObservabilityTargetsPath = "containers/observability/vmagent/metrics-targets.json"
)

// OutputPaths exposes the same selected output contracts used by generator publication and upgrade inspection.
func OutputPaths(selection GenerationSelection) []string {
	var paths []string
	for _, group := range []struct {
		enabled bool
		paths   []string
	}{
		{selection.Cache, []string{CacheManagerPath, CacheAccessorsPath}},
		{selection.Queue, []string{QueueManagerPath, QueueAccessorsPath}},
		{selection.Storage, []string{StorageManagerPath, StorageAccessorsPath}},
		{selection.Events, []string{EventsManagerPath, EventsAccessorsPath}},
		{selection.Mail, []string{MailManagerPath, MailAccessorsPath}},
		{selection.Database, []string{DatabaseConnectionsPath}},
		{selection.Observability, []string{ObservabilityTargetsPath}},
	} {
		if group.enabled {
			paths = append(paths, group.paths...)
		}
	}
	return paths
}
