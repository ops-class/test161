package test161

import (
	"errors"
	"github.com/satori/go.uuid"
	"os"
	"sync"
	"time"
)

type SubmissionUserInfo struct {
	Email string `yaml:"email"`
	Token string `yaml:"token"`
}

// SubmissionRequests are created by clients and used to generate Submissions.
// A SubmissionRequest represents the data required to run a test161 target
// for evaluation by the test161 server.
type SubmissionRequest struct {
	Target     string                // Name of the target
	Users      []*SubmissionUserInfo // Email addresses of users
	Repository string                // Git repository to clone
	CommitID   string                // Git commit id to checkout after cloning
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
	ID         string   `bson:"_id,omitempty"`
	Users      []string `bson:"users"`
	Repository string   `bson:"repository"`
	CommitID   string   `bson:"commit_id"`

	// Target details
	TargetID        string `bson:"-"` //TODO: Use this?
	TargetName      string `bson:"target_name"`
	TargetVersion   uint   `bson:"target_version"`
	PointsAvailable uint   `bson:"max_score"`
	TargetType      string `bson:"target_type"`

	// Results
	Status      string   `bson:"status"`
	Score       uint     `bson:"score"`
	Performance float64  `bson:"performance"`
	TestIDs     []string `bson:"tests"`
	Message     string   `bson:"message"`

	SubmissionTime time.Time `bson:"submission_time"`
	CompletionTime time.Time `bson:"completion_time"`

	Env *TestEnvironment `bson:"-"`

	BuildTest *BuildTest `bson:"-"`
	Tests     *TestGroup `bson:"-"`
}

type TargetResult struct {
	TargetName   string    `bson:"target_name"`
	Status       string    `bson:"status"`
	Score        uint      `bson:"score"`
	MaxScore     uint      `bson:"max_score"`
	Performance  float64   `bson:"performance"`
	Started      time.Time `bson:"started"`
	Completed    time.Time `bson:"completed"`
	SubmissionID string    `bson:"submission_id"`
}

type Student struct {
	ID             string          `bson:"_id"`
	Email          string          `bson:"email"`
	Token          string          `bson:"token"`
	LastSubmission *TargetResult   `bson:"last_submission"`
	TargetResults  []*TargetResult `bson:"target_results"`
}

// Keep track of pending submissions.  Keep this out of the database in case there are
// communication issues so that we don't need to manually reset things in the DB.
var userLock = &sync.Mutex{}
var pendingSubmissions = make(map[string]bool)

// Check users against users database and lock their user record.
func (req *SubmissionRequest) validateAndLockUsers(env *TestEnvironment) ([]*Student, error) {

	userLock.Lock()
	defer userLock.Unlock()

	allStudents := make([]*Student, 0)

	for _, user := range req.Users {
		request := map[string]interface{}{
			"email": user.Email,
			"token": user.Token,
		}

		students := []*Student{}
		if err := env.Persistence.Retrieve(PERSIST_TYPE_STUDENTS, request, &students); err != nil {
			return nil, err
		}

		if len(students) != 1 || students[0].Email != user.Email || students[0].Token != user.Token {
			return nil, errors.New("Unable to authenticate student: " + user.Email)
		} else if pending, _ := pendingSubmissions[students[0].ID]; pending {
			return nil, errors.New("A pending submission already exists for " + students[0].Email)
		}
		allStudents = append(allStudents, students[0])
	}

	// Mark everyone as having a pending submission
	//for _, student := range allStudents {
	//	pendingSubmissions[student].ID = true
	//}
	return allStudents, nil
}

func (req *SubmissionRequest) validate(env *TestEnvironment) error {

	if _, ok := env.Targets[req.Target]; !ok {
		return errors.New("Invalid target: " + req.Target)
	}

	if len(req.Users) == 0 {
		return errors.New("No usernames specified")
	}

	if env.Persistence != nil && env.Persistence.CanRetrieve() {
		if _, err := req.validateAndLockUsers(env); err != nil {
			return err
		}
	}

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
		ID:              uuid.NewV4().String(),
		Repository:      request.Repository,
		CommitID:        request.CommitID,
		TargetName:      target.Name,
		TargetVersion:   target.Version,
		PointsAvailable: target.Points,
		TargetType:      target.Type,

		Status:      SUBMISSION_SUBMITTED,
		Score:       uint(0),
		Performance: float64(0.0),
		TestIDs:     []string{buildTest.ID},
		Message:     "",

		SubmissionTime: time.Now(),

		Env:       env,
		BuildTest: buildTest,
		Tests:     tg,
	}

	s.Users = make([]string, 0, len(request.Users))
	for _, u := range request.Users {
		s.Users = append(s.Users, u.Email)
	}

	if env.Persistence != nil {
		env.Persistence.Notify(buildTest, MSG_PERSIST_CREATE, 0)
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
		res, err = s.BuildTest.Run(s.Env)
		if err != nil {
			s.Status = SUBMISSION_ABORTED
			s.Env.Persistence.Notify(s, MSG_PERSIST_COMPLETE, 0)
			return err
		}

		// Build output
		s.Env.RootDir = res.RootDir
		s.Env.KeyMap = res.KeyMap

		// Clean up temp build directory
		if len(res.TempDir) > 0 {
			defer os.RemoveAll(res.TempDir)
		}
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
