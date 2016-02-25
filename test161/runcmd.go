package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/ops-class/test161"
	"os"
	"sort"
	"strings"
)

// 'test161 run' flags
var runCommandVars struct {
	dryRun     bool
	explain    bool
	sequential bool
	deps       bool
	verbose    string
	isTag      bool
	tests      []string
}

func __printRunError(err error) {
	fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
}

func printRunError(err error) {
	__printRunError(err)
	fmt.Println()
}

func printRunErrors(errs []error) {
	if len(errs) > 0 {
		for _, e := range errs {
			__printRunError(e)
		}
		fmt.Println()
	}
}

func doRun() int {
	if err := getRunArgs(); err != nil {
		printRunError(err)
		return 1
	}

	exitcode, errs := runTests()
	if len(errs) > 0 {
		printRunErrors(errs)
	}

	return exitcode
}

// Verbose settings
const (
	VERBOSE_LOUD    = "loud"
	VERBOSE_QUIET   = "quiet"
	VERBOSE_WHISPER = "whisper"
)

func getRunArgs() error {

	runFlags := flag.NewFlagSet("test161 run", flag.ExitOnError)
	runFlags.Usage = usage

	runFlags.BoolVar(&runCommandVars.dryRun, "dry-run", false, "")
	runFlags.BoolVar(&runCommandVars.dryRun, "r", false, "")
	runFlags.BoolVar(&runCommandVars.explain, "explain", false, "")
	runFlags.BoolVar(&runCommandVars.explain, "x", false, "")
	runFlags.BoolVar(&runCommandVars.sequential, "sequential", false, "")
	runFlags.BoolVar(&runCommandVars.sequential, "s", false, "")
	runFlags.BoolVar(&runCommandVars.deps, "dependencies", false, "")
	runFlags.BoolVar(&runCommandVars.deps, "d", false, "")
	runFlags.StringVar(&runCommandVars.verbose, "verbose", "loud", "")
	runFlags.StringVar(&runCommandVars.verbose, "v", "loud", "")
	runFlags.BoolVar(&runCommandVars.isTag, "tag", false, "")

	runFlags.Parse(os.Args[2:]) // this may exit

	runCommandVars.tests = runFlags.Args()

	if len(runCommandVars.tests) == 0 {
		return errors.New("At least one test or target must be specified")
	}

	switch runCommandVars.verbose {
	case VERBOSE_LOUD:
	case VERBOSE_QUIET:
	case VERBOSE_WHISPER:
	default:
		return errors.New("verbose flag must be one of 'loud', 'quiet', or 'whisper'")
	}

	return nil
}

func runTestGroup(tg *test161.TestGroup, useDeps bool) int {
	var r test161.TestRunner
	if useDeps {
		r = test161.NewDependencyRunner(tg)
	} else {
		r = test161.NewSimpleRunner(tg)
	}

	if runCommandVars.sequential {
		test161.SetManagerCapacity(1)
	} else {
		test161.SetManagerCapacity(0)
	}

	// Set up a PersistenceManager that just outputs to the console
	if runCommandVars.verbose == VERBOSE_LOUD {
		// Compute the max witdth for pretty-printing lines
		max := 0
		for _, t := range tg.Tests {
			if max < len(t.DependencyID) {
				max = len(t.DependencyID)
			}
		}
		env.Persistence = &ConsolePersistence{max}
	}

	totalPoints := uint(0)
	totalAvail := uint(0)
	totals := []int{0, 0, 0, 0}

	// For printing
	hasScore := false
	results := make([][]string, 0)

	headers := []*Heading{
		&Heading{
			Text:          "Test",
			MinWidth:      30,
			LeftJustified: true,
		},
		&Heading{
			Text:          "Result",
			MinWidth:      10,
			LeftJustified: true,
		},
		&Heading{
			Text:     "Score",
			MinWidth: 10,
		},
	}

	// Run it
	test161.StartManager()
	done := r.Run()

	for res := range done {
		totalPoints += res.Test.PointsEarned
		totalAvail += res.Test.PointsAvailable

		hasScore = hasScore || res.Test.PointsAvailable > 0

		row := []string{
			res.Test.DependencyID,
			string(res.Test.Result),
			fmt.Sprintf("%v/%v", res.Test.PointsEarned, res.Test.PointsAvailable),
		}

		results = append(results, row)

		if res.Err != nil {
			fmt.Printf("Error (%v): %v\n", res.Test.DependencyID, res.Err)
		}

		switch res.Test.Result {
		case test161.TEST_RESULT_CORRECT:
			totals[0] += 1
		case test161.TEST_RESULT_INCORRECT:
			totals[1] += 1
		case test161.TEST_RESULT_SKIP:
			totals[2] += 1
		case test161.TEST_RESULT_ABORT:
			totals[3] += 1
		}
	}

	test161.StopManager()

	if runCommandVars.verbose != VERBOSE_WHISPER {
		// Chop off the score if it's not a graded target
		if !hasScore {
			headers = headers[0:len(headers)]
			for i, row := range results {
				results[i] = row[0:len(row)]
			}
		}

		fmt.Println()
		printColumns(headers, results, defaultPrintConf)
	}

	// Print totals
	desc := []string{"Total Correct", "Total Incorrect",
		"Total Skipped", "Total Aborted",
	}

	fmt.Println()

	for i := 0; i < len(desc); i++ {
		if i == 0 || totals[i] > 0 {
			fmt.Printf("%-15v: %v/%v\n", desc[i], totals[i], len(tg.Tests))
		}
	}

	if totalAvail > 0 {
		fmt.Printf("\n%-15v: %v/%v\n", "Total Score", totalPoints, totalAvail)
	}

	fmt.Println()

	if len(tg.Tests) == totals[0] {
		return 0
	} else {
		return 1
	}
}

