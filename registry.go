package zkregistry

import (
	"encoding/json"
	"errors"
	"fmt"
	stdLog "log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agrarianlabs/zkwatcher"
	"github.com/samuel/go-zookeeper/zk"
)

// ZKRegistry is an implementation of the registry with Zookeeper.
type ZKRegistry struct {
	// Underlying ZK connection.
	conn   *zk.Conn
	logger zk.Logger

	// Internal meta data.
	offset       uint // offset of the original ZKPath used.
	tickInterval time.Duration

	// Internal controls.
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Registry state.
	lock     sync.RWMutex
	services map[string]map[string][]string
}

// Common errors.
var (
	ErrNilConn         = errors.New("can't create registry with <nil> zk connection")
	ErrServiceNotFound = errors.New("service not found")
)

// New .
func New(conn *zk.Conn, zkPath string, logger zk.Logger) (*ZKRegistry, error) {
	if conn == nil {
		return nil, ErrNilConn
	}
	if logger == nil {
		logger = stdLog.New(os.Stderr, "", stdLog.LstdFlags)
	}

	// Make sure the path exists,
	if err := createTree(conn, zkPath); err != nil {
		return nil, err
	}

	reg := &ZKRegistry{
		conn:         conn,
		logger:       logger,
		offset:       uint(len(strings.Split(sanitizePath(zkPath), "/"))),
		services:     map[string]map[string][]string{},
		stopChan:     make(chan struct{}),
		tickInterval: 10 * time.Second,
	}

	if err := reg.startWatcher(zkPath); err != nil {
		_ = reg.Close() // Best effort.
		return nil, err
	}

	return reg, nil
}

func (reg *ZKRegistry) watcher(watcher *zkwatcher.Watcher) {
	ticker := time.NewTicker(reg.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-reg.stopChan:
			return
		case <-ticker.C:
		case event := <-watcher.C:
			name, version, endpoint, err := ParseConfigPath(event.Path, reg.offset)
			if err != nil {
				reg.logger.Printf("error parsing the event from zookeeper: %s (%v)", err, event.Error)
				break
			}
			if event.Error != nil {
				reg.logger.Printf("watch error from zookeeper for %s/%s: %s", name, version, event.Error)
				break
			}
			// no error with empty name means the event is not for an endpoint, discard.
			if name == "" {
				break
			}
			switch event.Type {
			case zkwatcher.Create:
				// If version or endpoint or nil, it is an event on parents. Discard.
				if version != "" && endpoint != "" {
					reg.Add(name, version, endpoint)
				}
			case zkwatcher.Delete:
				if version == "" {
					reg.DeleteService(name)
				} else if endpoint == "" {
					reg.DeleteVersion(name, version)
				} else {
					reg.DeleteEndpoint(name, version, endpoint)
				}
			case zkwatcher.Update:
				// We do not use the value, discard Update events.
			}
		}
	}
}

func (reg *ZKRegistry) startWatcher(zkPath string) error {
	watcher := zkwatcher.NewWatcher(reg.conn)
	if err := watcher.WatchLimit(zkPath, 2); err != nil {
		return err
	}
	reg.wg.Add(1)
	go func() {
		defer reg.wg.Done()
		reg.watcher(watcher)
	}()

	return nil
}

// Close terminates the registry.
func (reg *ZKRegistry) Close() error {
	close(reg.stopChan)
	reg.wg.Wait()
	return nil
}

// SetLogger overrides the default logger.
func (reg *ZKRegistry) SetLogger(logger zk.Logger) *ZKRegistry {
	reg.conn.SetLogger(logger)
	reg.logger = logger
	return reg
}

// String returns the json representation of the registered services.
// NOTE: induces a lock. You should not let users call this.
func (reg *ZKRegistry) String() string {
	var ret string
	reg.lock.RLock()
	buf, err := json.Marshal(reg.services)
	if err != nil {
		ret = fmt.Sprintln(reg.services)
	}
	reg.lock.RUnlock()
	if err == nil {
		ret = string(buf)
	}
	return ret
}

/// registry.Registry implementation.

// Lookup return the endpoint list for the given service name/version.
func (reg *ZKRegistry) Lookup(name, version string) ([]string, error) {
	reg.lock.RLock()
	targets, ok := reg.services[name][version]
	reg.lock.RUnlock()
	if !ok {
		return nil, ErrServiceNotFound
	}
	return targets, nil
}

// Failure marks the given endpoint for service name/version as failed.
func (reg *ZKRegistry) Failure(name, version, endpoint string, err error) {
	// Would be used to remove an endpoint from the rotation, log the failure, etc.
	reg.logger.Printf("Error accessing %s/%s (%s): %s", name, version, endpoint, err)
}

// Add adds the given endpoit for the service name/version.
func (reg *ZKRegistry) Add(name, version, endpoint string) {
	reg.lock.Lock()

	service, ok := reg.services[name]
	if !ok {
		service = map[string][]string{}
		reg.services[name] = service
	}
	service[version] = append(service[version], endpoint)

	reg.lock.Unlock()
}

// DeleteEndpoint removes the given endpoit for the service name/version.
func (reg *ZKRegistry) DeleteEndpoint(name, version, endpoint string) {
	reg.lock.Lock()

	service, ok := reg.services[name]
	if !ok {
		reg.lock.Unlock()
		return
	}
begin:
	for i, svc := range service[version] {
		if svc == endpoint {
			copy(service[version][i:], service[version][i+1:])
			service[version][len(service[version])-1] = ""
			service[version] = service[version][:len(service[version])-1]
			goto begin
		}
	}

	reg.lock.Unlock()
}

// DeleteVersion removes the given version for the service name.
func (reg *ZKRegistry) DeleteVersion(name, version string) {
	reg.lock.Lock()

	service, ok := reg.services[name]
	if !ok {
		reg.lock.Unlock()
		return
	}
	delete(service, version)

	reg.lock.Unlock()
}

// DeleteService removes the given service.
func (reg *ZKRegistry) DeleteService(name string) {
	reg.lock.Lock()

	delete(reg.services, name)

	reg.lock.Unlock()
}
