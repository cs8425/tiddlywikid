package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
)

// TODO: lock?
type ACL struct {
	Allow []netip.Prefix `json:"allow,omitempty"`
	Block []netip.Prefix `json:"block,omitempty"`
	Def   bool           `json:"def"`
}

func (a *ACL) Check(addrPort string) bool {
	ipport, err := netip.ParseAddrPort(addrPort)
	if err != nil {
		return a.Def
	}
	ip := ipport.Addr()
	for _, prefix := range a.Block {
		if prefix.Contains(ip) {
			return false
		}
	}
	for _, prefix := range a.Allow {
		if prefix.Contains(ip) {
			return true
		}
	}
	return a.Def
}

// no change any if have any error?
func (a *ACL) Set(def bool, allowIPs []string, blockIPs []string) error {
	allow := make([]netip.Prefix, 0, len(allowIPs))
	for _, str := range allowIPs {
		addr, err := netip.ParsePrefix(str)
		if err != nil {
			return err
		}
		allow = append(allow, addr)
	}
	block := make([]netip.Prefix, 0, len(blockIPs))
	for _, str := range blockIPs {
		addr, err := netip.ParsePrefix(str)
		if err != nil {
			return err
		}
		block = append(block, addr)
	}
	a.Def = def
	a.Allow = allow
	a.Block = block
	return nil
}

func NewACL(def bool) *ACL {
	return &ACL{
		Def: def,
	}
}

type AuthCustom struct {
	AccessStaticFile *ACL    `json:"static-file,omitempty"`
	Anonymous        *ACL    `json:"allow-anonymous,omitempty"`
	AnonymousEdit    *ACL    `json:"allow-anonymous-edit,omitempty"`
	UserDB           *UserDB `json:"users,omitempty"`
}

func (a *AuthCustom) AllowAnonymousAccessStaticFile(req *http.Request) bool {
	return a.AccessStaticFile.Check(req.RemoteAddr)
}

func (a *AuthCustom) SetStaticFile(def bool, allowIPs []string, blockIPs []string) error {
	return a.AccessStaticFile.Set(def, allowIPs, blockIPs)
}

func (a *AuthCustom) AllowAnonymous(req *http.Request) bool {
	return a.Anonymous.Check(req.RemoteAddr)
}

func (a *AuthCustom) SetAnonymous(def bool, allowIPs []string, blockIPs []string) error {
	return a.Anonymous.Set(def, allowIPs, blockIPs)
}

func (a *AuthCustom) AllowAnonymousEdit(req *http.Request) bool {
	return a.AnonymousEdit.Check(req.RemoteAddr)
}

func (a *AuthCustom) SetAnonymousEdit(def bool, allowIPs []string, blockIPs []string) error {
	return a.AnonymousEdit.Set(def, allowIPs, blockIPs)
}

func (a *AuthCustom) Login(user string, pwd string, req *http.Request) (displayName string, ok bool) {
	u := a.UserDB.Get(user)
	if u == nil {
		return "", false
	}
	ok = u.CheckPwd(pwd)
	if !ok {
		return "", false
	}
	return u.Name, true
}

func (a *AuthCustom) Load(fp string) error {
	fd, err := os.Open(fp)
	if err != nil {
		return err
	}
	defer fd.Close()

	nacl := AuthCustom{}
	dec := json.NewDecoder(fd)
	err = dec.Decode(&nacl)
	if err != nil {
		return err
	}
	*a = nacl
	return nil
}

func NewAuthCustom() *AuthCustom {
	return &AuthCustom{
		Anonymous:        NewACL(true),
		AccessStaticFile: NewACL(true),
		AnonymousEdit:    NewACL(false),
		UserDB:           NewUserDB(""),
	}
}

// make sure AuthCustom implement Auth
var _ Auth = (*AuthCustom)(nil)

// simple db
type User struct {
	Login string `json:"id"`
	Name  string `json:"name,omitempty"` // for display in wiki
	Hash  string `json:"hash"`           // with salt
}

func (u *User) CheckPwd(pwd string) bool {
	sh := strings.SplitN(u.Hash, ":", 2)
	if len(sh) != 2 {
		return false
	}
	hash := pwdHash(pwd, sh[0])
	return hash == sh[1]
}

func (u *User) SetPwd(pwd string) bool {
	salt := genSalt()
	if salt == "" {
		return false
	}
	hash := pwdHash(pwd, salt)
	u.Hash = salt + ":" + hash
	return true
}

func genSalt() string {
	buf := make([]byte, 12)
	_, err := rand.Read(buf)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func pwdHash(pwd string, salt string) string {
	shah := sha256.New()
	shah.Write([]byte(pwd + "-:-" + salt))
	return base64.StdEncoding.EncodeToString(shah.Sum([]byte("")))
}

type UserDB struct {
	atomic.Value // map[string]*User // login -> user
}

func (s *UserDB) Get(login string) *User {
	lst := s.Value.Load().(map[string]*User)
	return lst[login]
}

func (s *UserDB) UnmarshalJSON(buf []byte) error {
	aux := make([]*User, 0, 64)
	err := json.Unmarshal(buf, &aux)
	if err != nil {
		return err
	}

	ns := make(map[string]*User)
	for _, user := range aux {
		ns[user.Login] = user
	}
	s.Value.Store(ns)
	return nil
}

func (s *UserDB) Load(fp string) error {
	fd, err := os.Open(fp)
	if err != nil {
		return err
	}
	defer fd.Close()

	dec := json.NewDecoder(fd)
	err = dec.Decode(&s)
	if err != nil {
		return err
	}
	return nil
}

func NewUserDB(fp string) *UserDB {
	db := &UserDB{}
	if fp != "" {
		err := db.Load(fp)
		if err != nil {
			db.Value.Store(make(map[string]*User))
		}
	} else {
		db.Value.Store(make(map[string]*User))
	}
	return db
}
