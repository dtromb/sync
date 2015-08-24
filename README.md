# sync
--
    import "github.com/dtromb/sync"



     This project is in early development.

## Usage

#### type Monitor

```go
type Monitor interface {

	// Attempt to acquire the monitor - wait indefinitely.  Second argument is
	// true on success, false if the monitor was closed.
	//
	// To use re-entrace, utilize the following idiom:
	//
	//   previousWait, ok = monitor.Acquire(previousWait)
	//
	Acquire(reenter MonitorWait) (MonitorWait, bool)

	// Attempt to acquire the monitor, and return false in the second argument
	// immediately if it is not available to this thread.
	AcquirePoll(reenter MonitorWait) (MonitorWait, bool)

	// Attempt to acquire the monitor for at least *millis* milliseconds, then
	// return false in second argument if still not available.
	AcquireFor(reenter MonitorWait, millis int) (MonitorWait, bool)

	// Notify a single waiting MonitorWait.  It will return from Wait*(...) as
	// soon as it either re-acquires the monitor or is interrupted.
	Notify() bool

	// Notify exactly *n* waiting MonitorWait.
	NotifyN(n int) bool

	// Notify every waiting MonitorWait.
	NotifyAll() bool

	// Interrupt any pending operations on this Monitor, causing them to return
	// failure.
	Interrupt()
}
```

Monitor provides an implementation of an Aquire/Wait/Notify pattern. It is
re-entrant (recursive mutex functionality), pollable/timable, and transferable.

Please note that dispite the superficial similarity there are important
differences between Java-style monitors and this interface. In particular,
Monitor is not in any way bound to the callers goroutine / thread, and
**interrupted waits will not resume with the monitor acquired** but must be
explicitly re-entered.

#### type MonitorWait

```go
type MonitorWait interface {

	// Atomically release the monitor and enter a wait state.  Will return
	// true after a successful notify/re-acquire, or false if interrupted.
	Wait() bool

	// Atomically release the monitor and enter a wait state for at least
	// *millis* milliseconds.  Will return false if a notify/reacquire does
	// not occur, or interupted before the time elapses.
	WaitFor(millis int) (bool, bool)

	// Atomically release the monitor and immediately re-acquire it.  If other
	// acquires are in progress, the winner will be pseudo-randomly chosen.
	Yield() bool

	// Release the monitor.  No further calls on this object will succeed after
	// this.  Returns true if successful, false if the monitor was already
	// released.
	Release() bool

	// Interrupt any pending wait state or reacquire operation for this
	// MonitorWait, causing it to return false.
	Interrupt()
}
```

MonitorWait is the interface a monitor owner uses to interact with the monitor.
Caution should be used if this object is passed between goroutines -
unexpected/undesirable results may occur if no external synchronization is used.
(For example, a Wait() in goroutine A might fail because B has already entered a
wait state and therefore released the monitor...)

#### type Mutex

```go
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
```

Mutex is a pollable, waitable mutual exclusion facility. It is non-re-entrant
and does not check release callers.

#### func  OpenMutex

```go
func OpenMutex() Mutex
```
OpenMutex opens a single Mutex implementation backed by channel-based
concurrency.

#### type MutexPair

```go
type MutexPair interface {
	Left() Mutex
	Right() Mutex
	Both() Mutex
}
```

MutexPair is a pair of mutexes that can be locked/unlocked atomically. It is
presented as a triple of Mutex interfaces that operate on nonempty subsets of
the pair.

#### func  OpenMutexPair

```go
func OpenMutexPair() MutexPair
```
OpenMutexPair opens a MutexPair implementation backed by channel-based
concurrency. It spawns a single goroutine which coordinates atomic operations
without retry.

#### type Signal

```go
type Signal interface{}
```


#### type SignalWait

```go
type SignalWait interface {
	Target() Signal
	Wait() bool
	WaitFor(millis int) bool
}
```
