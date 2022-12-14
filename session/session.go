package session

import (
	"sync"
	"time"

	"tiddlywikid/utils"
)

var (
	SESSION_TTL = 15 * 60 * time.Second // 15 min
)

type SessionStore interface {
	GetOrRenew(token string) *SessionData
	Destroy(token string)
	New(token string) *SessionData
	NewToken() (string, *SessionData)
	Close()
}

type SessionData2 interface {
	IsTimeout() bool
	Renew()
	Get(k interface{}) (interface{}, bool)
	Set(k interface{}, v interface{})
}

type SessionData struct {
	mx  sync.RWMutex
	ttl time.Time
	lst map[interface{}]interface{}
}

func (sd *SessionData) IsTimeout() bool {
	sd.mx.RLock()
	defer sd.mx.RUnlock()
	return utils.Now().After(sd.ttl)
}

func (sd *SessionData) Renew() {
	sd.mx.Lock()
	sd.ttl = utils.Now().Add(SESSION_TTL)
	sd.mx.Unlock()
}

func (sd *SessionData) Get(k interface{}) (interface{}, bool) {
	sd.mx.RLock()
	v, ok := sd.lst[k]
	sd.mx.RUnlock()
	return v, ok
}

func (sd *SessionData) Set(k interface{}, v interface{}) {
	sd.mx.Lock()
	sd.lst[k] = v
	sd.mx.Unlock()
}

func NewSessionData() *SessionData {
	sd := &SessionData{
		ttl: utils.Now().Add(SESSION_TTL),
		lst: make(map[interface{}]interface{}),
	}
	return sd
}
