package test161

import (
	"io/ioutil"
	"path/filepath"
	"strings"
)

// Global Environment Cofiguration

type TestEnvironment struct {
	// These do not depend on the TestGroup/Target
	TestDir  string
	Commands map[string]*CommandTemplate
	Targets  map[string]*Target

	manager *manager

	CacheDir    string
	Persistence PersistenceManager

	// These depend on the TestGroup/Target
	KeyMap  map[string]string
	RootDir string
}

// Create a new TestEnvironment by copying the global state
// from an existing environment.  Local test state will
// be initialized to default values.
func (env *TestEnvironment) CopyEnvironment() *TestEnvironment {
	copy := &TestEnvironment{
		TestDir:     env.TestDir,
		Commands:    env.Commands,
		Targets:     env.Targets,
		manager:     env.manager,
		Persistence: env.Persistence,
		KeyMap:      make(map[string]string),
		RootDir:     "",
	}
	return copy
}

func NewEnvironment(testDir, targetDir string) (*TestEnvironment, error) {

	// Initialize the command template cache
	f := filepath.Join(testDir, "commands")
	templates, err := CommandTemplatesFromFile(f)
	if err != nil {
		return nil, err
	}

	env := &TestEnvironment{
		TestDir:  testDir,
		manager:  testManager,
		Commands: make(map[string]*CommandTemplate),
		Targets:  make(map[string]*Target),
		KeyMap:   make(map[string]string),
	}

	for _, templ := range templates.Templates {
		env.Commands[templ.Name] = templ
	}

	if len(targetDir) > 0 {
		// Initialize the targets cache
		dir, err := ioutil.ReadDir(targetDir)
		if err != nil {
			return nil, err
		}

		for _, f := range dir {
			if f.Mode().IsRegular() {
				if strings.HasSuffix(f.Name(), ".target") {
					if t, err := TargetFromFile(filepath.Join(targetDir, f.Name())); err != nil {
						return nil, err
					} else {
						// Only track the most recent version
						prev, ok := env.Targets[t.Name]
						if !ok || t.Version > prev.Version {
							env.Targets[t.Name] = t
						}
					}
				}
			}
		}
	}

	return env, nil
}

func (env *TestEnvironment) TargetList() *TargetList {
	list := &TargetList{}
	list.Targets = make([]*TargetListItem, 0, len(env.Targets))

	for _, t := range env.Targets {
		list.Targets = append(list.Targets, &TargetListItem{
			Name:    t.Name,
			Version: t.Version,
			File:    "",
			Hash:    "",
		})
	}
	return list
}
