package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nzhussup/fleck/internal/model"
	"github.com/nzhussup/fleck/internal/resolver"
)

type NodeID string
type NodeKind string
type EdgeKind string
type RelationshipState string

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
	NodeConfigMap     NodeKind = "config-map"
	NodeConfigMapKey  NodeKind = "config-map-key"
	NodeSecret        NodeKind = "secret"
	NodeSecretKey     NodeKind = "secret-key"
	EdgeReads         EdgeKind = "READS"
	EdgeRuns          EdgeKind = "RUNS"
	EdgeSupplies      EdgeKind = "SUPPLIES"
	EdgeReferences    EdgeKind = "REFERENCES"
	EdgeMatches       EdgeKind = "MATCHES"
	EdgeContains      EdgeKind = "CONTAINS"
	EdgeListens       EdgeKind = "LISTENS_ON"
	EdgeDeclaresPort  EdgeKind = "DECLARES_PORT"
	EdgeSelects       EdgeKind = "SELECTS"
	EdgeRoutes        EdgeKind = "ROUTES_TO"

	RelationshipVerified   RelationshipState = "verified"
	RelationshipCandidate  RelationshipState = "candidate"
	RelationshipUnresolved RelationshipState = "unresolved"
	RelationshipAbsent     RelationshipState = "absent"
	RelationshipInvalid    RelationshipState = "invalid"
)

