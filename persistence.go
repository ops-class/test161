package test161

const (
	MSG_PERSIST_CREATE   = iota // The object has been created
	MSG_PERSIST_UPDATE          // Generic update message.
	MSG_PERSIST_OUTPUT          // Added an output line (command types only)
	MSG_PERSIST_COMPLETE        // We won't update the object any more
	MSG_TARGET_LOAD             // When a target is loaded
)

// Inidividual field updates
const (
	MSG_FIELD_SCORE = 1 << iota
	MSG_FIELD_STATUS
	MSG_FIELD_TESTS
	MSG_FIELD_OUTPUT
	MSG_FIELD_STATUSES
)

const (
	PERSIST_TYPE_STUDENTS = 1 << iota
)

// Each Submission has at most one PersistenceManager, and it is pinged when a
// variety of events occur.  These callbacks are invoked synchronously, so it's
// up to the PersistenceManager to not slow down the tests. We do this because
// the PersistenceManager can create goroutines if applicable, but we can't
// make an asynchronous call synchronous when it might be needed. So, be kind
// ye PersistenceManagers.
type PersistenceManager interface {
	Close()
	Notify(entity interface{}, msg, what int) error
	CanRetrieve() bool

	// what should be PERSIST_TYPE_*
	// who is a map of field:value
	// res is where to deserialize the data
	Retrieve(what int, who map[string]interface{}, res interface{}) error
}

type DoNothingPersistence struct {
}

func (d *DoNothingPersistence) Close() {
}

func (d *DoNothingPersistence) Notify(entity interface{}, msg, what int) error {
	return nil
}

func (d *DoNothingPersistence) CanRetrieve() bool {
	return false
}

func (d *DoNothingPersistence) Retrieve(what int, who map[string]interface{}, res interface{}) error {
	return nil
}
