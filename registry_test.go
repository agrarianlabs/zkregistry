package zkregistry

import (
	"bytes"
	"log"
	"path"
	"testing"
	"time"

	"github.com/samuel/go-zookeeper/zk"
)

func TestNewZKRegistryFailure(t *testing.T) {
	if _, err := New(nil, "", nil); err != ErrNilConn {
		t.Fatalf("Unexpected error.\nExpect:\t%v\nGot:\t%v", ErrNilConn, err)
	}
	if err := testTimeout(t,
		"invalid zk conn object, connect",
		500*time.Millisecond,
		func(t *testing.T) {
			if _, err := New(&zk.Conn{}, "", nil); err != ErrNilConn {
				t.Fatalf("Unexpected error.\nExpect:\t%v\nGot:\t%v", ErrNilConn, err)
			}
		}); err == nil {
		t.Fatal("Invalid zk.Conn object should fail the connection")
	}
}

// Make sure the watcher connects to ZK and that the Close() does not block.
// Also checks that the discovery path gets automatically created.
func TestNewZKRegistry(t *testing.T) {
	conn := zkConnect(t)
	defer conn.Close()

	zkPath := path.Join(conn.prefix, "/test/discovery")

	reg, err := New(conn.conn, zkPath, discardLogger)
	if err != nil {
		t.Fatalf("Error creating new registry: %s", err)
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Error closing the registry: %s", err)
	}

	assertZKPathExist(t, conn, zkPath)
}

func TestAddEndpoint(t *testing.T) {
	conn := zkConnect(t)
	defer conn.Close()

	// Manually register a new service in ZK.
	assertCreateTree(t, conn, "/discovery/name/version/addr")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", []string{"addr"}, nil)

	// TODO: test to add a 2nd then a 3rd with same name/version/addr
}

// TODO: check the watcher's stats for goroutines.
func TestDeleteEndpoint(t *testing.T) {
	conn := zkConnect(t)
	defer conn.Close()

	// Manually register a new service in ZK.
	assertCreateTree(t, conn, "/discovery/name/version/addr")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", []string{"addr"}, nil)

	// The service is registered. Now manually remove it.
	assertRemoveTree(t, conn, "/discovery/name/version/addr")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", []string{}, nil)

	// Manually check the state of the registry as well.
	conn.ZKRegistry.lock.RLock()
	defer conn.ZKRegistry.lock.RUnlock()

	service, ok := conn.ZKRegistry.services["name"]
	if !ok {
		t.Fatal("services map should not be empty after removing an endpoint")
	}
	version, ok := service["version"]
	if !ok {
		t.Fatal("version map should not be empty after removing an endpoint")
	}
	for _, endpoint := range version {
		if endpoint == "addr" {
			t.Fatal("The endpoint should not be present after removing it")
		}
	}
}

// TODO: check the watcher's stats for goroutines.
// TODO: refactor all those test in one.
func TestDeleteVersion(t *testing.T) {
	conn := zkConnect(t)
	defer conn.Close()

	// Manually register a new service in ZK.
	assertCreateTree(t, conn, "/discovery/name/version/addr")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", []string{"addr"}, nil)

	// The service is registered. Now manually remove it.
	assertRemoveTree(t, conn, "/discovery/name/version")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", nil, ErrServiceNotFound)

	// Manually check the state of the registry as well.
	conn.ZKRegistry.lock.RLock()
	defer conn.ZKRegistry.lock.RUnlock()

	service, ok := conn.ZKRegistry.services["name"]
	if !ok {
		t.Fatal("services map should not be empty after removing just the version")
	}
	if _, ok := service["version"]; ok {
		t.Fatal("version map should not exists after removing the version")
	}
}

