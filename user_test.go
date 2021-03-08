package openproject

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
)

func TestUserService_Get_Success(t *testing.T) {
	setup()
	defer teardown()
	raw, err := ioutil.ReadFile("./mocks/get/get-users-no-filters.json")
	if err != nil {
		t.Error(err.Error())
	}
	testMux.HandleFunc("/api/v3/users", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testRequestURL(t, r, "/api/v3/users")

		fmt.Fprint(w, string(raw))
	})

	if user, _, err := testClient.User.Get(""); err != nil {
		t.Errorf("Error given: %s", err)
	} else if user == nil {
		t.Error("Expected user. User is nil")
	}
}

func TestUserService_GetByID_Success(t *testing.T) {
	setup()
	defer teardown()
	raw, err := ioutil.ReadFile("./mocks/get/get-user.json")
	if err != nil {
		t.Error(err.Error())
	}
	testMux.HandleFunc("/api/v3/users", func(w http.ResponseWriter, r *http.Request) {
		testMethod(t, r, "GET")
		testRequestURL(t, r, "/api/v3/users?id=1")

		fmt.Fprint(w, string(raw))
	})

	if user, _, err := testClient.User.Get("1"); err != nil {
		t.Errorf("Error given: %s", err)
	} else if user == nil {
		t.Error("Expected user. User is nil")
	}
}