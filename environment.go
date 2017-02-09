package test161

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// TestEnvironment encapsultes the environment tests runs in. Much of the
// environment is global - commands, targets, etc. However, some state
// is local, such as the secure keyMap and OS/161 root directory.
type TestEnvironment struct {
	// These do not depend on the TestGroup/Target
	TestDir  string
	Commands map[string]*CommandTemplate
	Targets  map[string]*Target

	// Optional - added in version 1.2.6
	Tags map[string]*TagDescription

	manager *manager

	CacheDir    string
	OverlayRoot string
	KeyDir      string
	Persistence PersistenceManager

	Log *log.Logger

	// These depend on the TestGroup/Target
	keyMap  map[string]string
	RootDir string
}

// Create a new TestEnvironment by copying the global state from an existing
// environment.  Local test state will be initialized to default values.
func (env *TestEnvironment) CopyEnvironment() *TestEnvironment {

	// Global
	copy := *env

	// Local
	copy.keyMap = make(map[string]string)
	copy.RootDir = ""

	return &copy
}

// Handle a single commands file (.tc) and load it into the TestEnvironment.
func envCommandHandler(env *TestEnvironment, f string) error {
	if templates, err := CommandTemplatesFromFile(f); err != nil {
		return err
	} else {
		// If we already know about the command, it's an error
		for _, templ := range templates.Templates {
			if _, ok := env.Commands[templ.Name]; ok {
				return fmt.Errorf("Duplicate command (%v) in file %v", templ.Name, f)
			}
			env.Commands[templ.Name] = templ
		}
		return nil
	}
}

// Handle a single targets file (.tt) and load it into the TestEnvironment.
func envTargetHandler(env *TestEnvironment, f string) error {
	if t, err := TargetFromFile(f); err != nil {
		return err
	} else {
		// Only track the most recent version, and only track active targets.
		if t.Active == "true" {
			prev, ok := env.Targets[t.Name]
			if !ok || t.Version > prev.Version {
				env.Targets[t.Name] = t
			}
		}

		if env.Persistence != nil {
			return env.Persistence.Notify(t, MSG_TARGET_LOAD, 0)
		} else {
			return nil
		}
	}
}

// Handle a single tag description file (.td) and load it into the
// TestEnvironment.
func envTagDescHandler(env *TestEnvironment, f string) error {
	if tags, err := TagDescriptionsFromFile(f); err != nil {
		return err
	} else {
		// If we already know about the tag, it's an error
		for _, tag := range tags.Tags {
			if _, ok := env.Tags[tag.Name]; ok {
				return fmt.Errorf("Duplicate tag (%v) in file %v", tag.Name, f)
			}
			env.Tags[tag.Name] = tag
		}
		return nil
	}
}

// envReadLoop searches a directory for files with a certain extention. When it
// finds one, it calls handler().
func (env *TestEnvironment) envReadLoop(searchDir, ext string,
	handler func(env *TestEnvironment, f string) error) error {

	dir, err := ioutil.ReadDir(searchDir)
	if err != nil {
		return err
	}

	for _, f := range dir {
		if f.Mode().IsRegular() {
			if strings.HasSuffix(f.Name(), ext) {
				if err := handler(env, filepath.Join(searchDir, f.Name())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Create a new TestEnvironment from the given test161 directory.  The directory
// must contain these subdirectories: commands/ targets/ tests/
// In addition to loading tests, commands, and targets, a logger is set up that
// writes to os.Stderr.  This can be changed by changing env.Log.
func NewEnvironment(test161Dir string, pm PersistenceManager) (*TestEnvironment, error) {

	cmdDir := path.Join(test161Dir, "commands")
	testDir := path.Join(test161Dir, "tests")
	targetDir := path.Join(test161Dir, "targets")
	tagDir := path.Join(test161Dir, "tags")

	env := &TestEnvironment{
		TestDir:     testDir,
		manager:     testManager,
		Commands:    make(map[string]*CommandTemplate),
		Targets:     make(map[string]*Target),
		Tags:        make(map[string]*TagDescription),
		keyMap:      make(map[string]string),
		Log:         log.New(os.Stderr, "test161: ", log.Ldate|log.Ltime|log.Lshortfile),
		Persistence: pm,
	}

	resChan := make(chan error)

	go func() {
		resChan <- env.envReadLoop(targetDir, ".tt", envTargetHandler)
	}()

	go func() {
		resChan <- env.envReadLoop(cmdDir, ".tc", envCommandHandler)
	}()

	// Tags are optional
	numExpected := 2
	if _, err := os.Stat(tagDir); err == nil {
		numExpected += 1
		go func() {
			resChan <- env.envReadLoop(tagDir, ".td", envTagDescHandler)
		}()
	}

	// Get the results
	var err error = nil
	for i := 0; i < numExpected; i++ {
		// Let the other finish, but just return one error
		temp := <-resChan
		if err == nil {
			err = temp
		}
	}

	if err == nil {
		err = env.linkMetaTargets()
	}

	return env, err
}

func (env *TestEnvironment) linkMetaTargets() error {
	// First, if the target is a subtarget, link it to its metatarget
	// and sibling subtargets.
	for _, target := range env.Targets {
		if len(target.MetaName) > 0 {
			if err := target.initAsSubTarget(env); err != nil {
				return err
			}
		}
	}

	// Next, validate the metatargets
	for _, target := range env.Targets {
		if target.IsMetaTarget {
			if err := target.initAsMetaTarget(env); err != nil {
				return err
			}
		}
	}

	return nil
}

func (env *TestEnvironment) TargetList() *TargetList {
	list := &TargetList{}
	list.Targets = make([]*TargetListItem, 0, len(env.Targets))

	for _, t := range env.Targets {
		list.Targets = append(list.Targets, &TargetListItem{
			Name:        t.Name,
			Version:     t.Version,
			PrintName:   t.PrintName,
			Description: t.Description,
			Active:      t.Active,
			Points:      t.Points,
			Type:        t.Type,
			FileName:    t.FileName,
			FileHash:    t.FileHash,
			CollabMsg:   collabMsgs[t.Name],
		})
	}
	return list
}

// Helper function for logging persistence errors
func (env *TestEnvironment) notifyAndLogErr(desc string, entity interface{}, msg, what int) {
	if env.Persistence != nil {
		err := env.Persistence.Notify(entity, msg, what)
		if err != nil {
			if env.Log != nil {
				env.Log.Printf("(%v) Error writing data: %v\n", desc, err)
			}
		}
	}
}

func (env *TestEnvironment) SetNullLogger() {
	env.Log.SetFlags(0)
	env.Log.SetOutput(ioutil.Discard)
}
