package sync

import (
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

func (c *Cond) Mutex() *Mutex {
	return c.m
}

func (c *Cond) Wait(key *MutexKey) error {
	<-c.m.lock
	if key == nil {
		log.Debugf("Wait() on conditon %p:%p with nil key", c, c.m)
	} else {
		log.Debugf("Wait() on condition %p:%p with key %s", c, c.m, key.debugStackEnt)
	}
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
	if key.m.owner == nil || key.m.owner.id != key.id {
		return errors.New("key is not the mutex locker")
	}
	key.waiting = true
	key.m.owner = nil
	log.Debugf(" Wait() clearing mutex")
	<-key.m.ownerLock
	log.Debugf(" Wait() done clearing mutex")
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
		log.Debugf(" Wait() created new signal channel")
		key.signal = make(chan bool, 1)
	}
	wl.active = true
	log.Debugf(" Wait() releasing mutex guard")
	c.m.lock <- true
	log.Debugf(" Wait() sleeping until signalled")
	<-key.signal
	log.Debugf(" Wait() woke, getting lock back")
	resume := key.count
	key.count = 0
	c.m.Lock(key)
	key.count = resume
	key.waiting = false
	log.Debugf(" Wait() resumed %d entries; finishing with locked %p", key.count, c.m)
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