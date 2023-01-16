package auth

import (
	"testing"
)

func TestACLDefaultBlock(t *testing.T) {
	var testCase = []struct {
		Addr string
		Ret  bool
	}{
		{
			"127.0.0.1:23456",
			true,
		},
		{
			"192.168.0.1:23456",
			true,
		},
		{
			"192.168.10.1:23456",
			false,
		},
		{
			"192.168.11.1:23456",
			false,
		},
		{
			"12.34.56.78:23456",
			false,
		},
	}

	acl := NewACL(false)
	acl.Set(false, []string{
		"127.0.0.1/8",
		"192.168.0.0/24",
	}, []string{
		"192.168.10.0/24",
	})

	testACL(t, acl, testCase)
}

func TestACLDefaultAllow(t *testing.T) {
	var testCase = []struct {
		Addr string
		Ret  bool
	}{
		{
			"127.0.0.1:23456",
			true,
		},
		{
			"192.168.0.1:23456",
			true,
		},
		{
			"192.168.10.1:23456",
			false,
		},
		{
			"192.168.11.1:23456",
			true,
		},
		{
			"12.34.56.78:23456",
			true,
		},
	}

	acl := NewACL(true)
	acl.Set(true, []string{
		"127.0.0.1/8",
		"192.168.0.0/24",
	}, []string{
		"192.168.10.0/24",
	})

	testACL(t, acl, testCase)
}

func testACL(t *testing.T, acl *ACL, testCase []struct {
	Addr string
	Ret  bool
}) {
	for _, test := range testCase {
		ret := acl.Check(test.Addr)
		if ret != test.Ret {
			t.Fatal("acl", test.Addr, "should be", test.Ret, "got", ret)
		}
	}
}

func TestUser(t *testing.T) {
	pwd := "test"
	user := &User{}

	// set and check
	user.SetPwd(pwd)
	if user.CheckPwd(pwd + "-----") {
		t.Fatal("password should not match")
	}
	if !user.CheckPwd(pwd) {
		t.Fatal("password should match")
	}

	// check precalculate hash
	user.Hash = "1vm953mjdu+u8t9I:Xm0ZJOrbz+G4B1SClnN27SHW6hJ3hBrSXBf4pBemYEQ="
	if !user.CheckPwd("test") {
		t.Fatal("password should match")
	}
}

func TestUserDB(t *testing.T) {
	db := NewUserDB("")

	// empty db
	if user := db.Get("user"); user != nil {
		t.Fatal("user should not exist")
	}

	// add a user
	id := "user1"
	m := db.Value.Load().(map[string]*User)
	m[id] = &User{}

	if user := db.Get(id); user == nil {
		t.Fatal("user should exist")
	}

	// load from json
	err := db.UnmarshalJSON(([]byte)(`
[
	{
		"id": "test",
		"name": "",
		"hash": "1vm953mjdu+u8t9I:Xm0ZJOrbz+G4B1SClnN27SHW6hJ3hBrSXBf4pBemYEQ="
	}
]
`))
	if err != nil {
		t.Fatal("load db error", err)
	}

	if user := db.Get(id); user != nil {
		t.Fatal("user should not exist")
	}
	if user := db.Get("test"); user == nil {
		t.Fatal("user should exist")
	}

}
