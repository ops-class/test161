package test161

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	yaml "gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"strings"
)

// For simple cases, it is annoying to have to specify the points for the test
// and the command.  So, if the test is made up of only one command, there is
// no reason to specify the command.  So, some rules and convention:
//
// 1) Points must always be specified in the Target (verification)
// 2) Points must always be specified in the Test   (verification)
// 3) Points can be ommitted from the commands, provided that:
//		a) test points - sum(assigned points) % (remaining points) == 0
//		   (i.e. no fractional points per test)

const (
	TARGET_ASST = "asst"
	TARGET_PERF = "perf"
)

const (
	TEST_SCORING_ENTIRE  = "entire"
	TEST_SCORING_PARTIAL = "partial"
)

// Make sure to update isChangeAllowed with any new fields that need to be versioned.
type Target struct {
	ID               string        `yaml:"-" bson:"_id"`
	Name             string        `yaml:"name"`
	PrintName        string        `yaml:"print_name" bson:"print_name"`
	Description      string        `yaml:"description"`
	Active           string        `yaml:"active"`
	Version          uint          `yaml:"version"`
	Type             string        `yaml:"type"`
	Points           uint          `yaml:"points"`
	KConfig          string        `yaml:"kconfig"`
	RequiredCommit   string        `yaml:"required_commit" bson:"required_commit"`
	RequiresUserland bool          `yaml:"userland" bson:"userland"`
	Tests            []*TargetTest `yaml:"tests"`
	FileHash         string        `yaml:"-" bson:"file_hash"`
	FileName         string        `yaml:"-" bson:"file_name"`
}

type TargetTest struct {
	Id       string           `yaml:"id" bson:"test_id"`
	Scoring  string           `yaml:"scoring"`
	Points   uint             `yaml:"points"`
	Commands []*TargetCommand `yaml:"commands"`
}

type TargetCommand struct {
	Id     string   `yaml:"id" bson:cmd_id"` // ID, must match ID in test file
	Index  int      `yaml:"index"`           // Index > 0 => match to index in test
	Points uint     `yaml:"points"`          // Points for this command
	Args   []string `yaml:"args"`            // Argument overrides
}

// TargetListItem is the target detail we send to remote clients about a target
type TargetListItem struct {
	Name        string
	PrintName   string
	Description string
	Active      string
	Type        string
	Version     uint
	Points      uint
	FileName    string
	FileHash    string
	CollabMsg   string
}

// TargetList is the JSON blob sent to clients
type TargetList struct {
	Targets []*TargetListItem
}

func NewTarget() *Target {
	t := &Target{
		Type: TARGET_ASST,
	}
	return t
}

// Ugly, but we need to merge defaults within inner structs
func (t *Target) fixDefaults() {
	for _, test := range t.Tests {
		if test.Scoring != TEST_SCORING_PARTIAL {
			test.Scoring = TEST_SCORING_ENTIRE
		}
	}
	if t.Active != "false" {
		t.Active = "true"
	}
}

func TargetFromFile(file string) (*Target, error) {
	var err error

	var info os.FileInfo
	if info, err = os.Stat(file); err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("Error reading file %v: %v", file, err)
	}

	if t, err := TargetFromString(string(data)); err != nil {
		return t, fmt.Errorf("Error loading target file %v: %v", file, err)
	} else {
		// Save file version and hash
		t.FileName = info.Name()
		raw := md5.Sum(data)
		t.FileHash = strings.ToLower(hex.EncodeToString(raw[:]))
		return t, nil
	}
}

func TargetFromString(text string) (*Target, error) {
	t := NewTarget()
	err := yaml.Unmarshal([]byte(text), t)

	if err != nil {
		return nil, err
	}

	t.fixDefaults()

	return t, nil
}

