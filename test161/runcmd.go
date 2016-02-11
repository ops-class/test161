package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/ops-class/test161"
	"os"
	"sort"
)

// 'test161 run' flags
var runCommandVars struct {
	dryRun     bool
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

func doRun() {
	if err := getRunArgs(); err != nil {
		printRunError(err)
		return
	}

	errs := runTests()
	if len(errs) > 0 {
		printRunErrors(errs)
	}

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
	runFlags.BoolVar(&runCommandVars.sequential, "sequential", false, "")
	runFlags.BoolVar(&runCommandVars.sequential, "s", false, "")
	runFlags.BoolVar(&runCommandVars.deps, "dependencies", false, "")
	runFlags.BoolVar(&runCommandVars.deps, "", false, "")
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

func runTestGroup(tg *test161.TestGroup, useDeps bool) {
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

	test161.StartManager()
	done, updates := r.Run()

	exitSync := make(chan int)

	// Print the output lines
	if runCommandVars.verbose == VERBOSE_LOUD {
		go func() {
			for update := range updates {
				if update.Reason == test161.UpdateReasonOutput {
					line := update.Data.(*test161.OutputLine)
					output := fmt.Sprintf("%.6f\t%s", line.SimTime, line.Line)
					fmt.Println(output)
				}
			}
			exitSync <- 1
		}()
	}

	// Collect the results
	complete := make([]*test161.Test161JobResult, 0)
	for res := range done {
		complete = append(complete, res)
	}

	// Wait for output to stop printing
	if runCommandVars.verbose == VERBOSE_LOUD {
		<-exitSync
	}

	test161.StopManager()

	totalPoints := uint(0)
	totalAvail := uint(0)
	totals := []int{0, 0, 0, 0}

	fmt.Println("\nResults:")
	fmt.Println("----------------------------------------------------------------------------")

	for _, res := range complete {

		if res == nil {
			fmt.Println("Res Nil!")
		} else if res.Test == nil {
			fmt.Println("Nil!")
		}

		totalPoints += res.Test.PointsEarned
		totalAvail += res.Test.PointsAvailable

		if runCommandVars.verbose != VERBOSE_WHISPER {
			if res.Test.PointsAvailable > 0 {
				fmt.Printf("Test: %-20s  Result: %-10v  Score: %v/%v\n", res.Test.DependencyID,
					res.Test.Result, res.Test.PointsEarned, res.Test.PointsAvailable)
			} else {
				fmt.Printf("Test: %-20s  Result: %-10v\n", res.Test.DependencyID, res.Test.Result)
			}

			if res.Err != nil {
				fmt.Printf("Error (%v): %v\n", res.Test.DependencyID, res.Err)
			}
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

	fmt.Printf("\nTotal Correct  : %v/%v\n", totals[0], len(tg.Tests))
	fmt.Printf("Total Incorrect: %v/%v\n", totals[1], len(tg.Tests))
	fmt.Printf("Total Aborted  : %v/%v\n", totals[2], len(tg.Tests))
	fmt.Printf("Total Skipped  : %v/%v\n", totals[3], len(tg.Tests))

	if totalAvail > 0 {
		fmt.Printf("\nTotal Score :  %v/%v\n", totalPoints, totalAvail)
	}
}

func runTests() []error {

	var target *test161.Target
	var ok bool

	// Try running as a Target first
	if len(runCommandVars.tests) == 1 && !runCommandVars.isTag {
		if target, ok = env.Targets[runCommandVars.tests[0]]; ok {
			tg, errs := target.Instance(env)
			if len(errs) > 0 {
				return errs
			} else {
				if runCommandVars.dryRun {
					printDryRun(tg)
				} else {
					runTestGroup(tg, true)
				}
				return nil
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

	if tg, errs := test161.GroupFromConfig(config); len(errs) > 0 {
		return errs
	} else {
		if runCommandVars.dryRun {
			printDryRun(tg)
		} else {
			runTestGroup(tg, config.UseDeps)
		}
		return nil
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
}
