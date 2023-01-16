package auth

import (
	"testing"
)

var testCase = []struct {
	inst   Auth
	Name   string
	Anno   bool
	Static bool
	Edit   bool
}{
	{
		&AuthAllowAll{},
		"AuthAllowAll",
		true,
		true,
		true,
	},
	{
		&AuthAnnoRead{},
		"AuthAnnoRead",
		true,
		true,
		false,
	},
	// {
	// 	NewAuthCustom(),
	// 	"AuthCustom (default)",
	// 	true,
	// 	true,
	// 	false,
	// },
}

func TestAuth(t *testing.T) {

	for _, test := range testCase {
		auth := test.inst

		b0 := auth.AllowAnonymous(nil)
		if b0 != test.Anno {
			t.Fatal("AllowAnonymous should be", test.Anno, "got", b0)
		}

		b1 := auth.AllowAnonymousAccessStaticFile(nil)
		if b1 != test.Static {
			t.Fatal("AllowAnonymousAccessStaticFile should be", test.Static, "got", b1)
		}

		b2 := auth.AllowAnonymousEdit(nil)
		if b1 != test.Static {
			t.Fatal("AllowAnonymousEdit should be", test.Edit, "got", b2)
		}
	}

}
