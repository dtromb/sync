package sync

type MonitorWait interface {
	Wait() bool
	WaitFor(millis int) (bool, bool)
	Yield() bool
	Release() bool
	Interrupt()
}

type Monitor interface {
	Acquire(reenter MonitorWait) (MonitorWait, bool)
	AcquirePoll(reenter MonitorWait) (MonitorWait, bool)
	AcquireFor(reenter MonitorWait, millis int) (MonitorWait, bool)
	Notify() bool
	NotifyN(n int) bool
	NotifyAll() bool
	Interrupt()
}
