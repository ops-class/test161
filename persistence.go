package test161

const (
	MSG_SUBMISSION_CREATE = iota
	MSG_SUBMISSION_SCORE
	MSG_SUBMISSION_STATUS
)

const (
	MSG_TEST_STATUS = iota
	MSG_TEST_SCORE
)

const (
	MSG_COMMAND_STATUS = iota
	MSG_COMMAND_SCORE
	MSG_COMMAND_OUTPUT
)

// Each Submission has at most one PersistenceManager, and it is pinged when a
// variety of events occur.  These callbacks are invoked synchronously, so it's
// up to the PersistenceManager to not slow down the tests. We do this because
// the PersistenceManager can create goroutines if applicable, but we can't
// make an asynchronous call synchronous when it might be needed. So, be kind
// ye PersistenceManagers.
type PersistenceManager interface {
	SubmissionChanged(s *Submission, msg int) error
	TestChanged(t *Test, msg int) error
	CommandChanged(t *Test, c *Command, l *OutputLine) error
	BuildTestChanged(t *BuildTest, msg int) error
	BuildCommandChanged(t *BuildTest, c *BuildCommand, l *OutputLine) error
}

type MongoPersistence struct {
}

func (m *MongoPersistence) SubmissionChanged(s *Submission, msg int) error {
	return nil
}

func (m *MongoPersistence) TestChanged(t *Test, msg int) error {
	return nil
}

func (m *MongoPersistence) CommandChanged(t *Test, c *Command, l *OutputLine) error {
	return nil
}

func (m *MongoPersistence) BuildTestChanged(t *BuildTest, msg int) error {
	return nil
}

func (m *MongoPersistence) BuildCommandChanged(t *BuildTest, c *BuildCommand, l *OutputLine) error {
	return nil
}
