package auth

import (
	"net/http"
)

type Auth interface {
	AllowAnonymous(req *http.Request) bool
	AllowAnonymousAccessStaticFile(req *http.Request) bool // TODO: implement
	AllowAnonymousEdit(req *http.Request) bool
	Login(user string, pwd string, req *http.Request) (displayName string, ok bool)
}

type AuthAllowAll struct{}

func (a *AuthAllowAll) AllowAnonymousAccessStaticFile(req *http.Request) bool {
	return true
}

func (a *AuthAllowAll) AllowAnonymous(req *http.Request) bool {
	return true
}

func (a *AuthAllowAll) AllowAnonymousEdit(req *http.Request) bool {
	return true
}

func (a *AuthAllowAll) Login(user string, pwd string, req *http.Request) (displayName string, ok bool) {
	return "", true
}

// make sure AuthAllowAll implement Auth
var _ Auth = (*AuthAllowAll)(nil)

type AuthAnnoRead struct{}

func (a *AuthAnnoRead) AllowAnonymousAccessStaticFile(req *http.Request) bool {
	return true
}

func (a *AuthAnnoRead) AllowAnonymous(req *http.Request) bool {
	return true
}

func (a *AuthAnnoRead) AllowAnonymousEdit(req *http.Request) bool {
	return false
}

func (a *AuthAnnoRead) Login(user string, pwd string, req *http.Request) (displayName string, ok bool) {
	return "", user == "aaa" && pwd == "123"
}

// make sure AuthAnnoRead implement Auth
var _ Auth = (*AuthAnnoRead)(nil)
