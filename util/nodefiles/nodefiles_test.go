package nodefiles

import (
	"os"
	"path/filepath"
	"testing"
)

const nodeID = "0afa7ea7-a181-4bd0-b73c-178c35163ec5"

func mkfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestRemove checks the series and the sysreport of the node are removed, and
// those of the other nodes and the other series kept.
func TestRemove(t *testing.T) {
	root := t.TempDir()
	d := Dirs{Stats: filepath.Join(root, "stats"), Sysreport: filepath.Join(root, "sysreport")}
	const other = "3d0b33a0-c8e7-4c10-8cbf-2dd009b43713"
	removed := []string{
		filepath.Join(d.Stats, "nodes", nodeID, "cpu", "usr.wsp"),
		filepath.Join(d.Stats, "nodes", nodeID, "checks", "fs_u", "root.wsp"),
		filepath.Join(d.Sysreport, nodeID, ".git", "HEAD"),
	}
	kept := []string{
		filepath.Join(d.Stats, "nodes", other, "cpu", "usr.wsp"),
		filepath.Join(d.Stats, "metrics", "1.wsp"),
		filepath.Join(d.Sysreport, other, ".git", "HEAD"),
	}
	for _, p := range append(removed, kept...) {
		mkfile(t, p)
	}
	if err := d.Remove(nodeID); err != nil {
		t.Fatal(err)
	}
	for _, p := range removed {
		if exists(p) {
			t.Errorf("%s not removed", p)
		}
	}
	for _, p := range kept {
		if !exists(p) {
			t.Errorf("%s removed", p)
		}
	}
	// Nothing left to remove is not an error.
	if err := d.Remove(nodeID); err != nil {
		t.Errorf("second remove: %s", err)
	}
}

// TestRemoveRefusesInvalidNodeID checks a node id other than a canonical uuid,
// which would designate another path, is refused.
func TestRemoveRefusesInvalidNodeID(t *testing.T) {
	root := t.TempDir()
	d := Dirs{Stats: filepath.Join(root, "stats"), Sysreport: filepath.Join(root, "sysreport")}
	mkfile(t, filepath.Join(d.Sysreport, "keep"))
	for _, id := range []string{"", ".", "..", "../sysreport", "*", "0AFA7EA7-A181-4BD0-B73C-178C35163EC5", "{" + nodeID + "}", "urn:uuid:" + nodeID} {
		if err := d.Remove(id); err == nil {
			t.Errorf("%q: no error", id)
		}
	}
	if !exists(filepath.Join(d.Sysreport, "keep")) {
		t.Error("files removed")
	}
}

// TestPathsSkipsEmptyDirs checks a directory not configured is skipped.
func TestPathsSkipsEmptyDirs(t *testing.T) {
	paths, err := Dirs{Sysreport: "/oc3/uploads/sysreport"}.Paths(nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/oc3/uploads/sysreport/"+nodeID {
		t.Errorf("paths %v", paths)
	}
}
