package zkregistry

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"path"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/samuel/go-zookeeper/zk"
)

// global test zk instance host.
var testZKHost string

// randomName generates a random string prefixed with "zkwatchtest".
func randomName() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

	result := make([]byte, 16)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))]
	}
	return "zkwatchtest_" + string(result)
}

// NOTE: return error so we can test expected timeout vs unexpected.
func testTimeout(t *testing.T, text string, duration time.Duration, fct func(t *testing.T)) error {
	timeout := time.NewTicker(duration)
	defer timeout.Stop()

	ch := make(chan struct{})
	go func() { defer close(ch); fct(t) }()
	select {
	case <-timeout.C:
		return fmt.Errorf("%s - function timed out", text)
	case <-ch:
	}
	return nil
}

var discardLogger = log.New(ioutil.Discard, "", 0)

// zkConn wraps the zk.Conn with a prefix.
type zkConn struct {
	*ZKRegistry
	conn   *zk.Conn
	prefix string
}

// Close wraps zk.Conn.Close, removes the prefix
// and call zk.Conn.Close.
func (c *zkConn) Close() {
	_ = removeTree(c.conn, c.prefix)
	c.conn.Close()
	_ = c.ZKRegistry.Close() // Best effort.
}

func zkConnect(t *testing.T) *zkConn {
	file, line := getCaller(t, 1)

	// Connect to ZK.
	conn, _, err := zk.Connect([]string{testZKHost}, 1*time.Second)
	if err != nil {
		t.Fatalf("[%s:%d] Unable to connect to ZK: %s", file, line, err)
	}
	conn.SetLogger(discardLogger)

	prefix := "/" + randomName() + "/"

	// Create the registry.
	reg, err := New(conn, prefix+"discovery", discardLogger)
	if err != nil {
		t.Fatalf("[%s:%d] Error creating new registry: %s", file, line, err)
	}
	reg.SetLogger(discardLogger)

	return &zkConn{
		ZKRegistry: reg,
		conn:       conn,
		prefix:     prefix,
	}
}

func assertZKPathExist(t *testing.T, conn *zkConn, zkPath string) {
	file, line := getCaller(t, 1)
	if ok, _, err := conn.conn.Exists(zkPath); err != nil {
		t.Fatalf("[%s:%d] Error looking up ZK path %q: %s", file, line, zkPath, err)
	} else if !ok {
		t.Fatalf("[%s:%d] %q should exist in zookeeper", file, line, zkPath)
	}
}

// assertCreateTree prefix the given zkPath and creates the nodes.
func assertCreateTree(t *testing.T, conn *zkConn, zkPath string) {
	file, line := getCaller(t, 1)
	if err := createTree(conn.conn, path.Join(conn.prefix, zkPath)); err != nil {
		t.Fatalf("[%s:%d] Error creating %q: %s", file, line, zkPath, err)
	}
}

// assertRemoveTree prefix the given zkPath and remove the nodes.
func assertRemoveTree(t *testing.T, conn *zkConn, zkPath string) {
	file, line := getCaller(t, 1)
	if err := removeTree(conn.conn, path.Join(conn.prefix, zkPath)); err != nil {
		t.Fatalf("[%s:%d] Error removing %q: %s", file, line, zkPath, err)
	}
}

func assertLookupResult(t *testing.T, conn *zkConn, svcName, svcVersion string, expect []string, expectErr error) {
	file, line := getCaller(t, 1)
	if got, err := conn.Lookup(svcName, svcVersion); err != expectErr {
		t.Fatalf("[%s:%d] Unexpected error looking up on the registry for %s/%s:\nExpect:\t%v\nGot:\t%v",
			file, line, svcName, svcVersion, expectErr, err)
	} else if !reflect.DeepEqual(expect, got) {
		t.Fatalf("[%s:%d] Unexpected value.\nExpect:\t%s\nGot:\t%v", file, line, expect, got)
	}
}

type tcpProxy struct {
	t          *testing.T
	targetAddr string
	ln         net.Listener
	URL        string
}

func newTCPProxy(t *testing.T, targetAddr string) *tcpProxy {
	return &tcpProxy{
		t:          t,
		targetAddr: targetAddr,
	}
}

func (p *tcpProxy) Start() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		p.t.Fatal(err)
	}
	p.ln = ln
	p.URL = ln.Addr().String()
	go func() {
		for {
			connClient, err := ln.Accept()
			if err != nil {
				// Discard accept errors.
				return
			}
			connServer, err := net.DialTimeout("tcp", p.targetAddr, 100*time.Millisecond)
			if err != nil {
				// Discard dial errors. Let the caller check for issues.
				return
			}
			go func() { defer func() { _ = connServer.Close() }(); _, _ = io.Copy(connServer, connClient) }()
			go func() { _, _ = io.Copy(connClient, connServer) }()
		}
	}()
}

func (p *tcpProxy) Stop() {
	_ = p.ln.Close() // Best effort.
}

func getCaller(t testing.TB, offset int) (file string, line int) {
	// Pull the line of the caller.
	_, file, line, ok := runtime.Caller(offset + 1)
	if !ok {
		t.Fatal("Error looking up callstack")
	}

	file = path.Base(file)
	return file, line
}

// TestMain initializes the random seed and pull the zk-host from command line.
func TestMain(m *testing.M) {
	rand.Seed(time.Now().UTC().UnixNano())

	// Get the testing zookeeper host from command line.
	flag.StringVar(
		&testZKHost,
		"zk-host",
		"",
		"test zookeeper instance address",
	)
	flag.Parse()

	if testZKHost == "" {
		fmt.Fprintf(os.Stderr, "Usage: go test [opts] -zk-host <zk host>\nPrefer: make test [SKIP_FMT=1]\n\n")
		os.Exit(1)
	}

	os.Exit(m.Run())
}
