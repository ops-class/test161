package test161

import (
	"errors"
	"fmt"
	"github.com/satori/go.uuid"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type MongoPersistence struct {
	session *mgo.Session
	dbName  string
}

const (
	COLLECTION_SUBMISSIONS = "submissions"
	COLLECTION_TESTS       = "tests"
	COLLECTION_STUDENTS    = "students"
	COLLECTION_TARGETS     = "targets"
	COLLECTION_USERS       = "users"
	COLLECTION_USAGE       = "usage"
)

func NewMongoPersistence(dial *mgo.DialInfo) (PersistenceManager, error) {
	var err error

	m := &MongoPersistence{}

	if m.session, err = mgo.DialWithInfo(dial); err != nil {
		return nil, fmt.Errorf("Mongo Create Session: %s\n", err)
	}
	m.dbName = dial.Database

	return m, nil
}

func (m *MongoPersistence) insertDocument(s *mgo.Session, collection string, data interface{}) error {
	c := s.DB(m.dbName).C(collection)
	err := c.Insert(data)
	return err
}

func (m *MongoPersistence) updateDocumentByID(s *mgo.Session, collection string, id, data interface{}) error {
	c := s.DB(m.dbName).C(collection)
	err := c.UpdateId(id, data)
	return err
}

func (m *MongoPersistence) updateDocument(s *mgo.Session, collection string, selector, data interface{}) error {
	c := s.DB(m.dbName).C(collection)
	err := c.Update(selector, data)
	return err
}

// Update if it exists, otherwise insert
func (m *MongoPersistence) upsertDocument(s *mgo.Session, collection string, selector, data interface{}) error {
	c := s.DB(m.dbName).C(collection)
	_, err := c.Upsert(selector, data)
	return err
}

func (m *MongoPersistence) Close() {
	m.session.Close()
}

func getTestUpdateMap(test *Test, what int) bson.M {
	changes := bson.M{}

	if what&MSG_FIELD_SCORE == MSG_FIELD_SCORE {
		changes["points_earned"] = test.PointsEarned
	}
	if what&MSG_FIELD_STATUS == MSG_FIELD_STATUS {
		changes["result"] = test.Result
	}

	return changes
}

func (m *MongoPersistence) Notify(t interface{}, msg, what int) (err error) {

	session := m.session.Copy()
	defer session.Close()

	switch t.(type) {
	default:
		{
			err = fmt.Errorf("Unexpected type in Notify(): %T", t)
		}
	case *Test:
		{
			test := t.(*Test)
			switch msg {
			case MSG_PERSIST_CREATE:
				err = m.insertDocument(session, COLLECTION_TESTS, test)
			case MSG_PERSIST_COMPLETE:
				err = m.updateDocumentByID(session, COLLECTION_TESTS, test.ID, test)
			case MSG_PERSIST_UPDATE:
				changes := bson.M{}
				if what&MSG_FIELD_SCORE == MSG_FIELD_SCORE {
					changes["points_earned"] = test.PointsEarned
				}

				if what&MSG_FIELD_STATUS == MSG_FIELD_STATUS {
					changes["result"] = test.Result
				}

				if len(changes) > 0 {
					err = m.updateDocumentByID(session, COLLECTION_TESTS, test.ID, bson.M{"$set": changes})
				}
			}
		}
	case *Command:
		{
			cmd := t.(*Command)
			switch msg {
			case MSG_PERSIST_UPDATE:
				selector := bson.M{
					"_id":          cmd.Test.ID,
					"commands._id": cmd.ID,
				}
				changes := bson.M{}

				if what&MSG_FIELD_OUTPUT == MSG_FIELD_OUTPUT {
					changes["commands.$.output"] = cmd.Output
				}

				if what&MSG_FIELD_SCORE == MSG_FIELD_SCORE {
					changes["commands.$.points_earned"] = cmd.PointsEarned
				}

				if what&MSG_FIELD_STATUS == MSG_FIELD_STATUS {
					changes["commands.$.status"] = cmd.Status
				}

				err = m.updateDocument(session, COLLECTION_TESTS, selector, bson.M{"$set": changes})

			}
		}
	case *Submission:
		{
			submission := t.(*Submission)
			switch msg {
			case MSG_PERSIST_CREATE:
				err = m.insertDocument(session, COLLECTION_SUBMISSIONS, submission)
			case MSG_PERSIST_COMPLETE:
				fallthrough
			case MSG_PERSIST_UPDATE:
				err = m.updateDocumentByID(session, COLLECTION_SUBMISSIONS, submission.ID, submission)
			}
		}
	case *BuildTest:
		{
			test := t.(*BuildTest)
			switch msg {
			case MSG_PERSIST_CREATE:
				err = m.insertDocument(session, COLLECTION_TESTS, test)
			case MSG_PERSIST_COMPLETE:
				err = m.updateDocumentByID(session, COLLECTION_TESTS, test.ID, test)
			case MSG_PERSIST_UPDATE:
				changes := bson.M{}
				if what&MSG_FIELD_STATUS == MSG_FIELD_STATUS {
					changes["result"] = test.Result
				}
				if len(changes) > 0 {
					err = m.updateDocumentByID(session, COLLECTION_TESTS, test.ID, bson.M{"$set": changes})
				}
			}
		}
	case *BuildCommand:
		{
			cmd := t.(*BuildCommand)
			switch msg {
			case MSG_PERSIST_UPDATE:
				selector := bson.M{
					"_id":          cmd.test.ID,
					"commands._id": cmd.ID,
				}
				changes := bson.M{}

				if what&MSG_FIELD_OUTPUT == MSG_FIELD_OUTPUT {
					changes["commands.$.output"] = cmd.Output
				}

				if what&MSG_FIELD_STATUS == MSG_FIELD_STATUS {
					changes["commands.$.status"] = cmd.Status
				}

				err = m.updateDocument(session, COLLECTION_TESTS, selector, bson.M{"$set": changes})

			}
		}
	case *Student:
		{
			student := t.(*Student)
			switch msg {
			case MSG_PERSIST_UPDATE:
				err = m.updateDocumentByID(session, COLLECTION_STUDENTS, student.ID, student)
			}
		}
	case *Target:
		{
			target := t.(*Target)
			switch msg {
			case MSG_TARGET_LOAD:
				c := session.DB(m.dbName).C(COLLECTION_TARGETS)
				targets := []*Target{}
				c.Find(bson.M{
					"name":    target.Name,
					"version": target.Version,
				}).All(&targets)
				if len(targets) == 0 {
					// Insert
					target.ID = uuid.NewV4().String()
					err = m.insertDocument(session, COLLECTION_TARGETS, target)
				} else if len(targets) == 1 {
					target.ID = targets[0].ID
					if target.FileHash != targets[0].FileHash {
						// Sanity checks to make sure no one changed a target that has a submission.
						// (If this happens in testing, just clear the DB manually)
						changeErr := targets[0].isChangeAllowed(target)

						// Figure out if there are any submissions with this id.  If so, fail.
						var submissions []*Submission
						subColl := session.DB(m.dbName).C(COLLECTION_SUBMISSIONS)
						err = subColl.Find(bson.M{"target_id": target.ID}).Limit(1).All(&submissions)
						if err == nil {
							if len(submissions) > 0 && changeErr != nil {
								err = fmt.Errorf(
									"Target details changed and previous submissions exist. Increment the version number of the new target.\n%v", changeErr)
							} else {
								// Just update it with the new version
								err = m.updateDocumentByID(session, COLLECTION_TARGETS, target.ID, target)
							}
						}
					}
				} else {
					err = errors.New("Multiple targets exist in DB for '" + target.Name + "'")
				}
			}
		}
	case *UsageStat:
		{
			stat := t.(*UsageStat)
			switch msg {
			case MSG_PERSIST_CREATE:
				if len(stat.ID) == 0 {
					return errors.New("ID required to upsert UsageStat")
				}
				selector := bson.M{
					"_id": stat.ID,
				}
				err = m.upsertDocument(session, COLLECTION_USAGE, selector, stat)
			}
		}
	}
	return
}

func (m *MongoPersistence) CanRetrieve() bool {
	return true
}

func (m *MongoPersistence) Retrieve(what int, who map[string]interface{}, filter map[string]interface{}, res interface{}) error {
	session := m.session.Copy()
	defer session.Close()

	collection := ""

	switch what {
	case PERSIST_TYPE_STUDENTS:
		collection = COLLECTION_STUDENTS
	case PERSIST_TYPE_USERS:
		collection = COLLECTION_USERS
	default:
		return errors.New("Persistence: Invalid data type")
	}

	c := session.DB(m.dbName).C(collection)
	query := c.Find(bson.M(who))
	if filter != nil {
		query = query.Select(bson.M(filter))
	}
	return query.All(res)
}
