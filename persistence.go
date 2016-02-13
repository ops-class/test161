package test161

const (
	MSG_PERSIST_CREATE     = iota // The object has been created
	MSG_PERSIST_UPDATE            // Generic update message.
	MSG_PERSIST_CMD_UPDATE        //
	MSG_PERSIST_OUTPUT            // Added an output line (command types only)
	MSG_PERSIST_COMPLETE          // We won't update the object any more
)

// Inidividual field updates
const (
	MSG_FIELD_SCORE = 1 << iota
	MSG_FIELD_STATUS
	MSG_FIELD_TESTS
	MSG_FIELD_OUTPUT
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
}

type DoNothingPersistence struct {
}

func (d *DoNothingPersistence) Close() {
}

func (d *DoNothingPersistence) Notify(entity interface{}, msg, what int) error {
	return nil
}
