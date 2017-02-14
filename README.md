# sync
--
    import "github.com/dtromb/sync"

### Enhanced concurrency for Go.

     This project is in development.

## Usage

#### type MutexKey

A MutexKey represents a single locker's association with a mutex.

```go
	// Returns true iff this key currently has the mutex lock.
	func (m *MutexKey) HasLock() bool
	// Forcably releases one lock level held by this mutex key.  Returns
	// true on success, false if the key does not have the lock. 
	func (m *MutexKey) Release() bool 
	// Forcably releases all locks held by this mutex ket.  Returns
	// true on success, false if the key does not have the lock.
	// Use with extreme caution - indirect callers may acquire a lock
	// and assume it is still valid after the call. 
	func (m *MutexKey) ReleaseAll() bool 
```
#### type Mutex

A Mutex provides a keyed, re-entrant mutual exclusion concurrency unit.  

```go
type Mutex struct {
	...
}

// Create a new mutex.
func NewMutex() *Mutex

// Creates a new MutexKey that may be used with this mutex.
func (m *Mutex) NewKey() *MutexKey 

// Blocks until the MutexKey acquires the mutex.  Call with the constant
// NewKey to generate a new mutex key.   Returns the key.
func (m *Mutex) Lock(k *MutexKey) *MutexKey


// Blocks until the mutex is acquired by the key, or until the amount of
// time indicated by the second argument has passed.  Call with the constant
// NewKey to generate a new key.  Returns the key used, and a boolean indicating
// if the mutex was acquired.
func (m *Mutex) LockWait(k *MutexKey, wait time.Duration) (*MutexKey,bool) 

// Attempt to unlock the mutex using the provided key.  If successful,
// one lock will be removed.  If a thread is locked n times with a key,
// it must be unlocked n times before other lockers may acquire.  Returns
// true if successful.
func (m *Mutex) Unlock(k *MutexKey) bool 

// Get the current locking key and lock depth of the mutex.
func (m *Mutex) Query() (bool, *MutexKey, int) 

```
#### type Cond

Cond provides a Mutex-based condition variable coordination point.  

```go

	type Cond struct {
		...
	}
	
	// Blocks until the condition variable is signalled.  Returns non-nil
	// if the key has not previously acquired the mutex lock.
	func (c *Cond) Wait(key *MutexKey) error 
	
	// Blocks until the condition variable is signalled, or until a specified
	// amount of time has passed.  Returns true iff the condition was signalled.
	// Note that if the return value is false, the key *may not have reacquired
	// the mutex* and should test for this with Mutex.Query() before proceeding.
	// Therefore - special care should be taken when combining timed condition 
	// waits with recursive locks.  One recommended pattern is to panic when
	// the lock is broken by a timed wait, and recover() above the first mutex
	// lock (so that code may freely assume it still has the lock after calling
	// WaitTimed() either directly or indirectly.)
	func (c *Cond) WaitTimed(key *MutexKey, wait time.Duration) (bool, error) {
	
	// Notifies and wakes exactly one condition wait.
	func (c *Cond) Notify() 
	
	// Notifies and wakes every current condition wait.  These will execute
	// in turn as the associated mutex becomes available to each.
	func (c *Cond) Broadcast() 
```

#### type RunGroup

RunGroup provides a managed execution interface that can be coordinated with
the other concurrency primitives.

```go

type RunGroup interface {
	// Returns the current number of active Execution.
	ActiveCount() int
	// Returns a particular execution by unique id.  Returns the execution and
	// true if such an Execution exists and is active; nil and false otherwise.
	Execution(execId uint64) (Execution,bool)
	// Returns a slice of all active executions at a point-in-time.
	ActiveExecutions() []Execution
	// Open a Cond which is signalled when the number of active 
	// threads has jst transitioned past the requested minimum or 
	// maximum.  This may be used, for example, to implement a managed 
	// worker-thread set.  Returns the condition, and a mutex key that 
	// may be used to lock the associated mutex before a wait.
	OpenNotificationCondition(min, max int) (*Cond,*MutexKey)
	// Starts a new Execution.  The argument must have one of the 
	// following types:
	//
	// func()
	// func() error
	// func(ExecutionContext)
	// func(ExecutionContext) error
	//
	// If the argument returns error, that error value will be used
	// to set the error status of the Execution when it returns.
	//
	// If the argument takes an ExecutionContext, it will be passed
	// an interface that may be used to coordinate with other goroutines'
	// status change requests via the Yield() function.
	Spawn(f interface{}) (Execution,error)
}

```
#### type ExecutionStatus

ExecutionStatus describes the lifecycle state of a particular Execution at a single 
point in time. 

```go
type ExecutionStatus uint8
const (
	// Invalid / unused state identifier.
	ExecutionStatusInvalid		ExecutionStatus = iota
	// The Execution has been created but not yet started.
	ExecutionStatusNew
	// The Execution is currently running and active.
	ExecutionStatusRunning	
	// The Execution has completed without error.
	ExecutionStatusCompleted
	// The Execution has returned an error status.
	ExecutionStatusError
	// The Execution has has a panic() that was not recover()'d.
	ExecutionStatusPanic
	// The Execution is paused.
	ExecutionStatusPaused
)
```
#### type Execution

Execution describes a managed execution, which corresponds to a 
particular goroutine (sequence) under a single entry point.  It 
is therefore similar to a thread.

```go
type Execution interface {
	// Return the unique id (unique per run-group) of this execution.
	Id() uint64
	// Return the RunGroup this Execution is associated with.
	RunGroup() RunGroup
	// Returns true if the Execution is acitve (running or paused).
	Active() bool
	// Returns the time this Execution was started.
	StartTime() time.Time
	// Returns the time this Execution was finished.
	FinishTime() time.Time
	// Returns true if the Execution terminated with an error status.
	DidError() bool
	// Returns true if the Execution terminated via an unrecovered panic.
	DidPanic() bool
	// Returns the error status flagged by the execution, if any.
	Error() error
	// Returns the value of the unrecovered panic, if any.
	Panic() interface{}
	// Returns a condition that will be signalled when the Execution status
	// changes, and a MutexKey that may be used to acquire the associated 
	// mutex.
	OpenNotificationCondition() (*Cond, *MutexKey)
	// Requests that the Execution transition to a given state.  An 
	// optional argument may be provided.  The running execution may
	// or may not honor this request per implementation.
	RequestYield(status ExecutionStatus, arg interface{}) error
}
```


