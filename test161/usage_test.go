package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestUploadChunks(t *testing.T) {
	assert := assert.New(t)
	sizes := []*fileSizeInfo{
		&fileSizeInfo{
			name: "foo",
			size: 1024,
		},
		&fileSizeInfo{
			name: "bar",
			size: 1024,
		},
		&fileSizeInfo{
			name: "food",
			size: 2048,
		},
		&fileSizeInfo{
			name: "bars",
			size: 1,
		},
	}

	files, pos := nextUploadChunk(sizes, 0, 2048)
	assert.Equal(2, pos)
	assert.Equal(2, len(files))
	assert.Equal("foo", files[0])
	assert.Equal("bar", files[1])

	files, pos = nextUploadChunk(sizes, pos, 2048)
	assert.Equal(3, pos)
	assert.Equal(1, len(files))
	assert.Equal("food", files[0])

	files, pos = nextUploadChunk(sizes, pos, 2048)
	assert.Equal(4, pos)
	assert.Equal(1, len(files))
	assert.Equal("bars", files[0])

	files, pos = nextUploadChunk(sizes, 0, 512)
	assert.Equal(0, len(files))
	assert.Equal(0, pos)
}

func TestGetUsageFiles(t *testing.T) {
	assert := assert.New(t)

	USAGE_DIR = "./fixtures/files/"

	assert.Equal("fixtures/files/usage.json.gz", getCurUsageFilename())

	files, err := getAllUsageFiles()
	assert.Nil(err)
	assert.Equal(2, len(files))
	if len(files) != 2 {
		t.FailNow()
	}

	sizes, err := getFileSizes(files)
	assert.Nil(err)
	assert.Equal(2, len(sizes))

	sizeMap := make(map[string]int64)
	for _, fi := range sizes {
		sizeMap[fi.name] = fi.size
	}

	sz, ok := sizeMap["fixtures/files/usage_013999999999.json.gz"]
	assert.True(ok)
	assert.Equal(int64(490), sz)

	sz, ok = sizeMap["fixtures/files/usage_014999999999.json.gz"]
	assert.True(ok)
	assert.Equal(int64(490), sz)
}

func TestFileLock(t *testing.T) {
	assert := assert.New(t)

	lock_file := "fixtures/files/test161.lock"
	err := lockFile(lock_file, true)
	assert.Nil(err)

	err = lockFile(lock_file, true)
	assert.NotNil(err)
	t.Log(err)

	err = unlockFile(lock_file)
	assert.Nil(err)
}
