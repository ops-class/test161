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
	SUBMISSION_BUILDIND  = "building"  // Building the kernel
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
	TargetID        bson.ObjectId `bson:"target_id"`
	TargetName      string        `bson:"target_name"`
	TargetVersion   uint          `bson"target_version"`
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

	env *TestEnvironment
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
func NewSubmission(request *SubmissionRequest, defaultEnv *TestEnvironment) (*Submission, error) {
	if err := request.validate(defaultEnv); err != nil {
		return nil, err
	}

	// TODO: Create build "test"

	//target := env.Targets[request.Target]
	return nil, nil
}
