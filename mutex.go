package sync

import (
	"fmt"
	"runtime"
	"time"
)

// MutexKey represents a single locker's association with a Mutex.  Users 
// may use MutexKey one-per-goroutine, analogous to thread-based locking,
// or one-per-object, analogous to Java montors.  
type MutexKey struct {
	m *Mutex
	wl *waitList
	id uint32
	count int
	waiting bool
	signal chan bool
	debugStackEnt string
}

var NewKey *MutexKey = nil

// Mutex is a keyed, re-entrant mutual exclusion lock that can be 
// multiplexed with other concurrency features.
type Mutex struct {
	lock chan bool
	ownerLock chan *MutexKey
	owner *MutexKey
	nextId uint32
}

// Create a new mutex.
func NewMutex() *Mutex {
	m := &Mutex{
		nextId: 100,
		lock: make(chan bool, 1),
		ownerLock: make(chan *MutexKey, 1),
	}
	m.lock <- true
	return m
}

func releaseMutex(x *MutexKey) {
	if x.m != nil {
		<-x.m.lock
		defer func() { x.m.lock <- true }()
		if x.m.owner == x {
			x.m.owner = nil
			<-x.m.ownerLock
		}
	}
}

// Creates a new condtion variable coordination point based on this mutex.
func (m *Mutex) CreateCond() *Cond {
	return &Cond{m: m}
}

// Creates a new MutexKey that may be used with this mutex.
func (m *Mutex) NewKey() *MutexKey {
	<-m.lock
	defer func() { m.lock <- true }()
	_, file, line, _ := runtime.Caller(1)
	debugStackEnt := fmt.Sprintf("%s:%d", file, line)
	newKey := &MutexKey{m:m, id: m.nextId, debugStackEnt: debugStackEnt}
	fmt.Printf(" - new key for caller %s\n", debugStackEnt)
	runtime.SetFinalizer(newKey, releaseMutex)
	m.nextId++
	return newKey
}

// Blocks until the MutexKey acquires the mutex.  Call with the constant
// NewKey to generate a new mutex key.   Returns the key.
func (m *Mutex) Lock(k *MutexKey) *MutexKey {
	<-m.lock
	defer func() { 
		select {
			case m.lock <- true:
			default:
		}
	}()
	if k != nil && k.m != m {
		panic("foreign mutex key")
	}
	if k != nil && (k.id != 0 || (m.owner != nil && k.id == m.owner.id)) {
		k.count++
		return k
	}
	if k == nil || k.id == 0 {
		// DEBUG
		// _, file, line, _ := runtime.Caller(1)
		// debugStackEnt := fmt.Sprintf("%s:%d", file, line)
		// k = &MutexKey{m:m, id: m.nextId, debugStackEnt: debugStackEnt}
		// fmt.Printf(" - new key for caller %s\n", debugStackEnt)
		k = &MutexKey{m:m, id: m.nextId}
		runtime.SetFinalizer(k, releaseMutex)
		m.nextId++
	}
	k.count = 0
	m.lock <- true
	m.ownerLock <- k
	<-m.lock
	m.owner = k
	k.count++
	return k
}

// Blocks until the mutex is acquired by the key, or until the amount of
// time indicated by the second argument has passed.  Call with the constant
// NewKey to generate a new key.  Returns the key used, and a boolean indicating
// if the mutex was acquired.
func (m *Mutex) LockWait(k *MutexKey, wait time.Duration) (*MutexKey,bool) {
	timer := time.NewTimer(wait)
	<-m.lock
	defer func() { 
		select {
			case m.lock <- true:
			default:
		}
	}()
	if k != nil && k.m != m {
		panic("foreign mutex key")
	}
	if (k != nil && k.id != 0) || (m.owner != nil && k.id == m.owner.id) {
		k.count++
		return k, true
	}
	if k == nil || k.id == 0 {
		k = &MutexKey{m:m, id: m.nextId}
		runtime.SetFinalizer(k, releaseMutex)
		m.nextId++
	}
	k.count = 0
	m.lock <- true
	select {
		case m.ownerLock <- k: {
			timer.Stop()
			<-m.lock
			m.owner = k
			k.count++
			return k, true
		}
		case <-timer.C: {
			return k, false
		}
	}
}


// Attempt to unlock the mutex using the provided key.  If successful,
// one lock will be removed.  If a thread is locked n times with a key,
// it must be unlocked n times before other lockers may acquire.  Returns
// true if successful.
func (m *Mutex) Unlock(k *MutexKey) bool {
	<-m.lock
	defer func() { 
		select {
			case m.lock <- true:
			default:
		}
	}()
	if k != nil && k.m != m {
		panic("foreign mutex key")
	}
	if k == nil || k.id == 0 {
		return false
	}
	if m.owner != nil && k.id != m.owner.id {
		return false
	}
	k.count--
	if k.count == 0 {
		m.owner = nil
		<-m.ownerLock
	}
	return true
}

// Get the current locking key and lock depth of the mutex.
func (m *Mutex) Query() (bool, *MutexKey, int) {
	<-m.lock
	defer func() { m.lock <- true }()
	if m.owner == nil {
		return false, nil, 0
	}
	return true, m.owner, m.owner.count
}

func (mk *MutexKey) Release() bool {
	return mk.m.Unlock(mk)
}

func (mk *MutexKey) ReleaseAll() bool {
	ok := false
	for {
		if mk.m.Unlock(mk) {
			ok = true
		} else {
			break
		}
	}
	return ok
}