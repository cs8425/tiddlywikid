package utils

import (
	"testing"
	"time"
)

func TestLowPreciseTime(t *testing.T) {
	t0 := Now()
	t1 := Now()
	time.Sleep(1 * time.Second)
	t2 := Now()

	if t0 == t2 {
		t.Fatal("LowPreciseTimer not run:", t0, t2)
	}

	dt02 := t2.Sub(t0)
	if dt02 < 500*time.Millisecond || dt02 > 1500*time.Millisecond {
		t.Fatal("LowPreciseTimer rate error?", t0, t2, dt02)
	}

	dt01 := t1.Sub(t0)
	if dt01 > 600*time.Millisecond {
		t.Fatal("LowPreciseTimer update time != 500 ms?", t0, t1, dt01)
	}
}