// Map the target test points onto the runnable test
func (tt *TargetTest) applyTo(test *Test) error {
	test.PointsAvailable = tt.Points
	test.ScoringMethod = tt.Scoring

	// We (may) need to apply arguments and points to each command.
	// For "entire" scoring, all commands must complete successfully
	// in order to gain any (and all) points. In this case, we still
	// need to apply the args, but we that's it. For partial scoring,
	// each command that receives points must be specified.

	// Before we do that, we need to be able to find the commands.
	// Moreover, we need to make sure the input is sane.  We allow
	// a single instance of a command to apply to multiple command
	// instances, and also a per-instance 1-1 mapping. For example,
	// we may have a test that consists of:
	//		/testbin/forktest
	//		/testbin/forktest
	//		/testbin/forktest
	// If /testbin/forktest is specified once (with no index) in the
	// target, then its point mapping applies to all 3 instances.
	// But, one can also specify different point values for each
	// test.  In this case, we require a 1-1 mapping.

	// Store a mapping of id -> list of command instances so we can (1) verify
	// all instances have been accounted for if indexes are specified and (2)
	// find the command instance to apply points and args to.
	type cmdData struct {
		command *Command
		done    bool
	}

	// id -> list of command instances (there could be more than 1)
	commandInstances := make(map[string][]*cmdData)

	for _, cmd := range test.Commands {
		id := cmd.Id()
		if _, ok := commandInstances[id]; !ok {
			commandInstances[id] = make([]*cmdData, 0)
		}
		commandInstances[id] = append(commandInstances[id], &cmdData{cmd, false})
	}

	// If partial scoring, this eventually needs to match the test points
	pointsAssigned := uint(0)

	// First pass - apply the arguments and command points if partial scoring.
	for _, cmd := range tt.Commands {
		instances, ok := commandInstances[cmd.Id]
		if !ok {
			return errors.New("Cannot find command instance: " + cmd.Id)
		}

		// This only applies to a certain index
		if cmd.Index > 0 {
			if cmd.Index > len(instances) {
				return errors.New("Invalid command index for " + cmd.Id)
			}
			instances = []*cmdData{instances[cmd.Index-1]}
		}

		for _, instance := range instances {
			if instance.done {
				return errors.New("Command instance already instantiated. Command: " + cmd.Id)
			} else {
				instance.done = true
				if len(cmd.Args) > 0 {
					instance.command.Input.replaceArgs(cmd.Args)
				}

				if tt.Scoring == TEST_SCORING_PARTIAL {
					instance.command.PointsAvailable = cmd.Points
					pointsAssigned += cmd.Points
				}
			}
		}
	}

	// Next, verify the following:
	//  1) Exactly all the points were assigned
	//	2) If indexes were specified, all instances were covered

	// (1)
	if tt.Scoring == TEST_SCORING_PARTIAL && pointsAssigned != tt.Points {
		return errors.New(fmt.Sprintf("Invalid partial command point assignment: available (%v) != assigned (%v)",
			tt.Points, pointsAssigned))
	}

	// (2) Verify all instances of specified commands are covered
	for _, cmd := range tt.Commands {
		instances := commandInstances[cmd.Id]
		for _, instance := range instances {
			if !instance.done {
				return errors.New("Unassigned command instance: " + cmd.Id)
			}
		}
		// We only need to check a command id once
		delete(commandInstances, cmd.Id)
	}

	return nil
}

// Instance creates a runnable TestGroup from this Target
func (t *Target) Instance(env *TestEnvironment) (*TestGroup, []error) {

	// First, create a group config and convert it to a TestGroup.
	config := &GroupConfig{
		Name:    t.Name,
		UseDeps: true,
		Env:     env,
	}

	config.Tests = make([]string, 0, len(t.Tests))

	for _, tt := range t.Tests {
		config.Tests = append(config.Tests, tt.Id)
	}

	group, errs := GroupFromConfig(config)
	if len(errs) > 0 {
		return nil, errs
	}

	// We have a runnable group with dependencies.  Next, we need
	// to assign points, scoring method, args, etc.
	total := uint(0)
	for _, tt := range t.Tests {
		test, ok := group.Tests[tt.Id]
		if !ok {
			return nil, []error{errors.New("Cannot find " + tt.Id + " in the TestGroup")}
		}
		if err := tt.applyTo(test); err != nil {
			return nil, []error{err}
		}
		total += tt.Points
	}

	if total != t.Points {
		return nil, []error{fmt.Errorf("Target points (%v) do not match sum(test points) (%v)", t.Points, total)}
	}

	return group, nil
}

// Determine whether or not we'll allow the target to replaced in the DB. If we change
// things like the print name, active flag, etc. we should just update it in the DB.
// But, if we chahnge the tests or points, we should be creating a new version.
func (old *Target) isChangeAllowed(other *Target) error {

	if old.Version != other.Version {
		return errors.New("Mismatched target versions. isChangeAllowed only applies to targets with the same version number.")
	}

	// The ID will be different, and that's OK, as long as we update the right one in the DB.
	if old.Name != other.Name {
		return errors.New("Changing the target name requires a version change")
	}
	if old.Type != other.Type {
		return errors.New("Changing the target type requires a version change")
	}
	if old.Points != other.Points {
		return errors.New("Changing the target points requires a version change")
	}

	// TODO: Relying on no duplicate tests

	// We do care about the tests, just not the order
	if len(old.Tests) != len(other.Tests) {
		return errors.New("Changing the number of tests requiers a version change")
	}

	oldMap := make(map[string]*TargetTest)

	for _, t := range old.Tests {
		oldMap[t.Id] = t
	}

	for _, t := range other.Tests {
		if oldVer, ok := oldMap[t.Id]; !ok {
			return fmt.Errorf("Test %v was removed from the new target, which requires a version change", t.Id)
		} else if oldVer.Points != t.Points {
			return fmt.Errorf("The point distribution for %v changed from the new target, which requires a version change", t.Id)
		} else if oldVer.Scoring != t.Scoring {
			return fmt.Errorf("The scoring method for %v changed from the new target, which requires a version change", t.Id)
		}
	}

	// Fields we don't care about:
	//
	// PrintName, Description, Active, RequiredCommit
	// KConfig is set based on the Name
	// RequiresUserland: if this was broken, tests would have failed
	// FileHash: this will change
	// FileName: OK if if moves

	return nil
}
