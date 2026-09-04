package bootstrap

import (
	"fmt"
	"sort"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/api"
)

// Where the operators actually are.
//
// The loop posts work near people who could take it, which means it first has
// to know where "near people" is. An operator is a point and a range; a
// cluster is a set of operators who could all reach the same ground.

// Cluster is a group of operators who overlap.
type Cluster struct {
	// Key names the cluster stably across cycles: the centre to two decimal
	// places, which is about a kilometre.
	Key          string
	LatE7, LonE7 int64
	// RangeMiles is the smallest range among the members, so a place inside
	// it is reachable by every one of them.
	RangeMiles int
	Workers    []string
}

// Clusters groups positioned, accepting operators who take observe work.
//
// Greedy single pass in a stable order: an operator joins the first cluster
// whose centre they can reach and which can reach them, else starts one. It
// is not optimal and does not need to be — the question is "is there anyone
// here", not "what is the best partition of Detroit".
func Clusters(ops map[string]api.Capacity) []Cluster {
	ids := make([]string, 0, len(ops))
	for id, c := range ops {
		if !c.Positioned() || !c.Accepting || !c.Takes(api.KindObserve) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []Cluster
	for _, id := range ids {
		c := ops[id]
		placed := false
		for i := range out {
			cl := &out[i]
			d := api.MilesBetween(c.LatE7, c.LonE7, cl.LatE7, cl.LonE7)
			if d <= float64(c.RangeMiles) && d <= float64(cl.RangeMiles) {
				// Recentre on the mean of the members.
				n := int64(len(cl.Workers))
				cl.LatE7 = (cl.LatE7*n + c.LatE7) / (n + 1)
				cl.LonE7 = (cl.LonE7*n + c.LonE7) / (n + 1)
				if c.RangeMiles < cl.RangeMiles {
					cl.RangeMiles = c.RangeMiles
				}
				cl.Workers = append(cl.Workers, id)
				placed = true
				break
			}
		}
		if !placed {
			out = append(out, Cluster{
				LatE7: c.LatE7, LonE7: c.LonE7, RangeMiles: c.RangeMiles,
				Workers: []string{id},
			})
		}
	}
	for i := range out {
		out[i].Key = clusterKey(out[i].LatE7, out[i].LonE7)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ClustersFromInterest groups people who said where they would work but have
// not registered a capacity.
//
// Same grouping, different evidence. A registered interest is the same three
// facts a capacity carries — a point, a range, and what somebody will take —
// so it is clustered by exactly the code above rather than a parallel one that
// could drift. The entries are anonymous here on purpose: this decides where
// to post, and it has no business knowing who.
func ClustersFromInterest(in []api.Capacity) []Cluster {
	ops := make(map[string]api.Capacity, len(in))
	for i, c := range in {
		ops[fmt.Sprintf("interest-%06d", i)] = c
	}
	return Clusters(ops)
}

func clusterKey(latE7, lonE7 int64) string {
	return fmt.Sprintf("%.2f,%.2f", api.Deg(latE7), api.Deg(lonE7))
}

// Reaches reports whether a point is inside the cluster's shared range.
func (c Cluster) Reaches(latE7, lonE7 int64) bool {
	return api.InRange(latE7, lonE7, c.LatE7, c.LonE7, c.RangeMiles)
}
