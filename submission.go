package test161

import (
	"encoding/json"
	"errors"
	"fmt"
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
	Errors      []string `bson:"errors"`

	SubmissionTime time.Time `bson:"submission_time"`
	CompletionTime time.Time `bson:"completion_time"`

	Env *TestEnvironment `bson:"-"`

	BuildTest *BuildTest `bson:"-"`
	Tests     *TestGroup `bson:"-"`

	students []*Student
}

type TargetResult struct {
	TargetName     string    `bson:"target_name"`
	TargetVersion  uint      `bson:"target_version"`
	TargetType     string    `bson:"target_type"`
	Status         string    `bson:"status"`
	Score          uint      `bson:"score"`
	MaxScore       uint      `bson:"max_score"`
	Performance    float64   `bson:"performance"`
	SubmissionTime time.Time `bson:"submission_time"`
	CompletionTime time.Time `bson:"completion_time"`
	SubmissionID   string    `bson:"submission_id"`
}

type Student struct {
	ID             string                   `bson:"_id"`
	Email          string                   `bson:"email"`
	Token          string                   `bson:"token"`
	LastSubmission *TargetResult            `bson:"last_submission"`
	TargetResults  map[string]*TargetResult `bson:"target_results"`
}

// Keep track of pending submissions.  Keep this out of the database in case there are
// communication issues so that we don't need to manually reset things in the DB.
var userLock = &sync.Mutex{}
var pendingSubmissions = make(map[string]bool)

// Check users against users database.  Don't lock them until we run though
func (req *SubmissionRequest) validateUsers(env *TestEnvironment) ([]*Student, error) {

	allStudents := make([]*Student, 0)

	for _, user := range req.Users {
		request := map[string]interface{}{
			"email": user.Email,
			"token": user.Token,
		}

		fmt.Println("Received:", user.Email, user.Token)

		students := []*Student{}
		if err := env.Persistence.Retrieve(PERSIST_TYPE_STUDENTS, request, &students); err != nil {
			return nil, err
		}

		if len(students) != 1 || students[0].Email != user.Email || students[0].Token != user.Token {
			return nil, errors.New("Unable to authenticate student: " + user.Email)
		}
		allStudents = append(allStudents, students[0])
	}

	return allStudents, nil
}

func (req *SubmissionRequest) validate(env *TestEnvironment) ([]*Student, error) {

	students := []*Student{}
	var err error

	if _, ok := env.Targets[req.Target]; !ok {
		return students, errors.New("Invalid target: " + req.Target)
	}

	if len(req.Users) == 0 {
		return students, errors.New("No usernames specified")
	}

	if env.Persistence != nil && env.Persistence.CanRetrieve() {
		if students, err = req.validateUsers(env); err != nil {
			return students, err
		}
	}

	if len(req.Repository) == 0 || len(req.CommitID) == 0 {
		return students, errors.New("Must specify a Git repository and commit id")
	}

	return students, nil
}

// Create a new Submission that can be evaluated by the test161 server or client.
func NewSubmission(request *SubmissionRequest, env *TestEnvironment) (*Submission, []error) {
	var students []*Student
	var err error

	if students, err = request.validate(env); err != nil {
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
		Errors:      []string{},

		SubmissionTime: time.Now(),

		Env:       env,
		BuildTest: buildTest,
		Tests:     tg,
	}

	s.students = students
	s.Users = make([]string, 0, len(request.Users))
	for _, u := range request.Users {
		s.Users = append(s.Users, u.Email)
	}

	// Try and lock students now so we don't
	userLock.Lock()
	defer userLock.Unlock()

	// First pass - just check
	for _, student := range students {
		if running := pendingSubmissions[student.Email]; running {
			msg := fmt.Sprintf("Cannot submit at this time: User %v has a submission pending.", student.Email)
			env.Log.Println(msg)
			return nil, []error{errors.New(msg)}
		}
	}

	// Now lock
	for _, student := range students {
		pendingSubmissions[student.Email] = true
	}

	if env.Persistence != nil {
		if buildTest != nil {
			// If we get an error here, we can still hopefully recover
			env.notifyAndLogErr("Create Build Test", buildTest, MSG_PERSIST_CREATE, 0)
		}
		// This we can't recover from
		err = env.Persistence.Notify(s, MSG_PERSIST_CREATE, 0)
	}

	// Unlock so they can resubmit
	if err != nil {
		for _, student := range students {
			delete(pendingSubmissions, student.Email)
		}
		return nil, []error{err}
	}

	return s, nil
}

