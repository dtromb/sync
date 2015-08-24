# sync
--
    import "github.com/dtromb/sync"

     This project is in early development.

## Usage

#### type Monitor

```go
type Monitor interface {
	Acquire(reenter MonitorWait) (MonitorWait, bool)
	AcquirePoll(reenter MonitorWait) (MonitorWait, bool)
	AcquireFor(reenter MonitorWait, millis int) (MonitorWait, bool)
	Notify() bool
	NotifyN(n int) bool
	NotifyAll() bool
	Interrupt()
}
```


#### type MonitorWait

```go
type MonitorWait interface {
	Wait() bool
	WaitFor(millis int) (bool, bool)
	Yield() bool
	Release() bool
	Interrupt()
}
```


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
