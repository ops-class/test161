package test161

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestVersion(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)

	testVer := ProgramVersion{2, 5, 1}

	vers := []ProgramVersion{
		ProgramVersion{2, 5, 1},
		ProgramVersion{2, 5, 2},
		ProgramVersion{2, 6, 1},
		ProgramVersion{3, 0, 0},
		ProgramVersion{2, 4, 9},
		ProgramVersion{2, 5, 0},
		ProgramVersion{1, 9, 9},
	}
	expected := []int{0, -1, -1, -1, 1, 1, 1}

	for i, version := range vers {
		t.Log(version)
		assert.Equal(expected[i], testVer.CompareTo(version))
	}
}