func (s *Submission) persistFailure() {
	// TODO: handle failures
	s.Status = SUBMISSION_ABORTED
}

func (s *Submission) TargetResult() (result *TargetResult) {
	result = &TargetResult{
		TargetName:     s.TargetName,
		TargetVersion:  s.TargetVersion,
		TargetType:     s.TargetType,
		Status:         s.Status,
		Score:          s.Score,
		MaxScore:       s.PointsAvailable,
		Performance:    s.Performance,
		SubmissionTime: s.SubmissionTime,
		CompletionTime: s.CompletionTime,
		SubmissionID:   s.ID,
	}
	return
}

// Update students.  We copy metadata to make this quick and store the
// submision id to look up the full details.
func (s *Submission) updateStudents() {

	res := s.TargetResult()
	for _, student := range s.students {
		// This might be nil coming out of Mongo
		if student.TargetResults == nil {
			student.TargetResults = make(map[string]*TargetResult)
		}

		student.LastSubmission = res
		if s.Status == SUBMISSION_COMPLETED {
			// Update the high score for the target
			if s.TargetType == TARGET_ASST {
				if prev, ok := student.TargetResults[s.TargetName]; !ok || prev.Score < s.Score {
					student.TargetResults[s.TargetName] = res
				}
			} else if s.TargetType == TARGET_PERF && s.Score == s.PointsAvailable {
				if prev, ok := student.TargetResults[s.TargetName]; !ok || prev.Performance < s.Performance {
					student.TargetResults[s.TargetName] = res
				}
			}
		}
		if s.Env.Persistence != nil {
			if err := s.Env.Persistence.Notify(student, MSG_PERSIST_UPDATE, 0); err != nil {
				if sbytes, jerr := json.Marshal(student); jerr != nil {
					s.Env.Log.Printf("Error updating student: %v  (%v)\n", student.Email, err)
				} else {
					s.Env.Log.Printf("Error updating student: %v  (%v)\n", string(sbytes), err)
				}
			}
		}
	}

	userLock.Lock()
	defer userLock.Unlock()

	// Unblock the students from resubmitting
	for _, student := range s.students {
		delete(pendingSubmissions, student.Email)
	}
}

func (s *Submission) finish() {

	s.CompletionTime = time.Now()
	if s.Status == SUBMISSION_RUNNING {
		s.Status = SUBMISSION_COMPLETED
	}

	// Send the final submission update to the db
	s.Env.notifyAndLogErr("Finish Submission", s, MSG_PERSIST_COMPLETE, 0)

	if len(s.students) > 0 {
		s.updateStudents()
	}
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

	defer s.finish()

	// Build os161
	if s.BuildTest != nil {
		s.Status = SUBMISSION_BUILDING
		s.Env.notifyAndLogErr("Submission Status Building", s, MSG_PERSIST_UPDATE, MSG_FIELD_STATUS)

		res, err := s.BuildTest.Run(s.Env)
		if err != nil {
			s.Status = SUBMISSION_ABORTED
			s.Env.notifyAndLogErr("Submission Complete (Aborted)", s, MSG_PERSIST_COMPLETE, 0)
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
		// If this fails, we abort the submission beacase we can't verify the results
		err = s.Env.Persistence.Notify(test, MSG_PERSIST_CREATE, 0)
		if err != nil {
			s.Status = SUBMISSION_ABORTED
			return err
		}
	}

	// Run it
	s.Status = SUBMISSION_RUNNING
	s.Env.notifyAndLogErr("Submission Status (Running) ", s, MSG_PERSIST_UPDATE, MSG_FIELD_TESTS|MSG_FIELD_STATUS)

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
		if r.Err != nil {
			s.Errors = append(s.Errors, fmt.Sprintf("%v", r.Err))
		}
	}

	return err
}
