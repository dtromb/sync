package sync

import (
	"reflect"
	"time"
)

// Monitor provides an implementation of an Acquire/Wait/Notify concurrency
// pattern.  It is re-entrant (recursive mutex functionality), pollable/timable,
// and transferable.
//
// Please note that despite the superficial similarity there are important
// differences between Java-style monitors and this interface.  In particular,
// Monitor is not in any way bound to the callers goroutine / thread, and
// **interrupted waits will not resume with the monitor acquired** but must
// be explicitly re-entered.
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

// MonitorWait is the interface a monitor owner uses to interact with the
// monitor.  Caution should be used if this object is passed between
// goroutines - unexpected/undesirable results may occur if no external
// synchronization is used.  (For example, a Wait() in goroutine A might fail
// because B has already entered a wait state and therefore released the
// monitor...)
type MonitorWait interface {

	// Atomically release the monitor and enter a wait state.  Will return
	// true after a successful notify/re-acquire, or false if interrupted.
	// The monitor is held on exit iff the return value is true.
	Wait() bool

	// Atomically release the monitor and enter a wait state for at least
	// *millis* milliseconds.  Will return false if a notify/timeout and
	// reacquire does not occur, or interupted before the time elapses.
	// The monitor is held on exit iff the second return value is true.  The
	// wait exited via a notify (and not a timeout, in particular) iff the
	// first return value is true.
	WaitFor(millis int) (bool, bool)

	// Atomically release the monitor and immediately re-acquire it.  If other
	// acquires are in progress, the winner will be pseudo-randomly chosen.
	// The monitor is held iff the return value is true.
	Yield() bool

	// Release the monitor.  No further calls on this object will succeed after
	// this.  Returns true if successful, false if the monitor was already
	// released.  The monitor is not held by the wait on exit.
	Release() bool

	// Interrupt any pending wait state or reacquire operation for this
	// MonitorWait, causing it to return false.
	Interrupt()
}

type simpleChannelAcquirePair struct {
	wait    *simpleChannelMonitorWait
	monitor *simpleChannelMonitor
}

type simpleChannelMonitor struct {
	owner          *simpleChannelMonitorWait
	mtx            chan bool // buffer: 1
	interrupt      chan bool // buffer: 1
	acquireRequest chan *simpleChannelAcquirePair
	acquireAccept  chan *simpleChannelMonitorWait // buffer: 1
	notifySignal   chan int
}

type simpleChannelMonitorWait struct {
	monitor      *simpleChannelMonitor
	mtx          chan bool // buffer: 1
	sig          chan bool // buffer: 1
	interrupt    chan bool // buffer: 1
	waiting      bool
	depth        int
	reenterDepth int
}

func simpleChannelMonitorMain(acquireRequest chan *simpleChannelAcquirePair,
	acquireAccept chan *simpleChannelMonitorWait) {
	var dummy simpleChannelMonitorWait
	for {

		// Clear the accept channel - this will block until the monitor
		// is free, as draining the accept is the way a wait signals release.
		acquireAccept <- &dummy
		_, ok := <-acquireAccept
		if !ok {
			goto shutdown
		}

		// The monitor is now available.  Grab an interested request, possibly
		// waiting to do so.
		select {
		case req, ok := <-acquireRequest:
			{
				if !ok {
					goto shutdown
				}
				// We are modifying the wait here under the mutex held in
				// its own Acquire*(...).  It will not proceed to return
				// until it is signaled.  The following are all atomic wrt the
				// request objects:
				req.monitor.mtx <- true
				req.wait.monitor = req.monitor
				req.wait.depth++
				req.monitor.owner = req.wait
				acquireAccept <- req.wait

				// Signal wait, unlock, and proceed back up to wait for a release (or close).
				req.wait.sig <- true
				<-req.monitor.mtx
			}
		}
	}
shutdown:
	{
	}
}

// OpenMonitor creates a Monitor backed by channel concurrency with goroutine 'daemon'
// providing synchronization.
func OpenMonitor() Monitor {
	m := &simpleChannelMonitor{
		interrupt:      make(chan bool, 1),
		acquireRequest: make(chan *simpleChannelAcquirePair),
		acquireAccept:  make(chan *simpleChannelMonitorWait, 1),
		notifySignal:   make(chan int),
	}
	go simpleChannelMonitorMain(m.acquireRequest, m.acquireAccept)
	return m
}

func (m *simpleChannelMonitor) Acquire(reenter MonitorWait) (MonitorWait, bool) {
	return m.AcquireTimed(false, nil, reenter)
}

func (m *simpleChannelMonitor) AcquirePoll(reenter MonitorWait) (MonitorWait, bool) {
	return m.AcquireTimed(true, nil, reenter)
}

