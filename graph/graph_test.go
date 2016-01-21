package graph

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGraphCycles(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	nodes := []Keyer{
		StringNode("shell"),
		StringNode("boot"),
		StringNode("badcall2"),
		StringNode("randcall"),
		StringNode("badcall"),
		StringNode("shll"),
		StringNode("badcall3"),
	}

	graph := New(nodes)
	assert.Equal(len(nodes), len(graph.NodeMap))
	if len(nodes) == len(graph.NodeMap) {
		for _, s := range nodes {
			var sn StringNode = s.(StringNode)
			key := string(sn)
			assert.NotNil(graph.NodeMap[key])
		}
	}

	graph.AddEdge(StringNode("shell"), StringNode("boot"))
	graph.AddEdge(StringNode("badcall"), StringNode("shell"))
	graph.AddEdge(StringNode("randcall"), StringNode("shell"))
	graph.AddEdge(StringNode("badcall2"), StringNode("badcall"))
	graph.AddEdge(StringNode("shll"), StringNode("boot"))
	graph.AddEdge(StringNode("badcall3"), StringNode("badcall2"))

	sorted, err := graph.TopSort()
	assert.Nil(err)
	t.Log(sorted)

	graph.AddEdge(StringNode("shell"), StringNode("badcall3"))
	_, err = graph.TopSort()
	assert.NotNil(err)
	t.Log(err)
}

func TestGraphForest(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	nodes := []Keyer{
		StringNode("shell"),
		StringNode("boot"),
		StringNode("badcall2"),
		StringNode("randcall"),
		StringNode("badcall"),
		StringNode("shll"),
		StringNode("badcall3"),
		StringNode("boot2"),
		StringNode("shell2"),
	}

	graph := New(nodes)
	graph.AddEdge(StringNode("shell"), StringNode("boot"))
	graph.AddEdge(StringNode("badcall"), StringNode("shell"))
	graph.AddEdge(StringNode("randcall"), StringNode("shell"))
	graph.AddEdge(StringNode("badcall2"), StringNode("badcall"))
	graph.AddEdge(StringNode("shll"), StringNode("boot"))
	graph.AddEdge(StringNode("badcall3"), StringNode("badcall2"))
	graph.AddEdge(StringNode("shell2"), StringNode("boot2"))

	sorted, err := graph.TopSort()
	assert.Nil(err)
	t.Log(sorted)

	graph.AddEdge(StringNode("boot2"), StringNode("shell2"))
	_, err = graph.TopSort()
	assert.NotNil(err)
	t.Log(err)
}
