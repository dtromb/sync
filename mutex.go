package sync

import (
	"time"
)

type Mutex interface {
	Lock() bool
	LockPoll() bool
	LockFor(millis int) bool
	Unlock() bool
}

type MutexPair interface {
	Left() Mutex
	Right() Mutex
	Both() Mutex
}

type ChannelMutex struct {
	lock   chan bool
	closed bool
}

func OpenMutex() Mutex {
	m := ChannelMutex{
		lock: make(chan bool, 1),
	}
	return &m
}

func (m *ChannelMutex) Lock() bool {
	m.lock <- true
	if m.closed {
		return false
	}
	return true
}

func (m *ChannelMutex) LockPoll() bool {
	select {
	case m.lock <- true:
		{
			if m.closed {
				return false
			}
			return true
		}
	default:
		{
		}
	}
	return false
}

func (m *ChannelMutex) LockFor(millis int) bool {
	if millis <= 0 {
		return m.LockPoll()
	}
	timer := time.NewTimer(time.Millisecond * time.Duration(millis))
	select {
	case m.lock <- true:
		{
			if m.closed {
				return false
			}
			return true
		}
	case <-timer.C:
		{
		}
	}
	return false
}

func (m *ChannelMutex) Unlock() bool {
	select {
	case _, ok := <-m.lock:
		{
			if !ok {
				return false
			}
			return true
		}
	default:
		{
		}
	}
	return false
}

type ChannelMutexPair struct {
	lock   Mutex
	aSig   chan bool
	bSig   chan bool
	abSig  chan bool
	aFree  chan bool
	bFree  chan bool
	abFree chan bool
	closed bool
}

func OpenMutexPair() MutexPair {
	mp := &ChannelMutexPair{
		lock:   OpenMutex(),
		aSig:   make(chan bool, 1),
		bSig:   make(chan bool, 1),
		abSig:  make(chan bool, 1),
		aFree:  make(chan bool, 1),
		bFree:  make(chan bool, 1),
		abFree: make(chan bool, 1),
	}
	// No reference to mp so that GC can close on finalize, killing this
	// goroutine.
	go func(lock Mutex, a chan bool, b chan bool, ab chan bool,
		fA chan bool, fB chan bool, fAB chan bool) {
		// First signal A+B free.
		var aState bool
		var bState bool
		fAB <- true
		for {
			// Wait for a state change signal.
			select {
			case ax, ok := <-a:
				{
					if !ok {
						goto shutdown
					}
					lock.Lock()
					if ax {
						// Acquire A
						aState = true
					} else {
						// Release A
						aState = false
						if bState {
							select {
							case fA <- true:
								{
								}
							default:
								{
								}
							}
						} else {
							select {
							case fAB <- true:
								{
								}
							default:
								{
								}
							}
						}
					}
					lock.Unlock()
				}
			case bx, ok := <-b:
				{
					if !ok {
						goto shutdown
					}
					lock.Lock()
					if bx {
						// Acquire B
						bState = true
					} else {
						// Release B
						bState = false
						if aState {
							select {
							case fB <- true:
								{
								}
							default:
								{
								}
							}
						} else {
							select {
							case fAB <- true:
								{
								}
							default:
								{
								}
							}
						}
					}
					lock.Unlock()
				}
			case abx, ok := <-ab:
				{
					if !ok {
						goto shutdown
					}
					lock.Lock()
					if abx {
						// Acquire both
						aState = true
						bState = true
					} else {
						// Release both
						aState = false
						bState = false
						select {
						case fAB <- true:
							{
							}
						default:
							{
							}
						}
					}
					lock.Unlock()
				}
			}
		}
	shutdown:
		{
		}
	}(mp.lock, mp.aSig, mp.bSig, mp.abSig, mp.aFree, mp.bFree, mp.abFree)
	return mp
}

type ChannelMutexPairSingleSide struct {
	cmp  *ChannelMutexPair
	side bool
}

func (ss *ChannelMutexPairSingleSide) Lock() bool {
	return ss.TimedLock(nil)
}

func (ss *ChannelMutexPairSingleSide) LockFor(millis int) bool {
	return ss.TimedLock(time.NewTimer(time.Millisecond * time.Duration(millis)).C)
}

func (ss *ChannelMutexPairSingleSide) LockPoll() bool {
	c := make(chan time.Time, 1)
	c <- time.Now()
	return ss.TimedLock(c)
}

func (ss *ChannelMutexPairSingleSide) TimedLock(tc <-chan time.Time) bool {
	var sig, singleFree chan bool
	if ss.side {
		sig = ss.cmp.bSig
		singleFree = ss.cmp.bFree
	} else {
		sig = ss.cmp.aSig
		singleFree = ss.cmp.aFree
	}
	select {
	case _, ok := <-ss.cmp.abFree:
		{
			if !ok {
				return false
			}
			sig <- true
			return true
		}
	case _, ok := <-singleFree:
		{
			if !ok {
				return false
			}
			sig <- true
			return true
		}
	case <-tc:
		{
			return false
		}
	}
}

func (ss *ChannelMutexPairSingleSide) Unlock() bool {
	if ss.side {
		ss.cmp.bSig <- false
	} else {
		ss.cmp.aSig <- false
	}
	return true
}

func (cmp *ChannelMutexPair) Left() Mutex {
	return &ChannelMutexPairSingleSide{
		cmp:  cmp,
		side: false,
	}
}

func (cmp *ChannelMutexPair) Right() Mutex {
	return &ChannelMutexPairSingleSide{
		cmp:  cmp,
		side: true,
	}
}

func (cmp *ChannelMutexPair) Both() Mutex {
	return cmp
}

func (cmp *ChannelMutexPair) Lock() bool {
	return cmp.TimedLock(nil)
}

func (cmp *ChannelMutexPair) LockFor(millis int) bool {
	return cmp.TimedLock(time.NewTimer(time.Millisecond * time.Duration(millis)).C)
}

func (cmp *ChannelMutexPair) LockPoll() bool {
	c := make(chan time.Time, 1)
	c <- time.Now()
	return cmp.TimedLock(c)
}

func (cmp *ChannelMutexPair) TimedLock(tc <-chan time.Time) bool {
	select {
	case _, ok := <-cmp.abFree:
		{
			if !ok {
				return false
			}
			cmp.abSig <- true
			return true
		}
	case <-tc:
		{
			return false
		}
	}
}

func (cmp *ChannelMutexPair) Unlock() bool {
	cmp.abSig <- false
	return true
}