func (m *simpleChannelMonitor) AcquireFor(reenter MonitorWait, millis int) (MonitorWait, bool) {
	return m.AcquireTimed(false, time.NewTimer(time.Millisecond*time.Duration(millis)).C, reenter)
}

func (m *simpleChannelMonitor) AcquireTimed(poll bool, tc <-chan time.Time, reenter MonitorWait) (MonitorWait, bool) {
	if reenter != nil {
		if rmw, ok := reenter.(*simpleChannelMonitorWait); ok {
			// Recursive entry.
			rmw.mtx <- true
			if rmw.monitor != m {
				if rmw.monitor != nil {
					<-rmw.mtx
					return nil, false
				}
			} else {
				if m.owner == rmw {
					rmw.depth++
					<-rmw.mtx
					return rmw, true
				}
				rmw.depth = 0
			}
			<-rmw.mtx
		}
	}

	// We will attempt to acquire.
	// Create a new Monitor Wait if we are not re-using an old one.
	var wait *simpleChannelMonitorWait
	if rmw, ok := reenter.(*simpleChannelMonitorWait); ok {
		wait = rmw
	} else {
		wait = &simpleChannelMonitorWait{
			mtx:       make(chan bool, 1),
			sig:       make(chan bool, 1),
			interrupt: make(chan bool, 1),
		}
	}

	// Lock the wait.
	wait.mtx <- true
	defer func() { <-wait.mtx }()

	// Drain the signal in case of a bad state - we don't want a spurious
	// signal delivered later.
	select {
	case <-wait.sig:
		{
		}
	default:
		{
		}
	}
	wait.depth = 0
	wait.monitor = m
	request := &simpleChannelAcquirePair{
		monitor: m,
		wait:    wait,
	}

	// Construct the select call.  We use reflection so that
	// we can include a default case or not, depending on the poll parameter.
	// This differs from a zero-length timer because the latter would be
	// included in the pseudo-random ready selection, instead of a subordinate
	// last-case.
	cases := make([]reflect.SelectCase, 3)
	cases[0] = reflect.SelectCase{
		Dir:  reflect.SelectSend,
		Chan: reflect.ValueOf(m.acquireRequest),
		Send: reflect.ValueOf(request),
	}
	cases[1] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(m.interrupt),
	}
	cases[2] = reflect.SelectCase{
		Dir:  reflect.SelectRecv,
		Chan: reflect.ValueOf(wait.interrupt),
	}
	if poll {
		cases[3] = reflect.SelectCase{
			Dir: reflect.SelectDefault,
		}
	} else {
		cases[3] = reflect.SelectCase{
			Dir:  reflect.SelectRecv,
			Chan: reflect.ValueOf(tc),
		}
	}

	scase, val, ok := reflect.Select(cases)

	switch scase {
	//case m.acquireRequest <- request: {
	case 0:
		{
			<-wait.sig
			if wait.monitor == m {
				return wait, true
			}
			return nil, false
		}
	// case val, ok := <- m.interrupt: {
	case 1:
		{
			if ok {
				// Chain the interrupt along.
				select {
				case m.interrupt <- val.Bool():
					{
					}
				default:
					{
					}
				}
			}
			return nil, false
		}
	case 2:
		{
			if ok {
				// Chain the interrupt along.
				select {
				case wait.interrupt <- val.Bool():
					{
					}
				default:
					{
					}
				}
			}
			return nil, false
		}
	// case <- tc: {
	// default:
	case 3:
		{
			// Timed out.
			return nil, false
		}
	}
	return nil, false
}

func (m *simpleChannelMonitor) NotifyN(n int) bool {
	m.mtx <- true
	defer func() { <-m.mtx }()
	select {
	case m.notifySignal <- n:
		{
			return true
		}
	default:
		{
			return false
		}
	}
}

func (m *simpleChannelMonitor) Notify() bool {
	return m.NotifyN(1)
}

func (m *simpleChannelMonitor) NotifyAll() bool {
	var c int
	m.mtx <- true
	defer func() { <-m.mtx }()
	for {
		select {
		case m.notifySignal <- 1:
			{
				c++
				continue
			}
		default:
			{
				return c > 0
			}
		}
	}
}

func (m *simpleChannelMonitor) Interrupt() {
	select {
	case m.interrupt <- true:
		{
		}
	default:
		{
		}
	}
}

func (w *simpleChannelMonitorWait) Wait() bool {
	_, restored := w.TimedWait(nil)
	return restored
}

func (w *simpleChannelMonitorWait) WaitFor(millis int) (bool, bool) {
	return w.TimedWait(time.NewTimer(time.Duration(millis) * time.Millisecond).C)
}

func (w *simpleChannelMonitorWait) Yield() bool {
	_, restored := w.TimedWait(time.NewTimer(time.Duration(0) * time.Millisecond).C)
	return restored
}

