package concurrency

import (
	"fmt"
	"time"
	"errors"
)

type waitList struct {
	key *MutexKey
	nxt *waitList
	prv *waitList
	active bool
}
type Cond struct {
	m *Mutex
	hd *waitList
	tl *waitList
}

func (c *Cond) Wait(key *MutexKey) error {
	if key == nil {
		fmt.Println("WAIT <nil>")
	} else {
		fmt.Printf("WAIT <%s>\n", key.debugStackEnt)
	}
	<-c.m.lock
	defer func() { 
		select{
			case c.m.lock <- true:
			default:
		}
	}()
	if key == nil || key.id == 0 {
		return errors.New("invalid mutex key")
	}
	if key.m != c.m {
		return errors.New("incorrect mutex key context")
	}
	if key.m.owner.id != key.id {
		return errors.New("key is not the mutex locker")
	}
	key.waiting = true
	key.m.owner = nil
	<-key.m.ownerLock
	wl := &waitList{key:key}
	key.wl = wl
	if c.hd == nil {
		c.hd = wl
	} else {
		c.tl.nxt = wl
		wl.prv = c.tl
	}
	c.tl = wl
	if key.signal == nil {
		key.signal = make(chan bool, 1)
	}
	wl.active = true
	c.m.lock <- true
	<-key.signal
	resume := key.count
	key.count = 0
	c.m.Lock(key)
	key.count = resume
	key.waiting = false
	return nil
}

func (c *Cond) WaitTimed(key *MutexKey, wait time.Duration) (bool, error) {
	ts := time.Now()
	expire := ts.Add(wait)
	<-c.m.lock
	defer func() { 
		select{
			case c.m.lock <- true:
			default:
		}
	}()
	if key == nil || key.id == 0 {
		return false, errors.New("invalid mutex key")
	}
	if key.m != c.m {
		return false, errors.New("incorrect mutex key context")
	}
	if key.m.owner.id != key.id {
		return false, errors.New("key is not the mutex locker")
	}
	key.waiting = true
	key.m.owner = nil
	<-key.m.ownerLock
	wl := &waitList{key:key}
	key.wl = wl
	if c.hd == nil {
		c.hd = wl
	} else {
		c.tl.nxt = wl
		wl.prv = c.tl
	}
	c.tl = wl
	if key.signal == nil {
		key.signal = make(chan bool, 1)
	}
	wl.active = true
	c.m.lock <- true
	waitTime := expire.Sub(time.Now())
	cancel := func() {
		<-c.m.lock
		if wl.active {
			if wl.prv != nil {
				wl.prv.nxt = wl.nxt
			} else {
				c.hd = wl.nxt
			}
			if wl.nxt != nil {
				wl.nxt.prv = wl.prv
			} else {
				c.tl = wl.prv
			}
			wl.active = false
		}
		key.waiting = false
	}
	if waitTime <= 0 {
		cancel()
		return false, nil
	}
	timer := time.NewTimer(wait)
	select {
		case <-key.signal: {
			timer.Stop()
			resume := key.count
			key.count = 0
			waitTime = expire.Sub(time.Now())
			if waitTime <= 0 {
				cancel()
				return false, nil
			}
			_, ok := c.m.LockWait(key, waitTime)
			if !ok {
				cancel()
				return false, nil
			}
			key.count = resume
			key.waiting = false
			return true, nil
		}
		case <-timer.C: {
			cancel()
			return false, nil
		}
	}
}

func (c *Cond) Notify() {
	<-c.m.lock
	defer func() { c.m.lock <- true }()
	if c.hd != nil {
		notifyKey := c.hd.key
		c.hd.active = false
		c.hd = c.hd.nxt
		select {
			case notifyKey.signal <- true:
			default:
		}
	}
}

func (c *Cond) Broadcast() {
	<-c.m.lock
	defer func() { c.m.lock <- true }()
	for c.hd != nil {
		notifyKey := c.hd.key
		c.hd.active = false
		c.hd = c.hd.nxt
		select {
			case notifyKey.signal <- true:
			default:
		}
	}
}