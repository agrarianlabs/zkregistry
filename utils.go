package zkregistry

import (
	"fmt"
	"path"
	"strings"

	"github.com/samuel/go-zookeeper/zk"
)

// sanitizePath trims the `/` prefix and suffix.
func sanitizePath(zkPath string) string {
	// Sanitize the path.
	if len(zkPath) > 0 && zkPath[0] == '/' {
		zkPath = zkPath[1:]
	}
	if len(zkPath) > 0 && zkPath[len(zkPath)-1] == '/' {
		zkPath = zkPath[:len(zkPath)-1]
	}
	return zkPath
}

// ParseConfigPath extracts name, version and endpoint from a Zookeeper event.
var ParseConfigPath = parseConfigPath

// parseConfigPath parses the given zkPath and extract the service name, version and endpoint.
// offset is the number of element to discard at the beginning of the path.
func parseConfigPath(zkPath string, offset uint) (serviceName, serviceVersion, endpoint string, err error) {
	off := int(offset)
	zkPath = sanitizePath(zkPath)

	// Explode the path.
	parts := strings.Split(zkPath, "/")
	switch {
	case len(parts) < off || len(parts) > off+3:
		return "", "", "", fmt.Errorf("invalid path received: %q", zkPath)
	case len(parts) == off: // Event on root. Discard.
		return "", "", "", nil
	case len(parts) == off+1: // Event on service.
		return parts[off], "", "", nil
	case len(parts) == off+2: // Event on service version.
		return parts[off], parts[off+1], "", nil
	default: // Event on service endpoint.
		return parts[off], parts[off+1], parts[off+2], nil
	}
}

// createTree recursively creates the given path.
// TODO: remove and use zkConnector.
func createTree(conn *zk.Conn, zkPath string) error {
	target := "/"
	for _, elem := range strings.Split(zkPath, "/") {
		if elem != "" {
			target = path.Join(target, elem)
			if ok, _, err := conn.Exists(target); err != nil {
				return fmt.Errorf("error looking up %q: %s", target, err)
			} else if ok {
				continue
			}
			if _, err := conn.Create(target, nil, 0, zk.WorldACL((zk.PermAll))); err != nil {
				return fmt.Errorf("error creating %q: %s", target, err)
			}
		}
	}
	return nil
}

// removeTree recursively removes the given path.
// TODO: remove and use zkConnector.
func removeTree(conn *zk.Conn, zkPath string) error {
	children, _, err := conn.Children(zkPath)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := removeTree(conn, path.Join(zkPath, child)); err != nil {
			return err
		}
	}
	if err := conn.Delete(zkPath, -1); err != nil {
		return fmt.Errorf("error removing %q: %s", zkPath, err)
	}
	return nil
}