type Node struct {
	ID    NodeID            `json:"id"`
	Kind  NodeKind          `json:"kind"`
	Label string            `json:"label"`
	State RelationshipState `json:"state"`
}
type Edge struct {
	From  NodeID            `json:"from"`
	To    NodeID            `json:"to"`
	Kind  EdgeKind          `json:"kind"`
	State RelationshipState `json:"state"`
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
	g.addNodeState(id, k, label, RelationshipVerified)
}
func (g *Graph) addNodeState(id string, k NodeKind, label string, state RelationshipState) {
	g.Nodes[NodeID(id)] = Node{ID: NodeID(id), Kind: k, Label: label, State: state}
}
func (g *Graph) addEdge(from, to string, k EdgeKind) {
	g.addEdgeState(from, to, k, RelationshipVerified)
}
func (g *Graph) addEdgeState(from, to string, k EdgeKind, state RelationshipState) {
	if _, exists := g.Nodes[NodeID(from)]; !exists {
		return
	}
	if _, exists := g.Nodes[NodeID(to)]; !exists {
		return
	}
	e := Edge{From: NodeID(from), To: NodeID(to), Kind: k, State: state}
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
	containers := append([]model.Container(nil), f.Workload.Containers...)
	selectedPresent := false
	for _, container := range containers {
		if container.ID == f.Container.ID {
			selectedPresent = true
		}
	}
	if f.Container.ID != "" && !selectedPresent {
		containers = append(containers, f.Container)
	}
	for _, container := range containers {
		g.addNode(container.ID, NodeContainer, container.Name)
		if f.Workload.ID != "" {
			g.addEdge(f.Workload.ID, container.ID, EdgeContains)
		}
		for index, port := range container.Ports {
			id := containerPortID(container.ID, index)
			g.addNode(id, NodeContainerPort, fmt.Sprintf("%s:%d/%s", labelOr(port.Name, "port"), port.Port, strings.ToUpper(port.Protocol)))
			g.addEdge(container.ID, id, EdgeDeclaresPort)
		}
	}
	if f.Layers.Application && f.Container.ID != "" {
		g.addEdge(app, f.Container.ID, EdgeRuns)
	}
	for _, r := range f.EnvReads {
		g.addNode(r.ID, NodeEnvRead, labelOr(r.Name, "<dynamic>"))
		if f.Layers.Application {
			g.addEdge(app, r.ID, EdgeReads)
		}
	}
	for _, s := range f.Container.Supplies {
		state := relationshipForSupply(s)
		g.addNodeState(s.ID, NodeEnvSupply, labelOr(s.Name, "<unresolved envFrom>"), state)
		if f.Container.ID != "" {
			g.addEdgeState(f.Container.ID, s.ID, EdgeSupplies, state)
		}
		addSupplyReference(g, s)
	}
	for _, r := range f.EnvReads {
		if !r.Resolved {
			continue
		}
		resolved := resolver.Resolve(f.Container.Supplies, r.Name)
		state := relationshipForResolution(resolved.Status)
		node := g.Nodes[NodeID(r.ID)]
		node.State = state
		g.Nodes[node.ID] = node
		if resolved.Source.ID != "" && resolved.Status != resolver.EnvironmentAbsent {
			g.addEdgeState(r.ID, resolved.Source.ID, EdgeMatches, state)
		}
	}
	for _, p := range f.ListenPorts {
		g.addNode(p.ID, NodeListenPort, portLabel(p.Port, p.EnvName))
		if f.Layers.Application {
			g.addEdge(app, p.ID, EdgeListens)
		}
	}
	for _, s := range f.Services {
		g.addNode(s.ID, NodeService, s.Name)
		if f.Workload.ID != "" {
			g.addEdge(s.ID, f.Workload.ID, EdgeSelects)
		}
		for i, p := range s.Ports {
			id := fmt.Sprintf("serviceport:%s:%d", s.ID, i)
			state := RelationshipVerified
			if p.TargetAmbiguous {
				state = RelationshipUnresolved
			} else if !p.TargetResolved {
				state = RelationshipAbsent
			}
			g.addNodeState(id, NodeServicePort, servicePortLabel(p), state)
			g.addEdge(s.ID, id, EdgeContains)
			if p.TargetResolved && p.TargetContainer != "" {
				for _, container := range containers {
					if container.ID != p.TargetContainer {
						continue
					}
					for index, containerPort := range container.Ports {
						if containerPort.Name == p.TargetPortName && containerPort.Port == p.TargetPort && strings.EqualFold(containerPort.Protocol, p.Protocol) {
							g.addEdge(id, containerPortID(container.ID, index), EdgeRoutes)
						}
					}
				}
			}
			if p.TargetResolved && (p.TargetContainer == "" || p.TargetContainer == f.Container.ID) && strings.EqualFold(p.Protocol, "TCP") {
				for _, listener := range f.ListenPorts {
					if listener.Resolved && listener.Port == p.TargetPort {
						g.addEdge(id, listener.ID, EdgeRoutes)
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
			if a.To != b.To {
				return a.To < b.To
			}
			return a.State < b.State
		})
	}
	for k := range g.Incoming {
		sort.Slice(g.Incoming[k], func(i, j int) bool { return g.Incoming[k][i].From < g.Incoming[k][j].From })
	}
	return g
}

func containerPortID(container string, index int) string {
	return fmt.Sprintf("containerport:%s:%d", container, index)
}

func servicePortLabel(port model.ServicePort) string {
	target := fmt.Sprint(port.TargetPort)
	if port.TargetPortName != "" {
		target = port.TargetPortName
	}
	if !port.TargetResolved && port.TargetPortName == "" {
		target = "<unresolved>"
	}
	return fmt.Sprintf("%d/%s -> %s", port.Port, strings.ToUpper(port.Protocol), target)
}

func relationshipForSupply(supply model.EnvSupply) RelationshipState {
	switch supply.State {
	case model.SupplyDefinite:
		return RelationshipVerified
	case model.SupplyUnverified:
		return RelationshipUnresolved
	case model.SupplyMissingOptional:
		return RelationshipAbsent
	case model.SupplyMissingMandatory:
		return RelationshipInvalid
	default:
		return RelationshipUnresolved
	}
}

func relationshipForResolution(status resolver.EnvironmentStatus) RelationshipState {
	switch status {
	case resolver.EnvironmentResolved:
		return RelationshipVerified
	case resolver.EnvironmentUnverified:
		return RelationshipUnresolved
	case resolver.EnvironmentInvalid:
		return RelationshipInvalid
	default:
		return RelationshipAbsent
	}
}

func addSupplyReference(g *Graph, supply model.EnvSupply) {
	if supply.Reference == "" || (supply.SourceKind != model.SupplyConfigMap && supply.SourceKind != model.SupplySecret) {
		return
	}
	resource, key, hasKey := strings.Cut(supply.Reference, ":")
	resourceKind, keyKind, prefix := NodeConfigMap, NodeConfigMapKey, "configmap"
	if supply.SourceKind == model.SupplySecret {
		resourceKind, keyKind, prefix = NodeSecret, NodeSecretKey, "secret"
	}
	state := relationshipForSupply(supply)
	resourceID := prefix + ":" + resource
	resourceState := RelationshipVerified
	if supply.State == model.SupplyUnverified {
		resourceState = RelationshipUnresolved
	}
	g.addNodeState(resourceID, resourceKind, resource, resourceState)
	if !hasKey {
		g.addEdgeState(supply.ID, resourceID, EdgeReferences, state)
		return
	}
	if supply.State == model.SupplyMissingMandatory || supply.State == model.SupplyMissingOptional {
		g.addEdgeState(supply.ID, resourceID, EdgeReferences, state)
		return
	}
	keyID := prefix + "key:" + resource + ":" + key
	g.addNodeState(keyID, keyKind, key, state)
	g.addEdgeState(resourceID, keyID, EdgeContains, state)
	g.addEdgeState(supply.ID, keyID, EdgeReferences, state)
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
		fmt.Fprintf(&b, "  %q [label=%q, shape=box];\n", id, string(n.Kind)+"\\n"+n.Label+"\\n"+string(n.State))
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
		if edges[i].Kind != edges[j].Kind {
			return edges[i].Kind < edges[j].Kind
		}
		return edges[i].State < edges[j].State
	})
	for _, e := range edges {
		fmt.Fprintf(&b, "  %q -> %q [label=%q];\n", e.From, e.To, string(e.Kind)+" "+string(e.State))
	}
	b.WriteString("}\n")
	return b.String()
}

