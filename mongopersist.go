package test161

import (
	"fmt"
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
		err = fmt.Errorf("Unexpected type in Notify(): %T", t)
	case *Test:
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
	case *Command:
		cmd := t.(*Command)
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

			if what&MSG_FIELD_SCORE == MSG_FIELD_SCORE {
				changes["commands.$.points_earned"] = cmd.PointsEarned
			}

			if what&MSG_FIELD_STATUS == MSG_FIELD_STATUS {
				changes["commands.$.status"] = cmd.Status
			}

			err = m.updateDocument(session, COLLECTION_TESTS, selector, bson.M{"$set": changes})

		}
	case *Submission:
		submission := t.(*Submission)
		switch msg {
		case MSG_PERSIST_CREATE:
			err = m.insertDocument(session, COLLECTION_SUBMISSIONS, submission)
		case MSG_PERSIST_COMPLETE:
			fallthrough
		case MSG_PERSIST_UPDATE:
			err = m.updateDocumentByID(session, COLLECTION_SUBMISSIONS, submission.ID, submission)
		}
	case *BuildTest:
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
	case *BuildCommand:
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

	return
}
