package test161

import (
	"errors"
	"gopkg.in/mgo.v2/bson"
	"time"
)

// SubmissionRequests are created by clients and used to generate Submissions.
// A SubmissionRequest represents the data required to run a test161 target
// for evaluation by the test161 server.
type SubmissionRequest struct {
	Target     string   // Name of the target
	Users      []string // Email addresses of users
	Repository string   // Git repository to clone
	CommitID   string   // Git commit id to checkout after cloning
}

const (
	SUBMISSION_SUBMITTED = "submitted" // Submitted and queued
	SUBMISSION_BUILDING  = "building"  // Building the kernel
	SUBMISSION_RUNNING   = "running"   // The tests started running
	SUBMISSION_ABORTED   = "aborted"   // Aborted because one or more tests failed to error
	SUBMISSION_COMPLETED = "completed" // Completed
)

type Submission struct {

	// Configuration
	ID         bson.ObjectId `bson:"_id,omitempty"`
	Users      []string      `bson:"users"`
	Repository string        `bson:"repository"`
	CommitID   string        `bson:"commit_id"`

	// Target details
	TargetID        bson.ObjectId `bson:"-"` //TODO: Use this?
	TargetName      string        `bson:"target_name"`
	TargetVersion   uint          `bson:"target_version"`
	PointsAvailable uint          `bson:"max_score"`
	TargetType      string        `bson:"target_type"`

	// Results
	Status      string          `bson:"status"`
	Score       uint            `bson:"score"`
	Performance float64         `bson:"performance"`
	TestIDs     []bson.ObjectId `bson:"tests"`
	Message     string          `bson:"message"`

	SubmissionTime time.Time `bson:"submission_time"`
	CompletionTime time.Time `bson:"completion_time"`

	Env *TestEnvironment `bson:"-"`

	BuildTest *BuildTest `bson:"-"`
	Tests     *TestGroup `bson:"-"`
}

func (req *SubmissionRequest) validate(env *TestEnvironment) error {

	if _, ok := env.Targets[req.Target]; !ok {
		return errors.New("Invalid target: " + req.Target)
	}

	// TODO: Check for closed targets

	if len(req.Users) == 0 {
		return errors.New("No usernames specified")
	}

	// TODO: Check users against users database

	if len(req.Repository) == 0 || len(req.CommitID) == 0 {
		return errors.New("Must specify a Git repository and commit id")
	}

	return nil
}

// Create a new Submission that can be evaluated by the test161 server or client.
func NewSubmission(request *SubmissionRequest, env *TestEnvironment) (*Submission, []error) {
	if err := request.validate(env); err != nil {
		return nil, []error{err}
	}

	// First, get the target because there is some build info there
	target := env.Targets[request.Target]

	conf := &BuildConf{}
	conf.Repo = request.Repository
	conf.CommitID = request.CommitID
	conf.KConfig = target.KConfig
	conf.CacheDir = env.CacheDir
	conf.RequiredCommit = target.RequiredCommit
	conf.RequiresUserland = target.RequiresUserland

	// Add first test (build)
	buildTest, err := conf.ToBuildTest()
	if err != nil {
		return nil, []error{err}
	}

	// Get the TestGroup. The root dir won't be set yet, but that's OK.  We'll
	// change it after the build

	tg, errs := target.Instance(env)
	if err != nil {
		return nil, errs
	}

	s := &Submission{
		ID:              bson.NewObjectId(),
		Users:           request.Users,
		Repository:      request.Repository,
		CommitID:        request.CommitID,
		TargetName:      target.Name,
		TargetVersion:   target.Version,
		PointsAvailable: target.Points,
		TargetType:      target.Type,

		Status:      SUBMISSION_SUBMITTED,
		Score:       uint(0),
		Performance: float64(0.0),
		TestIDs:     []bson.ObjectId{buildTest.ID},
		Message:     "",

		SubmissionTime: time.Now(),

		Env:       env,
		BuildTest: buildTest,
		Tests:     tg,
	}

	if env.Persistence != nil {
		env.Persistence.Notify(s, MSG_PERSIST_CREATE, 0)
	}

	return s, nil
}

func (s *Submission) persistFailure() {
	// TODO: handle failures
	s.Status = SUBMISSION_ABORTED
}

// Synchronous submission runner
func (s *Submission) Run() error {
	// Run the build first.  Right now this is the only thing the front-end sees.
	// We'll add the rest of the tests if this passes, otherwise we don't waste the
	// disk space.

	var err error

	// So we know it's not nil
	if s.Env.Persistence == nil {
		s.Env.Persistence = &DoNothingPersistence{}
	}

	// Build os161
	if s.BuildTest != nil {
		s.Status = SUBMISSION_BUILDING
		err = s.Env.Persistence.Notify(s, MSG_PERSIST_UPDATE, MSG_FIELD_STATUS)
		if err != nil {
			s.persistFailure()
			return err
		}

		var res *BuildResults
		res, err = s.BuildTest.Run()
		if err != nil {
			return err
		}

		// Build output
		s.Env.RootDir = res.RootDir
		s.Env.KeyMap = res.KeyMap
	}

	// Build succeeded, update things accordingly
	for _, test := range s.Tests.Tests {
		// Add test IDs to DB
		s.TestIDs = append(s.TestIDs, test.ID)

		// Create the test object in the DB
		err = s.Env.Persistence.Notify(test, MSG_PERSIST_CREATE, 0)
		if err != nil {
			s.persistFailure()
			return nil
		}
	}

	// Run it
	s.Status = SUBMISSION_RUNNING
	err = s.Env.Persistence.Notify(s, MSG_PERSIST_UPDATE, MSG_FIELD_TESTS|MSG_FIELD_STATUS)
	if err != nil {
		s.persistFailure()
		return nil
	}

	runner := NewDependencyRunner(s.Tests)
	done, _ := runner.Run()

	// Update the score unless a test aborts, then it's 0 and we abort (eventually)
	for r := range done {
		if s.Status == SUBMISSION_RUNNING {
			if r.Test.Result == TEST_RESULT_ABORT {
				s.Status = SUBMISSION_ABORTED
				s.Score = 0
				s.Performance = float64(0)
			} else {
				s.Score += r.Test.PointsEarned
				s.Env.Persistence.Notify(s, MSG_PERSIST_UPDATE, MSG_FIELD_SCORE)
			}
		}
	}

	if s.Status == SUBMISSION_RUNNING {
		s.Status = SUBMISSION_COMPLETED
	}
	s.CompletionTime = time.Now()
	err = s.Env.Persistence.Notify(s, MSG_PERSIST_COMPLETE, 0)

	return err

}
