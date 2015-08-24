package sync

import (
	"time"
)

// Mutex is a pollable, waitable mutual exclusion facility.  It is non-re-entrant
// and does not check release callers.
type Mutex interface {
	
	// Lock attempts to block indefinitely until the mutex can lock.  Returns true on success,
	// or false if the mutex was closed.
	Lock() bool
	
	// LockPoll attempts to acquire the mutex and returns false immediately if it is not available.
	LockPoll() bool
	
	// LockFor attempts to acquire the lock for at least *millis* milliseconds, and returns true
	// if successful.
	LockFor(millis int) bool
	
	// Unlock releases the mutex if it is locked.  It always returns true.
	Unlock() bool
}

// MutexPair is a pair of mutexes that can be locked/unlocked atomically.  It is
// presented as a triple of Mutex interfaces that operate on nonempty subsets of the pair.
type MutexPair interface {
	Left() Mutex
	Right() Mutex
	Both() Mutex
}

type channelMutex struct {
	lock   chan bool
	closed bool
}

// OpenMutex opens a single Mutex implementation backed by channel-based concurrency.
func OpenMutex() Mutex {
	m := channelMutex{
		lock: make(chan bool, 1),
	}
	return &m
}

func (m *channelMutex) Lock() bool {
	m.lock <- true
	if m.closed {
		return false
	}
	return true
}

func (m *channelMutex) LockPoll() bool {
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

func (m *channelMutex) LockFor(millis int) bool {
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

func (m *channelMutex) Unlock() bool {
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

type channelMutexPair struct {
	lock   Mutex
	aSig   chan bool
	bSig   chan bool
	abSig  chan bool
	aFree  chan bool
	bFree  chan bool
	abFree chan bool
	closed bool
}

// OpenMutexPair opens a MutexPair implementation backed by channel-based concurrency.
// It spawns a single goroutine which coordinates atomic operations without retry.
func OpenMutexPair() MutexPair {
	mp := &channelMutexPair{
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

type channelMutexPairSingleSide struct {
	cmp  *channelMutexPair
	side bool
}

func (ss *channelMutexPairSingleSide) Lock() bool {
	return ss.TimedLock(nil)
}

func (ss *channelMutexPairSingleSide) LockFor(millis int) bool {
	return ss.TimedLock(time.NewTimer(time.Millisecond * time.Duration(millis)).C)
}

func (ss *channelMutexPairSingleSide) LockPoll() bool {
	c := make(chan time.Time, 1)
	c <- time.Now()
	return ss.TimedLock(c)
}

func (ss *channelMutexPairSingleSide) TimedLock(tc <-chan time.Time) bool {
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

func (ss *channelMutexPairSingleSide) Unlock() bool {
	if ss.side {
		ss.cmp.bSig <- false
	} else {
		ss.cmp.aSig <- false
	}
	return true
}

func (cmp *channelMutexPair) Left() Mutex {
	return &channelMutexPairSingleSide{
		cmp:  cmp,
		side: false,
	}
}

func (cmp *channelMutexPair) Right() Mutex {
	return &channelMutexPairSingleSide{
		cmp:  cmp,
		side: true,
	}
}

func (cmp *channelMutexPair) Both() Mutex {
	return cmp
}

func (cmp *channelMutexPair) Lock() bool {
	return cmp.TimedLock(nil)
}

func (cmp *channelMutexPair) LockFor(millis int) bool {
	return cmp.TimedLock(time.NewTimer(time.Millisecond * time.Duration(millis)).C)
}

func (cmp *channelMutexPair) LockPoll() bool {
	c := make(chan time.Time, 1)
	c <- time.Now()
	return cmp.TimedLock(c)
}

func (cmp *channelMutexPair) TimedLock(tc <-chan time.Time) bool {
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

func (cmp *channelMutexPair) Unlock() bool {
	cmp.abSig <- false
	return true
}
