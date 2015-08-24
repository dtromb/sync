# sync
### High-level concurrency for Go.

       This project is in early development

--
    import "github.com/dtromb/sync"


## Usage

#### type ChannelMutex

```go
type ChannelMutex struct {
}
```


#### func (*ChannelMutex) Lock

```go
func (m *ChannelMutex) Lock() bool
```

#### func (*ChannelMutex) LockFor

```go
func (m *ChannelMutex) LockFor(millis int) bool
```

#### func (*ChannelMutex) LockPoll

```go
func (m *ChannelMutex) LockPoll() bool
```

#### func (*ChannelMutex) Unlock

```go
func (m *ChannelMutex) Unlock() bool
```

#### type ChannelMutexPair

```go
type ChannelMutexPair struct {
}
```


#### func (*ChannelMutexPair) Both

```go
func (cmp *ChannelMutexPair) Both() Mutex
```

#### func (*ChannelMutexPair) Left

```go
func (cmp *ChannelMutexPair) Left() Mutex
```

#### func (*ChannelMutexPair) Lock

```go
func (cmp *ChannelMutexPair) Lock() bool
```

#### func (*ChannelMutexPair) LockFor

```go
func (cmp *ChannelMutexPair) LockFor(millis int) bool
```

#### func (*ChannelMutexPair) LockPoll

```go
func (cmp *ChannelMutexPair) LockPoll() bool
```

#### func (*ChannelMutexPair) Right

```go
func (cmp *ChannelMutexPair) Right() Mutex
```

#### func (*ChannelMutexPair) TimedLock

```go
func (cmp *ChannelMutexPair) TimedLock(tc <-chan time.Time) bool
```

#### func (*ChannelMutexPair) Unlock

```go
func (cmp *ChannelMutexPair) Unlock() bool
```

#### type ChannelMutexPairSingleSide

```go
type ChannelMutexPairSingleSide struct {
}
```


#### func (*ChannelMutexPairSingleSide) Lock

```go
func (ss *ChannelMutexPairSingleSide) Lock() bool
```

#### func (*ChannelMutexPairSingleSide) LockFor

```go
func (ss *ChannelMutexPairSingleSide) LockFor(millis int) bool
```

#### func (*ChannelMutexPairSingleSide) LockPoll

```go
func (ss *ChannelMutexPairSingleSide) LockPoll() bool
```

#### func (*ChannelMutexPairSingleSide) TimedLock

```go
func (ss *ChannelMutexPairSingleSide) TimedLock(tc <-chan time.Time) bool
```

#### func (*ChannelMutexPairSingleSide) Unlock

```go
func (ss *ChannelMutexPairSingleSide) Unlock() bool
```

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
	Lock() bool
	LockPoll() bool
	LockFor(millis int) bool
	Unlock() bool
}
```


#### func  OpenMutex

```go
func OpenMutex() Mutex
```

#### type MutexPair

```go
type MutexPair interface {
	Left() Mutex
	Right() Mutex
	Both() Mutex
}
```


#### func  OpenMutexPair

```go
func OpenMutexPair() MutexPair
```

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