func runTests() (int, []error) {

	var target *test161.Target
	var ok bool

	// Try running as a Target first
	if len(runCommandVars.tests) == 1 && !runCommandVars.isTag {
		if target, ok = env.Targets[runCommandVars.tests[0]]; ok {
			tg, errs := target.Instance(env)
			if len(errs) > 0 {
				return 1, errs
			} else {
				if runCommandVars.explain {
					explain(tg)
				} else if runCommandVars.dryRun {
					printDryRun(tg)
				} else {
					runTestGroup(tg, true)
				}
				return 0, nil
			}
		}
	}

	// Not a target, run as a regular (ungraded) TestGroup
	config := &test161.GroupConfig{
		Name:    "test",
		UseDeps: runCommandVars.deps,
		Tests:   runCommandVars.tests,
		Env:     env,
	}

	exitcode := 0

	if tg, errs := test161.GroupFromConfig(config); len(errs) > 0 {
		return 1, errs
	} else {
		if runCommandVars.explain {
			explain(tg)
		} else if runCommandVars.dryRun {
			printDryRun(tg)
		} else {
			exitcode = runTestGroup(tg, config.UseDeps)
		}
		return exitcode, nil
	}
}

type testsByID []*test161.Test

func (t testsByID) Len() int           { return len(t) }
func (t testsByID) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t testsByID) Less(i, j int) bool { return t[i].DependencyID < t[j].DependencyID }

func printDryRun(tg *test161.TestGroup) {

	deps := make([]*test161.Test, 0)
	tests := make([]*test161.Test, 0)

	for _, test := range tg.Tests {
		if test.IsDependency {
			deps = append(deps, test)
		} else {
			tests = append(tests, test)
		}
	}

	sort.Sort(testsByID(deps))
	sort.Sort(testsByID(tests))

	fmt.Println()

	if len(deps) > 0 {
		for _, test := range deps {
			fmt.Printf("%-30v (dependency)\n", test.DependencyID)
		}
	}

	for _, test := range tests {
		if test.PointsAvailable > 0 {
			fmt.Printf("%-30v (%v points)\n", test.DependencyID, test.PointsAvailable)
		} else {
			fmt.Println(test.DependencyID)
		}
	}

	fmt.Println()
}

func explain(tg *test161.TestGroup) {

	deps := make([]*test161.Test, 0)
	tests := make([]*test161.Test, 0)

	for _, test := range tg.Tests {
		if test.IsDependency {
			deps = append(deps, test)
		} else {
			tests = append(tests, test)
		}
	}

	sort.Sort(testsByID(deps))
	sort.Sort(testsByID(tests))

	fmt.Println()

	if len(deps) > 0 {
		for _, test := range deps {
			fmt.Printf("%-30v (dependency)\n", test.DependencyID)
		}
	}

	for _, test := range tests {
		fmt.Println()

		// Test ID
		fmt.Println(test.DependencyID)
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println("Name        :", test.Name)
		fmt.Println("Description :", test.Description)

		// Points/scoring
		if test.PointsAvailable > 0 {
			fmt.Println("Points      : ", test.PointsAvailable)
			fmt.Println("Scoring     : ", test.ScoringMethod)
		}

		// Dependencies
		if len(test.ExpandedDeps) > 0 {
			sorted := make([]*test161.Test, 0)
			for _, dep := range test.ExpandedDeps {
				sorted = append(sorted, dep)
			}
			sort.Sort(testsByID(sorted))

			fmt.Println("Dependencies:")
			for _, dep := range sorted {
				fmt.Println("    ", dep.DependencyID)
			}
		}

		// Commands
		fmt.Println("Commands:")

		for _, cmd := range test.Commands {
			// Instantiate the command so we get the expected output
			cmd.Instantiate(env)
			fmt.Println("    Cmd Line :", cmd.Input.Line)
			fmt.Println("      Panics :", cmd.Panic)
			fmt.Println("      Points :", cmd.PointsAvailable)
			if len(cmd.ExpectedOutput) > 0 {
				fmt.Println("      Output :")
				for _, output := range cmd.ExpectedOutput {
					fmt.Println("          Text     :", output.Text)
					fmt.Println("          Trusted  :", output.Trusted)
					fmt.Println("          KeyID    :", output.KeyName)
				}
			}
		}
	}

	fmt.Println()
}
