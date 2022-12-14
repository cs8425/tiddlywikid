package utils

import (
	"log"
	"sync/atomic"
	"time"
)

var (
	_low_precise_time       atomic.Value // time.Time
	LOW_PRECISE_UPDATE_TIME = 500 * time.Millisecond
)

// low precise, should enough for session
func Now() time.Time {
	t, ok := _low_precise_time.Load().(time.Time)
	if !ok {
		return time.Now()
	}
	return t
}

var (
	Verbosity = 3
)

func Vf(level int, format string, v ...interface{}) {
	if level <= Verbosity {
		log.Printf(format, v...)
	}
}
func V(level int, v ...interface{}) {
	if level <= Verbosity {
		log.Print(v...)
	}
}
func Vln(level int, v ...interface{}) {
	if level <= Verbosity {
		log.Println(v...)
	}
}
