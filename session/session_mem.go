package session

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

type MemSession struct {
	die    chan struct{}
	mx     sync.RWMutex
	cookie map[string]*SessionData
}

func (ss *MemSession) Len() int {
	ss.mx.RLock()
	sz := len(ss.cookie)
	ss.mx.RUnlock()
	return sz
}

func (ss *MemSession) GetOrRenew(token string) *SessionData {
	ss.mx.RLock()
	sd, ok := ss.cookie[token]
	ss.mx.RUnlock()
	if !ok {
		return nil
	}
	if sd.IsTimeout() {
		ss.mx.Lock()
		delete(ss.cookie, token)
		ss.mx.Unlock()
		return nil
	}
	sd.Renew()
	return sd
}

func (ss *MemSession) Destroy(token string) {
	ss.mx.RLock()
	_, ok := ss.cookie[token]
	ss.mx.RUnlock()
	if ok {
		ss.mx.Lock()
		delete(ss.cookie, token)
		ss.mx.Unlock()
	}
}

func (ss *MemSession) New(token string) *SessionData {
	sd := NewSessionData()
	ss.mx.Lock()
	sd0, ok := ss.cookie[token]
	if ok {
		ss.mx.Unlock()
		return sd0
	}
	ss.cookie[token] = sd
	ss.mx.Unlock()
	return sd
}

func (ss *MemSession) NewToken() (string, *SessionData) {
	sd := NewSessionData()
	token := genToken()

	fail := true
	ss.mx.Lock()
	for i := 0; i < 10000; i++ {
		_, ok := ss.cookie[token]
		if !ok {
			ss.cookie[token] = sd
			fail = false
			break
		}
		token = genToken()
	}
	ss.mx.Unlock()

	if fail {
		return "", nil
	}
	return token, sd
}

func (ss *MemSession) clean() {
	ss.mx.RLock()
	rmLst := make([]string, 0, len(ss.cookie))
	for k, sd := range ss.cookie {
		if sd.IsTimeout() {
			rmLst = append(rmLst, k)
		}
	}
	ss.mx.RUnlock()

	if len(rmLst) > 0 {
		ss.mx.Lock()
		for _, token := range rmLst {
			delete(ss.cookie, token)
		}
		ss.mx.Unlock()
	}
}

func (ss *MemSession) cleaner() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ss.die:
			return
		case <-ticker.C:
			ss.clean()
		}
	}
}

func (ss *MemSession) Close() {
	select {
	case <-ss.die:
	default:
		close(ss.die)
	}
}

func NewMemSession() *MemSession {
	sess := &MemSession{
		cookie: make(map[string]*SessionData),
		die:    make(chan struct{}),
	}
	go sess.cleaner()
	return sess
}

// make sure MemSession implement Session
var _ SessionStore = (*MemSession)(nil)

func genToken() string {
	buf := make([]byte, 18)
	_, err := rand.Read(buf)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