func (w *simpleChannelMonitorWait) TimedWait(tc <-chan time.Time) (bool, bool) {

	// First we must acquire the wait mutex.  This might block if the wait was
	// copied or passed to other goroutines (a horrible idea), and is currently
	// doing Something Else (maybe acquiring a totally different mutex by the
	// time we are here!).

	select {
	case w.mtx <- true:
		{
		} // Success.
	case val, ok := <-w.interrupt:
		{
			if ok {
				w.interrupt <- val
			} // Chain the interrupt forward.
			if w.monitor != nil {
				return false, w.monitor.owner == w
			}
			return false, false
		}
	case <-tc:
		{
			return false, w.monitor.owner == w
		}
	}

	// Now we are in the wait lock.  The timer is still running, if active...
	defer func() { <-w.mtx }()
	if w.monitor == nil || w.monitor.owner != w {
		return false, false
	}

	// To transition to the wait state, we must release the monitor and enter
	// notify contention atomically (from the caller's perspective).  Since
	// the wait lock covers any returns to active state (commonly/hopefully
	// later in this function!), locking the monitor mutex will suffice.
	// However, this might cause a deadlock unless we always lock the /wait lock/
	// first, and then the /monitor lock/ the wait references after.  So, that's
	// the rule, everywhere that such a pair needs to be acquired...

	// This is exactly like above.
	select {
	case w.monitor.mtx <- true:
		{
		}
	case val, ok := <-w.interrupt:
		{
			if ok {
				w.interrupt <- val
			}
			return false, w.monitor.owner == w
		}
	case <-tc:
		{
			return false, w.monitor.owner == w
		}
	}

	// At this point we have locked both the wait and the monitor.  We are ready
	// to release the monitor and enter a notify-listen state.
	oldDepth := w.depth
	w.monitor.owner = nil
	w.depth--
	<-w.monitor.acquireAccept
	w.waiting = true
	<-w.monitor.mtx

	// It's true that a notify could deliver *right here* and we could miss it.
	// But that's not ordered with regard to what has happened since the function entered.
	// So, it doesn't matter.
	var notified bool
	select {
	case n, ok := <-w.monitor.notifySignal:
		{
			if !ok {
				return false, false // No recovery of ownership on close/interrupt!  Interrupt means exactly that.
			}
			notified = true
			// We've received notification.  We will enter a resume-acquire state,
			// but first pay the notification forward if nec'y.
			if n > 1 {
				select {
				case w.monitor.notifySignal <- (n - 1):
					{
					}
				default:
					{
					} // oh well, we tried?   XXX - No good.  Buffer these, maybe?
				}
			}
		}
	case val, ok := <-w.interrupt:
		{
			if ok {
				w.interrupt <- val
			}
			return false, false
		}
	case val, ok := <-w.monitor.interrupt:
		{
			if ok {
				w.monitor.interrupt <- val
			}
			return false, false
		}
	case <-tc:
		{
		} // This way out also leads to the resume-acquire.
	}

	// Now we must wait for re-acquire.
	request := &simpleChannelAcquirePair{
		wait:    w,
		monitor: w.monitor,
	}
	select {
	case w.monitor.acquireRequest <- request:
		{
			// Restore the state and return success.
			<-w.sig
			w.waiting = false
			w.depth = oldDepth
			return notified, true
		}
	case val, ok := <-w.monitor.interrupt:
		{
			w.reenterDepth = oldDepth
			if ok {
				select {
				case w.monitor.interrupt <- val:
					{
					}
				default:
					{
					}
				}
			}
			return notified, false
		}
	}
}

func (w *simpleChannelMonitorWait) Release() bool {

	// Lock the wait.
	select {
	case w.mtx <- true:
		{
		} // Success.
	case val, ok := <-w.interrupt:
		{
			if ok {
				w.interrupt <- val
			} // Chain the interrupt forward.
			if w.monitor != nil {
				return false
			}
		}
	}
	defer func() { <-w.mtx }()
	if w.monitor == nil || w.monitor.owner != w {
		return false
	}
	w.depth--
	if w.depth > 0 {
		return true
	}
	w.reenterDepth = 0

	// Lock the monitor.
	select {
	case w.monitor.mtx <- true:
		{
		} // Success.
	case val, ok := <-w.monitor.interrupt:
		{
			if ok {
				w.monitor.interrupt <- val
			} // Chain the interrupt forward.
			w.depth++
			return false
		}
	}

	// Release the monitor.
	w.monitor.owner = nil
	<-w.monitor.acquireAccept
	<-w.monitor.mtx
	return true
}

func (w *simpleChannelMonitorWait) Interrupt() {
	select {
	case w.interrupt <- true:
		{
		}
	default:
		{
		}
	}
}