// TODO: check the watcher's stats for goroutines.
// TODO: refactor all those test in one.
func TestDeleteService(t *testing.T) {
	conn := zkConnect(t)
	defer conn.Close()

	// Manually register a new service in ZK.
	assertCreateTree(t, conn, "/discovery/name/version/addr")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", []string{"addr"}, nil)

	// The service is registered. Now manually remove it.
	assertRemoveTree(t, conn, "/discovery/name")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", nil, ErrServiceNotFound)

	// Manually check the state of the registry as well.
	conn.ZKRegistry.lock.RLock()
	defer conn.ZKRegistry.lock.RUnlock()

	if _, ok := conn.ZKRegistry.services["name"]; ok {
		t.Fatal("services map should be empty after removing the service")
	}
}

// TODO: check the watcher's stats for goroutines.
func TestDeleteNotExisting(t *testing.T) {
	conn := zkConnect(t)
	defer conn.Close()

	// Make sure the service's endpoint deos not exists.
	assertLookupResult(t, conn, "name", "version", nil, ErrServiceNotFound)
	// Call the delete, should not panic.
	conn.ZKRegistry.DeleteEndpoint("name", "version", "addr")
	// Make sure it is stll not there.
	assertLookupResult(t, conn, "name", "version", nil, ErrServiceNotFound)

	// Call the delete, should not panic.
	conn.ZKRegistry.DeleteVersion("name", "version")
	// Make sure it is stll not there.
	assertLookupResult(t, conn, "name", "version", nil, ErrServiceNotFound)

	// Call the delete, should not panic.
	conn.ZKRegistry.DeleteService("name")
	// Make sure it is stll not there.
	assertLookupResult(t, conn, "name", "version", nil, ErrServiceNotFound)

}

func TestNewZKRegistryFailureClosedZK(t *testing.T) {
	conn, _, err := zk.Connect([]string{testZKHost}, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
	_ = testTimeout(t, "New Registry on closed ZK connection", 1*time.Second, func(t *testing.T) {
		if reg, err := New(conn, "/", discardLogger); err == nil {
			_ = reg.Close() // best effort.
			t.Fatal("New registry on closed ZK connection should error")
		}
	})
}

func TestNewZKRegistryInvalidZK(t *testing.T) {
	p := newTCPProxy(t, "321.321.321.321:321")
	p.Start()
	defer p.Stop()
	conn, _, err := zk.Connect([]string{p.URL}, 100*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if reg, err := New(conn, "/test/discovery", discardLogger); err == nil {
		_ = reg.Close() // best effort.
		t.Fatal("New registry on closed ZK connection should error")
	}

	// Skip the createTree and try to start the watcher.
	if reg, err := New(conn, "/", discardLogger); err == nil {
		_ = reg.Close() // best effort.
		t.Fatal("New registry on closed ZK connection should error")
	}
}

func TestRegStringer(t *testing.T) {
	conn := zkConnect(t)
	defer conn.Close()

	// Manually register a new service in ZK.
	assertCreateTree(t, conn, "/discovery/name/version/addr")

	// Give time to ZK to signal the event.
	// The test will fail if the event does not arrive before 1ms.
	time.Sleep(1 * time.Millisecond)

	// Make sure the registry picked it up.
	assertLookupResult(t, conn, "name", "version", []string{"addr"}, nil)

	if expect, got := `{"name":{"version":["addr"]}}`, conn.ZKRegistry.String(); expect != got {
		t.Fatalf("Unexpected data.\nExpect:\t%s\nGot:\t%s", expect, got)
	}
}

func TestFailure(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	logger := log.New(buf, "", 0)

	conn, _, err := zk.Connect([]string{testZKHost}, 1*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetLogger(discardLogger)
	defer conn.Close()

	reg, err := New(conn, "/", logger)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reg.Close() }() // Best effort.
	reg.Failure("name", "version", "addr", ErrNilConn)

	expect := `Error accessing name/version (addr): can't create registry with <nil> zk connection` + "\n"
	if got := buf.String(); expect != got {
		t.Fatalf("Unexpected data.\nExpect:\t%s\nGot:\t%s", expect, got)
	}
}
