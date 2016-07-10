package ratelimit

import "time"

type Limiter struct {
	rate time.Duration
	ch   chan struct{}
}

func New(rate time.Duration, eventsBeforeLimit int) *Limiter {
	l := &Limiter{rate, make(chan struct{}, uint(eventsBeforeLimit))}
	go l.watch()
	return l
}

func (l *Limiter) GrabTicket() {
	<-l.ch
}

func (l *Limiter) watch() {
	// fill queue to begin with
	for i := 0; i < cap(l.ch); i++ {
		l.ch <- struct{}{}
	}

	for {
		l.ch <- struct{}{}
		time.Sleep(l.rate)
	}
}
