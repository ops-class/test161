package test161

import (
	"errors"
)

// A fake persistence to use for test code

var testStudent = &Student{
	Email: "test@test161.ops-class.org",
	Token: "TestToken4$5^",
}

type TestingPersistence struct {
	Verbose bool
}

func (p *TestingPersistence) Close() {
}

func (p *TestingPersistence) Notify(entity interface{}, msg, what int) error {
	return nil
}

func (d *TestingPersistence) CanRetrieve() bool {
	return true
}

func (d *TestingPersistence) Retrieve(what int, who map[string]interface{},
	filter map[string]interface{}, res interface{}) error {

	switch what {
	case PERSIST_TYPE_STUDENTS:
		if email, _ := who["email"]; email == testStudent.Email {
			if token, _ := who["token"]; token == testStudent.Token {
				students := res.(*[]*Student)
				*students = append(*students, testStudent)
			}
		}

		return nil

	case PERSIST_TYPE_USERS:
		// Only currently used to get the staff flag, and only the length is
		// checked. Since it's like 4 levels deep, we'll just fake it.
		if id, _ := who["services.auth0.email"]; id == testStudent.Email {
			results := res.(*[]interface{})
			*results = append(*results, 1)
		}

		return nil
	default:
		return errors.New("Persistence: Invalid data type")
	}

}
