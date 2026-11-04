// Package nodefiles removes the files the collector keeps for a node outside
// the database, which the deletion of the node rows leaves behind.
package nodefiles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// Dirs are the directories holding files per node. An empty directory is
// skipped.
type Dirs struct {
	// Stats holds the whisper series of the metrics of the nodes, in
	// nodes/<node_id>, written by the feeder and the workers.
	Stats string
	// Sysreport holds the git repository of the sysreport of each node, in
	// <node_id>, committed by the scheduler.
	Sysreport string
}

// Paths returns the paths of the files of the node: a node id other than a
// canonical uuid is refused, as it is joined to the paths.
func (d Dirs) Paths(nodeID string) ([]string, error) {
	id, err := uuid.Parse(nodeID)
	if err != nil || id.String() != nodeID {
		return nil, fmt.Errorf("invalid node id %q", nodeID)
	}
	var paths []string
	if d.Stats != "" {
		paths = append(paths, filepath.Join(d.Stats, "nodes", nodeID))
	}
	if d.Sysreport != "" {
		paths = append(paths, filepath.Join(d.Sysreport, nodeID))
	}
	return paths, nil
}

// Remove removes the files of the node. The files absent are not an error.
func (d Dirs) Remove(nodeID string) error {
	paths, err := d.Paths(nodeID)
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range paths {
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
