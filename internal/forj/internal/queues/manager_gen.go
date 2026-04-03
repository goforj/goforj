package queues

import (
	"fmt"
	"strings"

	"github.com/goforj/env/v2"
	"github.com/goforj/queue"
	"github.com/goforj/str"
)

const defaultQueueName = "default"

const (
	driverMySQL      = "mysql"
	driverNATS       = "nats"
	driverNull       = "null"
	driverPostgres   = "postgres"
	driverRabbitMQ   = "rabbitmq"
	driverRedis      = "redis"
	driverSQLite     = "sqlite"
	driverSQS        = "sqs"
	driverSync       = "sync"
	driverWorkerpool = "workerpool"
)

var queueRootKeys = []string{
	"DRIVER",
	"WORKERS",
	"DEFAULT_QUEUE",
	"ADDR",
	"PASSWORD",
	"DB",
	"QUEUES",
	"SERVER_LOG_LEVEL",
	"URL",
	"REGION",
	"ENDPOINT",
	"ACCESS_KEY",
	"SECRET_KEY",
	"DSN",
	"WORKERPOOL_WORKERS",
	"WORKERPOOL_BUFFER",
	"WORKERPOOL_TASK_TIMEOUT_SECONDS",
	"PROCESSING_RECOVERY_GRACE_SECONDS",
	"PROCESSING_LEASE_NO_TIMEOUT_SECONDS",
}

type Manager struct {
	defaultQueue *queue.Queue
	critical     *queue.Queue
}

func NewManager() (*Manager, error) {
	return NewManagerWithObserver(nil, nil)
}

func NewManagerWithObserver(observer queue.Observer, logger queue.Logger) (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("QUEUE"), observer, logger)
}

func (m *Manager) Default() *queue.Queue {
	return m.defaultQueue
}

func (m *Manager) Names() []string {
	names := []string{"default"}
	if m.critical != nil {
		names = append(names, "critical")
	}
	return names
}

func (m *Manager) Named(name string) *queue.Queue {
	switch str.Of(name).TrimSpace().ToLower().String() {
	case "", "default":
		return m.defaultQueue
	case "critical":
		return m.critical
	default:
		return nil
	}
}

func (m *Manager) Critical() *queue.Queue {
	return m.critical
}

func newManagerFromEnv(queueScope env.Scope, observer queue.Observer, logger queue.Logger) (*Manager, error) {
	defaultQueue, err := buildQueue(defaultQueueName, queueScope, observer, logger)
	if err != nil {
		return nil, err
	}
	manager := &Manager{defaultQueue: defaultQueue}

	for _, child := range queueScope.ChildNames(queueRootKeys) {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		queueInstance, err := buildQueue(name, queueScope.Child(child), observer, logger)
		if err != nil {
			return nil, err
		}
		switch name {
		case "critical":
			manager.critical = queueInstance
		}
	}

	return manager, nil
}

func buildQueue(_ string, scope env.Scope, observer queue.Observer, logger queue.Logger) (*queue.Queue, error) {
	driver := str.Of(scope.Get("DRIVER", driverWorkerpool)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverWorkerpool
	}

	defaultQueue := queueDefaultQueue(scope)
	options := []queue.Option{
		queue.WithWorkers(queueWorkerCount(scope)),
	}

	switch driver {
	case driverNull:
		return queue.New(queue.Config{
			Driver:       queue.DriverNull,
			DefaultQueue: defaultQueue,
			Observer:     observer,
			Logger:       logger,
		}, options...)
	case driverSync:
		return queue.New(queue.Config{
			Driver:       queue.DriverSync,
			DefaultQueue: defaultQueue,
			Observer:     observer,
			Logger:       logger,
		}, options...)
	case driverWorkerpool:
		return queue.New(queue.Config{
			Driver:       queue.DriverWorkerpool,
			DefaultQueue: defaultQueue,
			Observer:     observer,
			Logger:       logger,
		}, options...)
	default:
		return nil, fmt.Errorf("queue: unsupported driver %q", driver)
	}
}

func queueWorkerCount(scope env.Scope) int {
	workers := scope.GetInt("WORKERS", "30")
	if workers <= 0 {
		workers = scope.GetInt("WORKERPOOL_WORKERS", "30")
	}
	if workers <= 0 {
		return 30
	}
	return workers
}

func queueDefaultQueue(scope env.Scope) string {
	value := strings.TrimSpace(scope.Get("DEFAULT_QUEUE", "default"))
	if value == "" {
		return "default"
	}
	return value
}
