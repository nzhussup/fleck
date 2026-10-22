package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nzhussup/fleck/internal/model"
)

type NodeID string
type NodeKind string
type EdgeKind string

const (
	NodeApplication   NodeKind = "application"
	NodeContainer     NodeKind = "container"
	NodeWorkload      NodeKind = "workload"
	NodeEnvRead       NodeKind = "environment-read"
	NodeEnvSupply     NodeKind = "environment-supply"
	NodeListenPort    NodeKind = "listening-port"
	NodeContainerPort NodeKind = "container-port"
	NodeService       NodeKind = "service"
	NodeServicePort   NodeKind = "service-port"
	EdgeReads         EdgeKind = "READS"
	EdgeRuns          EdgeKind = "RUNS"
	EdgeSupplies      EdgeKind = "SUPPLIES"
	EdgeReferences    EdgeKind = "REFERENCES"
	EdgeListens       EdgeKind = "LISTENS_ON"
	EdgeDeclaresPort  EdgeKind = "DECLARES_PORT"
	EdgeSelects       EdgeKind = "SELECTS"
	EdgeRoutes        EdgeKind = "ROUTES_TO"
)

type Node struct {
	ID    NodeID   `json:"id"`
	Kind  NodeKind `json:"kind"`
	Label string   `json:"label"`
}
type Edge struct {
	From NodeID   `json:"from"`
	To   NodeID   `json:"to"`
	Kind EdgeKind `json:"kind"`
}
type Graph struct {
	Nodes    map[NodeID]Node   `json:"nodes"`
	Outgoing map[NodeID][]Edge `json:"outgoing"`
	Incoming map[NodeID][]Edge `json:"incoming"`
}

func New() *Graph {
	return &Graph{Nodes: map[NodeID]Node{}, Outgoing: map[NodeID][]Edge{}, Incoming: map[NodeID][]Edge{}}
}
func (g *Graph) addNode(id string, k NodeKind, label string) {
	g.Nodes[NodeID(id)] = Node{ID: NodeID(id), Kind: k, Label: label}
}
func (g *Graph) addEdge(from, to string, k EdgeKind) {
	e := Edge{From: NodeID(from), To: NodeID(to), Kind: k}
	g.Outgoing[e.From] = append(g.Outgoing[e.From], e)
	g.Incoming[e.To] = append(g.Incoming[e.To], e)
}

func Build(f *model.Facts) *Graph {
	g := New()
	app := "application:" + f.ApplicationID
	if f.Layers.Application {
		g.addNode(app, NodeApplication, f.ApplicationID)
	}
	if f.Layers.Deployment {
		g.addNode(f.Workload.ID, NodeWorkload, f.Workload.Kind+" "+f.Workload.Name)
	}
	if f.Container.ID != "" {
		g.addNode(f.Container.ID, NodeContainer, f.Container.Name)
		if f.Layers.Application {
			g.addEdge(app, f.Container.ID, EdgeRuns)
		}
	}
	for _, r := range f.EnvReads {
		g.addNode(r.ID, NodeEnvRead, labelOr(r.Name, "<dynamic>"))
		if f.Layers.Application {
			g.addEdge(app, r.ID, EdgeReads)
		}
	}
	for _, s := range f.Container.Supplies {
		g.addNode(s.ID, NodeEnvSupply, labelOr(s.Name, "<unresolved envFrom>"))
		if f.Container.ID != "" {
			g.addEdge(f.Container.ID, s.ID, EdgeSupplies)
		}
		if s.State == model.SupplyDefinite {
			for _, r := range f.EnvReads {
				if r.Resolved && r.Name == s.Name {
					g.addEdge(r.ID, s.ID, EdgeReferences)
				}
			}
		}
	}
	for _, p := range f.ListenPorts {
		g.addNode(p.ID, NodeListenPort, portLabel(p.Port, p.EnvName))
		if f.Layers.Application {
			g.addEdge(app, p.ID, EdgeListens)
		}
	}
	for i, p := range f.Container.Ports {
		id := fmt.Sprintf("containerport:%s:%d", f.Container.ID, i)
		g.addNode(id, NodeContainerPort, fmt.Sprintf("%s:%d", labelOr(p.Name, "port"), p.Port))
		g.addEdge(f.Container.ID, id, EdgeDeclaresPort)
	}
	for _, s := range f.Services {
		g.addNode(s.ID, NodeService, s.Name)
		if f.Workload.ID != "" {
			g.addEdge(s.ID, f.Workload.ID, EdgeSelects)
		}
		for i, p := range s.Ports {
			id := fmt.Sprintf("serviceport:%s:%d", s.ID, i)
			g.addNode(id, NodeServicePort, fmt.Sprintf("%d -> %d", p.Port, p.TargetPort))
			if p.TargetResolved {
				for j, cp := range f.Container.Ports {
					if cp.Port == p.TargetPort {
						g.addEdge(id, fmt.Sprintf("containerport:%s:%d", f.Container.ID, j), EdgeRoutes)
					}
				}
			}
		}
	}
	for k := range g.Outgoing {
		sort.Slice(g.Outgoing[k], func(i, j int) bool {
			a, b := g.Outgoing[k][i], g.Outgoing[k][j]
			if a.Kind != b.Kind {
				return a.Kind < b.Kind
			}
			return a.To < b.To
		})
	}
	for k := range g.Incoming {
		sort.Slice(g.Incoming[k], func(i, j int) bool { return g.Incoming[k][i].From < g.Incoming[k][j].From })
	}
	return g
}
func labelOr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
func portLabel(p int, e string) string {
	if p > 0 {
		return fmt.Sprint(p)
	}
	if e != "" {
		return "$" + e
	}
	return "<unresolved>"
}

func (g *Graph) DOT() string {
	var b strings.Builder
	b.WriteString("digraph fleck {\n  rankdir=LR;\n")
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := g.Nodes[NodeID(id)]
		fmt.Fprintf(&b, "  %q [label=%q, shape=box];\n", id, string(n.Kind)+"\\n"+n.Label)
	}
	var edges []Edge
	for _, es := range g.Outgoing {
		edges = append(edges, es...)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Kind < edges[j].Kind
	})
	for _, e := range edges {
		fmt.Fprintf(&b, "  %q -> %q [label=%q];\n", e.From, e.To, e.Kind)
	}
	b.WriteString("}\n")
	return b.String()
}

type Queries struct {
	Facts *model.Facts
	Graph *Graph
}

func (q Queries) EnvironmentReads() []model.EnvRead    { return q.Facts.EnvReads }
func (q Queries) EffectiveSupplies() []model.EnvSupply { return q.Facts.Container.Supplies }
func (q Queries) ServicesForWorkload() []model.Service { return q.Facts.Services }
func (q Queries) FindSupply(name string) (model.EnvSupply, bool) {
	for i := len(q.Facts.Container.Supplies) - 1; i >= 0; i-- {
		s := q.Facts.Container.Supplies[i]
		if s.Name == name && s.State == model.SupplyDefinite {
			return s, true
		}
	}
	return model.EnvSupply{}, false
}
func (q Queries) HasUnverifiedWildcard() bool {
	for _, s := range q.Facts.Container.Supplies {
		if s.Name == "" && s.State == model.SupplyUnverified {
			return true
		}
	}
	return false
}
