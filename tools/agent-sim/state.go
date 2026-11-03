package main

import (
	"crypto/md5"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

type (
	// clusterState is the status of the objects of a cluster as its speaker
	// knows it, changed by the events of the run.
	clusterState struct {
		c         *cluster
		rand      *rand.Rand
		startedAt time.Time
		// inst is the status of the instances, by service then node index.
		inst    [][]*instanceState
		objAt   []time.Time
		gen     int
		version string
	}

	instanceState struct {
		// res is the status of each resource of the service.
		res       []string
		updatedAt time.Time
		startedAt time.Time
	}
)

func newClusterState(spec fleetSpec, c *cluster, version string, now time.Time) *clusterState {
	st := &clusterState{
		c:         c,
		rand:      spec.newRand("events", c.idx),
		startedAt: now,
		version:   version,
	}
	for _, s := range c.services {
		row := make([]*instanceState, len(c.nodes))
		for ni := range c.nodes {
			is := &instanceState{res: make([]string, len(s.resources)), updatedAt: now, startedAt: now}
			status := "down"
			if s.topology == "flex" || ni == s.primary {
				status = "up"
			}
			for ri := range is.res {
				is.res[ri] = status
			}
			row[ni] = is
		}
		st.inst = append(st.inst, row)
		st.objAt = append(st.objAt, now)
	}
	return st
}

// running tells if a service is meant to run on a node: its primary node for
// a failover service, every node for a flex one.
func (st *clusterState) running(si, ni int) bool {
	s := st.c.services[si]
	return s.topology == "flex" || ni == s.primary
}

// avail returns the availability and the overall status of an instance.
func (st *clusterState) avail(si, ni int) (avail, overall string) {
	s := st.c.services[si]
	is := st.inst[si][ni]
	if !st.running(si, ni) {
		return "down", "down"
	}
	avail, overall = "up", "up"
	for ri, r := range s.resources {
		switch is.res[ri] {
		case "up":
		case "warn":
			overall = "warn"
		default:
			if r.optional {
				overall = "warn"
			} else {
				avail, overall = "down", "down"
			}
		}
	}
	return
}

func (st *clusterState) objectAvail(si int) (avail, overall string, up int) {
	s := st.c.services[si]
	avail, overall = "down", "up"
	for ni := range st.c.nodes {
		a, o := st.avail(si, ni)
		if a == "up" {
			up++
		}
		if st.running(si, ni) && o != "up" {
			overall = "warn"
		}
	}
	switch {
	case up == 0:
		avail, overall = "down", "down"
	case s.topology == "flex" && up < len(st.c.nodes):
		avail = "warn"
	default:
		avail = "up"
	}
	return
}

// event changes the status of a service: mostly an optional resource flapping
// on a node it runs on, sometimes a resource of the service failing or coming
// back, and for a failover service of several nodes, a move to another node.
// It returns the changes as the agent names them: the object path, and
// path@nodename for each instance changed.
func (st *clusterState) event(si int, now time.Time) []string {
	s := st.c.services[si]
	nodes := st.c.nodes
	changed := map[int]bool{}
	draw := st.rand.IntN(100)
	switch {
	case draw < 10 && s.topology == "failover" && len(nodes) > 1:
		from := s.primary
		to := (from + 1 + st.rand.IntN(len(nodes)-1)) % len(nodes)
		for ri := range s.resources {
			st.inst[si][from].res[ri] = "down"
			st.inst[si][to].res[ri] = "up"
		}
		s.primary = to
		st.inst[si][to].startedAt = now
		changed[from], changed[to] = true, true
	default:
		ni := s.primary
		if s.topology == "flex" {
			ni = st.rand.IntN(len(nodes))
		}
		ri := st.rand.IntN(len(s.resources))
		is := st.inst[si][ni]
		switch is.res[ri] {
		case "up":
			if s.resources[ri].optional || draw < 30 {
				is.res[ri] = "warn"
				if !s.resources[ri].optional {
					is.res[ri] = "down"
				}
			}
		default:
			is.res[ri] = "up"
		}
		changed[ni] = true
	}
	changes := []string{s.path}
	for ni := range changed {
		st.inst[si][ni].updatedAt = now
		changes = append(changes, s.path+"@"+nodes[ni].name)
	}
	st.objAt[si] = now
	st.gen++
	return changes
}

func (st *clusterState) allChanges() []string {
	var l []string
	for _, s := range st.c.services {
		l = append(l, s.path)
		for _, n := range st.c.nodes {
			l = append(l, s.path+"@"+n.name)
		}
	}
	return l
}

func (st *clusterState) objectPaths() []string {
	l := []string{"cluster"}
	for _, s := range st.c.services {
		l = append(l, s.path)
	}
	return l
}

func (st *clusterState) nodeNames() []string {
	l := make([]string, len(st.c.nodes))
	for i, n := range st.c.nodes {
		l[i] = n.name
	}
	return l
}

func (st *clusterState) serviceIndex(path string) (int, bool) {
	for i, s := range st.c.services {
		if s.path == path {
			return i, true
		}
	}
	return 0, false
}

func namespaceOf(path string) string {
	ns, _, _ := strings.Cut(path, "/")
	return ns
}

func csum(keys ...any) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprint(keys...))))
}
