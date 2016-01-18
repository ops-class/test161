package graph

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGraphCycles(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	nodes := []string{"shell", "boot", "badcall2", "randcall", "badcall", "shll", "badcall3"}

	graph := New(nodes)
	assert.Equal(len(nodes), len(graph.NodeMap))
	if len(nodes) == len(graph.NodeMap) {
		for _, s := range nodes {
			assert.NotNil(graph.NodeMap[s])
		}
	}

	graph.AddEdge("shell", "boot")
	graph.AddEdge("badcall", "shell")
	graph.AddEdge("randcall", "shell")
	graph.AddEdge("badcall2", "badcall")
	graph.AddEdge("shll", "boot")
	graph.AddEdge("badcall3", "badcall2")

	sorted, err := graph.TopSort()
	assert.Nil(err)
	t.Log(sorted)

	graph.AddEdge("shell", "badcall3")
	_, err = graph.TopSort()
	assert.NotNil(err)
	t.Log(err)
}

func TestGraphForest(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)

	nodes := []string{"shell", "boot", "badcall2", "randcall", "badcall", "shll", "badcall3", "boot2", "shell2"}

	graph := New(nodes)
	graph.AddEdge("shell", "boot")
	graph.AddEdge("badcall", "shell")
	graph.AddEdge("randcall", "shell")
	graph.AddEdge("badcall2", "badcall")
	graph.AddEdge("shll", "boot")
	graph.AddEdge("badcall3", "badcall2")
	graph.AddEdge("shell2", "boot2")

	sorted, err := graph.TopSort()
	assert.Nil(err)
	t.Log(sorted)

	graph.AddEdge("boot2", "shell2")
	_, err = graph.TopSort()
	assert.NotNil(err)
	t.Log(err)
}
