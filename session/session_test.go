package session

import (
	"testing"
	"time"
)

func TestSessionData(t *testing.T) {
	sd := NewSessionData()

	// get not exist
	{
		v, ok := sd.Get(123)
		if v != nil {
			t.Fatal("NewSessionData should be empty", sd, v, ok)
		}
		if ok {
			t.Fatal("NewSessionData should be empty", sd, v, ok)
		}
	}

	// set & get
	{
		k, v := 123, "123"
		sd.Set(k, v)
		v1, ok := sd.Get(k)
		if !ok {
			t.Fatal("NewSessionData should have data", sd, v1, ok)
		}
		if v1 != v {
			t.Fatal("NewSessionData data not same", sd, v, v1, ok)
		}

	}

	// TTL
	{
		if sd.IsTimeout() {
			t.Fatal("NewSessionData should not timeout", sd)
		}

		sd.ttl = time.Now().Add(-1 * time.Second)
		if !sd.IsTimeout() {
			t.Fatal("NewSessionData should timeout", sd)
		}

		sd.Renew()
		if sd.IsTimeout() {
			t.Fatal("NewSessionData should not timeout", sd)
		}
	}

}

func TestMemSession(t *testing.T) {
	sess := NewMemSession()
	defer sess.Close()

	// get not exist
	{
		token := "11111"
		sd := sess.GetOrRenew(token)
		if sd != nil {
			t.Fatal("SessionData should be nil", sess, sd)
		}
		sess.Destroy(token)
	}

	// new, renew, ttl
	{
		token := "22222"
		sd := sess.New(token)
		if sd == nil {
			t.Fatal("SessionData should not be nil", sess, sd)
		}

		sd = sess.GetOrRenew(token)
		if sd == nil {
			t.Fatal("SessionData should not be nil", sess, sd)
		}

		sd.ttl = time.Now().Add(-1 * time.Second)
		sd = sess.GetOrRenew(token)
		if sd != nil {
			t.Fatal("SessionData should be nil (TTL)", sess, sd)
		}
		sess.Destroy(token)
	}

	// renew
	{
		token := "33333"
		sd := sess.New(token)
		if sd == nil {
			t.Fatal("SessionData should not be nil", sess, sd)
		}

		sd.ttl = time.Now().Add(1 * time.Second)
		sd = sess.GetOrRenew(token)
		if sd == nil {
			t.Fatal("SessionData should not be nil", sess, sd)
		}
		sess.Destroy(token)
	}

	// destroy
	{
		token := "44444"
		sd := sess.New(token)
		if sd == nil {
			t.Fatal("SessionData should not be nil", sess, sd)
		}

		sess.Destroy(token)

		sd = sess.GetOrRenew(token)
		if sd != nil {
			t.Fatal("SessionData should be nil", sess, sd)
		}

		// try destroy not exist
		sess.Destroy("[not exist]" + token + token)
	}

	// clean
	{
		token := "55555"
		sd := sess.New(token)
		if sd == nil {
			t.Fatal("SessionData should not be nil", sess, sd)
		}

		sz0 := sess.Len()
		sd.ttl = time.Now().Add(-1 * time.Second)
		sess.clean()
		sz1 := sess.Len()
		if sz0 == sz1 {
			t.Fatal("Session should be clean", sess, sz0, sz1)
		}

		sess.Destroy(token)
	}

	// auto gen
	{
		count := 100
		for i := 0; i < count; i++ {
			token, sd := sess.NewToken()
			if token == "" {
				t.Fatal("token should not be empty", sess, token, sd)
			}
			if sd == nil {
				t.Fatal("SessionData should not be nil", sess, token, sd)
			}
		}

		sz0 := sess.Len()
		if sz0 != count {
			t.Fatal("Session count should be same", sess, sz0, count)
		}
	}
}