// EvidencePath returns a deterministic shortest path over verified
// relationships. Traversal is undirected because evidence chains often move
// from a Service through shared ownership to the application side of the
// graph, while edge directions retain their domain meaning in the graph.
func (g *Graph) EvidencePath(from, to NodeID) []NodeID {
	if g == nil || g.Nodes[from].ID == "" || g.Nodes[to].ID == "" {
		return nil
	}
	queue := []NodeID{from}
	previous := map[NodeID]NodeID{}
	seen := map[NodeID]bool{from: true}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == to {
			break
		}
		neighbors := make([]NodeID, 0)
		for _, edge := range g.Outgoing[current] {
			if edge.State == RelationshipVerified {
				neighbors = append(neighbors, edge.To)
			}
		}
		for _, edge := range g.Incoming[current] {
			if edge.State == RelationshipVerified {
				neighbors = append(neighbors, edge.From)
			}
		}
		sort.Slice(neighbors, func(i, j int) bool { return neighbors[i] < neighbors[j] })
		for _, neighbor := range neighbors {
			if seen[neighbor] {
				continue
			}
			seen[neighbor] = true
			previous[neighbor] = current
			queue = append(queue, neighbor)
		}
	}
	if !seen[to] {
		return nil
	}
	path := []NodeID{to}
	for path[len(path)-1] != from {
		path = append(path, previous[path[len(path)-1]])
	}
	for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
		path[left], path[right] = path[right], path[left]
	}
	return path
}

type Queries struct {
	Facts *model.Facts
	Graph *Graph
}

func (q Queries) EnvironmentReads() []model.EnvRead { return q.Facts.EnvReads }
func (q Queries) EffectiveSupplies() []model.EnvSupply {
	resolved := resolver.ResolveAll(q.Facts.Container.Supplies)
	out := make([]model.EnvSupply, 0, len(resolved))
	for _, environment := range resolved {
		if environment.Status == resolver.EnvironmentResolved {
			out = append(out, environment.Source)
		}
	}
	return out
}
func (q Queries) ServicesForWorkload() []model.Service { return q.Facts.Services }
func (q Queries) ResolveEnvironment(name string) resolver.Environment {
	return resolver.Resolve(q.Facts.Container.Supplies, name)
}
