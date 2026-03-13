package queue

import (
	"fmt"
	"strings"

	"github.com/goforj/env/v2"
	goforjqueue "github.com/goforj/queue"
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
	defaultQueue *goforjqueue.Queue
	critical     *goforjqueue.Queue
}

func NewManager() (*Manager, error) {
	return NewManagerWithObserver(nil)
}

func NewManagerWithObserver(observer goforjqueue.Observer) (*Manager, error) {
	return newManagerFromEnv(env.WithPrefix("QUEUE"), observer)
}

func (m *Manager) Default() *goforjqueue.Queue {
	return m.defaultQueue
}

func (m *Manager) Critical() *goforjqueue.Queue {
	return m.critical
}

func newManagerFromEnv(queueScope env.Scope, observer goforjqueue.Observer) (*Manager, error) {
	defaultQueue, err := buildQueue(defaultQueueName, queueScope, observer)
	if err != nil {
		return nil, err
	}
	manager := &Manager{defaultQueue: defaultQueue}

	for _, child := range queueScope.ChildNames(queueRootKeys) {
		name := str.Of(child).TrimSpace().ToLower().String()
		if name == "" {
			continue
		}
		queueInstance, err := buildQueue(name, queueScope.Child(child), observer)
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

func buildQueue(_ string, scope env.Scope, observer goforjqueue.Observer) (*goforjqueue.Queue, error) {
	driver := str.Of(scope.Get("DRIVER", driverWorkerpool)).TrimSpace().ToLower().String()
	if driver == "" {
		driver = driverWorkerpool
	}

	defaultQueue := queueDefaultQueue(scope)
	options := []goforjqueue.Option{
		goforjqueue.WithWorkers(queueWorkerCount(scope)),
	}

	switch driver {
	case driverNull:
		return goforjqueue.New(goforjqueue.Config{
			Driver:       goforjqueue.DriverNull,
			DefaultQueue: defaultQueue,
			Observer:     observer,
		}, options...)
	case driverSync:
		return goforjqueue.New(goforjqueue.Config{
			Driver:       goforjqueue.DriverSync,
			DefaultQueue: defaultQueue,
			Observer:     observer,
		}, options...)
	case driverWorkerpool:
		return goforjqueue.New(goforjqueue.Config{
			Driver:       goforjqueue.DriverWorkerpool,
			DefaultQueue: defaultQueue,
			Observer:     observer,
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
