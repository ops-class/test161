package graph

import (
	"errors"
)

// Our Graph type consists of only a map of Nodes,
// indexed by strings. The graph's edge data is stored
// in the Nodes themselves, which makes cycle detection
// a bit easier.
type Graph struct {
	NodeMap map[string]*Node
}

// A Node of a directed graph, with incoming and outgoing edges.
type Node struct {
	Name     string
	Value    Keyer
	EdgesOut map[string]*Node
	EdgesIn  map[string]*Node
}

type Keyer interface {
	Key() string
}

type StringNode string

func (s StringNode) Key() string {
	return string(s)
}

// Add an incoming edge.  This needs to be paired with addEdgeOut.
func (n *Node) addEdgeIn(edgeNode *Node) {
	n.EdgesIn[edgeNode.Name] = edgeNode
}

// Add an outgoing edge.  This needs to be paired with addEdgeIn.
func (n *Node) addEdgeOut(edgeNode *Node) {
	n.EdgesOut[edgeNode.Name] = edgeNode
}

// Remove an incoming edge.  This needs to be paired with removeEdgeOut.
func (n *Node) removeEdgeIn(edgeNode *Node) {
	delete(n.EdgesIn, edgeNode.Name)
}

// Remove an outgoing edge.  This needs to be paired with removeEdgeIn.
func (n *Node) removeEdgeOut(edgeNode *Node) {
	delete(n.EdgesOut, edgeNode.Name)
}

// TopSort creates a topological sort of the Nodes of a Graph.
// If there is a cycle, an error is returned, otherwise the
// topological sort is returned as a list of node names.
func (g *Graph) TopSort() ([]string, error) {
	sorted := make([]string, 0)
	copy := g.copy()

	// Initially, add all nodes without dependencies
	empty := make([]*Node, 0)
	for _, node := range copy.NodeMap {
		if len(node.EdgesIn) == 0 {
			empty = append(empty, node)
		}
	}

	for len(empty) > 0 {
		node := empty[0]
		sorted = append(sorted, node.Name)
		empty = empty[1:]
		for _, outgoing := range node.EdgesOut {
			// delete the edge from node -> outgoing
			outgoing.removeEdgeIn(node)
			if len(outgoing.EdgesIn) == 0 {
				empty = append(empty, outgoing)
			}
		}
		node.EdgesOut = nil
	}

	// if there are any edges left, we have a cycle
	for _, n := range copy.NodeMap {
		if len(n.EdgesIn) > 0 || len(n.EdgesOut) > 0 {
			return nil, errors.New("Cycle!")
		}
	}
	return sorted, nil
}

// Copy an existing graph into an independent structure
// (i.e. new nodes/edges are created - pointers aren't copied)
func (g *Graph) copy() *Graph {
	// Copy nodes
	nodes := make([]Keyer, 0, len(g.NodeMap))
	for _, node := range g.NodeMap {
		nodes = append(nodes, node.Value)
	}
	other := New(nodes)

	// Copy edges
	for fromId, node := range g.NodeMap {
		for toId := range node.EdgesOut {
			other.addEdge(other.NodeMap[fromId], other.NodeMap[toId])
		}
	}

	return other
}

// Create a new graph consisting of a set of nodes with no edges.
func New(nodes []Keyer) *Graph {
	g := &Graph{}
	g.NodeMap = make(map[string]*Node)

	for _, s := range nodes {
		g.AddNode(s)
	}
	return g
}

func (g *Graph) addEdge(from *Node, to *Node) {
	from.addEdgeOut(to)
	to.addEdgeIn(from)
}

func (g *Graph) AddNode(node Keyer) {
	g.NodeMap[node.Key()] = &Node{node.Key(), node, make(map[string]*Node), make(map[string]*Node)}
}

// Add an edge from <from> to <to>
func (g *Graph) AddEdge(from Keyer, to Keyer) error {
	if from == to {
		return errors.New("From node cannot be the same as To node")
	}

	fromNode, ok1 := g.NodeMap[from.Key()]
	toNode, ok2 := g.NodeMap[to.Key()]

	if !ok1 {
		return errors.New("from node not found")
	} else if !ok2 {
		return errors.New("to node not found")
	} else {
		g.addEdge(fromNode, toNode)
		return nil
	}
}
