package sync

import (
	"reflect"
	//"errors"
	//"reflect"
	"time"
	logging "github.com/dtromb/log"
	"github.com/dtromb/errors"
)


var log = logging.Logger("sync")

type RunGroup interface {
	Keyable
	ActiveCount() int
	Mutex() *Mutex
	Execution(execId uint64) (Execution,bool)
	ActiveExecutions() []Execution
	StartStopCondition() *Cond
	Spawn(f interface{}) (Execution,error)
}

type ExecutionStatus uint8
const (
	ExecutionStatusInvalid		ExecutionStatus = iota
	ExecutionStatusNew
	ExecutionStatusRunning	
	ExecutionStatusCompleted
	ExecutionStatusError
	ExecutionStatusPanic
	ExecutionStatusPaused
)
func (es ExecutionStatus) Active() bool {
	switch(es) {
		case ExecutionStatusRunning: return true
		case ExecutionStatusPaused: return true
	}
	return false
}

type Execution interface {
	Id() uint64
	RunGroup() RunGroup
	Active() bool
	Status() ExecutionStatus
	StartTime() time.Time
	FinishTime() time.Time
	DidError() bool
	DidPanic() bool
	Error() error
	Panic() interface{}
	StatusCondition() *Cond
	RequestYield(status ExecutionStatus, arg interface{}) error
}

type ExecutionContext interface {
	Execution() Execution
	Yield()  (ExecutionStatus,interface{})
}

///

type notify struct {
}

type stdRunGroup struct {
	m *Mutex
	startStop *Cond
	activeExecutions map[uint64]Execution
	nextExecutionId uint64
}

type runGroupInterface struct {
	*stdRunGroup
	self RunGroup
}

type keyedRunGroup struct {
	runGroupInterface
	key *MutexKey
}

func NewRunGroup() RunGroup {
	m := NewMutex()
	return NewRunGroupMutex(m)
}

func NewRunGroupMutex(m *Mutex) RunGroup {
	rg := &stdRunGroup{
		m: m,
		startStop: m.CreateCond(),
		activeExecutions: make(map[uint64]Execution),
		nextExecutionId: 1000,
	}
	rgi := &runGroupInterface{
		stdRunGroup: rg,
	}
	rgi.self = rgi
	return rgi
}

func (rgi *runGroupInterface) Key(k *MutexKey) interface{} {
	krg := &keyedRunGroup{
		runGroupInterface: runGroupInterface{
			stdRunGroup: rgi.stdRunGroup,
		},
		key: k,
	}
	krg.self = krg
	return krg
}

func (krg *keyedRunGroup) GetKey() *MutexKey {
	return krg.key
}

func (rg *runGroupInterface) usekey() *MutexKey {
	if keyed, ok := rg.self.(Keyed); ok {
		return keyed.GetKey()
	}
	return NewKey
}

func (rgi *runGroupInterface) ActiveCount() int {
	key := rgi.m.Lock(rgi.usekey())
	defer rgi.m.Unlock(key)
	return len(rgi.activeExecutions)
}

func (rgi *runGroupInterface) Mutex() *Mutex {
	return rgi.m
}

func (rgi *runGroupInterface) Execution(execId uint64) (Execution,bool) {
	key := rgi.m.Lock(rgi.usekey())
	defer rgi.m.Unlock(key)
	if exec, has := rgi.activeExecutions[execId]; has {
		return exec, true
	}
	return nil, false
}

func (rgi *runGroupInterface) ActiveExecutions() []Execution {
	key := rgi.m.Lock(rgi.usekey())
	defer rgi.m.Unlock(key)
	res := make([]Execution, 0, len(rgi.activeExecutions))
	for _, exec := range rgi.activeExecutions {
		res = append(res, exec)
	}
	return res
}

func (rgi *runGroupInterface) StartStopCondition() *Cond {
	key := rgi.m.Lock(rgi.usekey())
	defer rgi.m.Unlock(key)
	return rgi.startStop
}

/*
func (rg *stdRunGroup) OpenNotificationCondition(min, max int) (*Cond,*MutexKey) {
}

/*
func (rg *stdRunGroup) OpenNotificationChannel(min, max int) <-chan RunGroup {
	key := rg.m.Lock(NewKey)
	defer rg.m.Unlock(key) 
	chanKey := rg.m.NewKey()
	nChan := make(chan RunGroup, 1)
	chanHandler := func() {
		for {
			select {
				case _, ok := <-nChan: { // Detect channel close by consumer
					rg.m.Lock(chanKey)
					defer rg.m.Unlock(chanKey)
					if m, has := rg.notifyMin[min]; has {
						for i := 0; i < len(m); i++ {
							if m[i] == nChan {
								nm := append(m[0:i],m[i+1:]...)
								rg.notifyMin[min] = nm 
								break
							}
						}
					}
					if m, has := rg.notifyMax[max]; has {
						for i := 0; i < len(m); i++ {
							if m[i] == nChan {
								nm := append(m[0:i],m[i+1:]...)
								rg.notifyMax[max] = nm
								break
							}
						}
					}
					return
				}
				case n := <-rg.changeNotify: { // an execution has gone active or inactive
					// First, spool this change out to all other listeners.
					for {
						select {
							case rg.
							
							
							 <- n:
							default:
						}
					}
					// All the listeners are woke.  Check the condition.
					if (min >= 0 || n.currentCount < min) ||
					   (max >= 0 || n.currentCount > max) {
						// We raise the notify signal even if something has changed
						// the active count concurrently - it is up to listeners to
						// acquire the group lock and check the condition is still true!
						select {
							case nChan <- rg:
							default:
						}
					}
				}
			}
		}
	}
	// Insert the handler into the listener extents and start it.
	if min >= 0 {
		if m, has := rg.notifyMin[min]; has {
			rg.notifyMin[min] = append(m, nChan)
		} else {
			rg.notifyMin[min] = []chan RunGroup{nChan}
		}
	}	
	if max >= 0 {
		if m, has := rg.notifyMin[max]; has {
			rg.notifyMin[max] = append(m, nChan)
		} else {
			rg.notifyMin[max] = []chan RunGroup{nChan}
		}
	}
	go chanHandler()
	return nChan
}
*/

type stdExecution struct {
	id uint64					// a unique id for this execution (per-rungroup)
	start time.Time				// the time the execution started
	stop time.Time				// the time the execution stopper
	state ExecutionStatus		// the current state of the execution
	runGroup RunGroup			// the rungroup that this execution belongs to
	panicObject interface{}		// the object that the execution panic()'d on, if any
	didPanic bool				// true iff the execution panic()'d
	errorObject error			// the error that the execution returned, if any
	didError bool				// true iff the execution returned an error
	key *MutexKey 				// the key used by the execution to acquire the rungroup
	statusCond *Cond			// a condition signalled when the execution status changes (uses rungroup mutex)
}

type stdExecutionContext struct {
	execution Execution
}

func (rgi *runGroupInterface) Spawn(f interface{}) (Execution,error) {	
    // The call parameter /f/ must have one of the following types:
	//
	// func()
	// func() error
	// func(ExecutionContext)
	// func(ExecutionContext) error
	log.Trace("In Spawn()")
	
	// Acquire the rungroup.
	key := rgi.m.Lock(rgi.usekey())
	defer rgi.m.Unlock(key)
	
	// Create the new execution and its context.
	execution := &stdExecution{
		runGroup: rgi,
		id: rgi.nextExecutionId,
		state: ExecutionStatusNew,
		statusCond: rgi.m.CreateCond(),
	}
	rgi.nextExecutionId++
	ctx := &stdExecutionContext{
		execution: execution,
	}
	
	// Introspect the argument type to create /thunk/, a known-type entry point.
	fType := reflect.TypeOf(f)
	if fType.Kind() != reflect.Func {
		return nil, errors.New("Spawn() argument was not a function")
	}
	var thunk func(ExecutionContext) error
	// This is the function /thunk/ will call first on starting, to 
	// change the rungroup state to reflect the new execution coming alive.
	startup := func() {	
		// Acquire the rungroup.
		execution.key = rgi.m.Lock(NewKey)
		defer rgi.m.Unlock(execution.key)
		// Set the run status and rungroup state.
		execution.state = ExecutionStatusRunning	
		execution.start = time.Now()
		rgi.activeExecutions[execution.id] = execution
		// Broadcast a change to the rungroup active count.
		rgi.startStop.Broadcast()
		// Broadcast the execution status change.
		execution.statusCond.Broadcast()
	}
	// This is the function /thunk/ will call during return, to
	// change the rungroup state to reflect the new execution finishing.
	parachute := func(err error) {
		// Update the execution state, setting any error conditions necessary.
		execution.stop = time.Now()
		log.Debugf("Execution parachute deployed at %s",execution.stop.String())
		r := recover()
		execution.state = ExecutionStatusCompleted
		if err != nil {
			execution.errorObject = err
			execution.didError = true
			execution.state = ExecutionStatusError
		}
		if r != nil {
			execution.panicObject = r
			execution.didPanic = true
			execution.state = ExecutionStatusPanic
		}
		// Acquire the rungroup to update its state.
		log.Debugf(" locking run group to finish execution")
		rgi.m.Lock(execution.key)
		defer rgi.m.Unlock(execution.key)
		// Remove this execution from the active set.
		delete (rgi.activeExecutions, execution.id)
		// Broadcast the start/stop condition.
		rgi.startStop.Broadcast()
		// Broadcast the execution status change.
		execution.statusCond.Broadcast()
		log.Debugf(" done, parachute closing")
	}
	// Construct the proper call wrapper based on the input argument type.
	if fType.NumIn() == 0 {
		if fType.NumOut() == 0 {   // TypeOf(f) == func()
			callFunc := f.(func()) 
			thunk = func(ExecutionContext) error { 
				defer parachute(nil)
				startup()
				callFunc()
				return nil
			}
		} else {
			if fType.NumOut() > 1 { // TypeOf(f) == func() error
				return nil, errors.New("Spawn() argument has multiple return values")
			}
			if !fType.Out(0).AssignableTo(reflect.TypeOf([]error{}).Elem()) {
				return nil, errors.New("Spawn() argument return type was not error")
			}
			callFuncValue := reflect.ValueOf(f)
			thunk = func(ExecutionContext) (err error) {
				defer parachute(err)
				startup()
				rv := callFuncValue.Call([]reflect.Value{})
				err = rv[0].Interface().(error)
				return err
			}
		}
	} else {
		if fType.NumIn() > 1 { 
			return nil, errors.New("Spawn() argument has more than one input parameter")
		}
		if !reflect.TypeOf([]ExecutionContext{}).Elem().AssignableTo(fType.In(0)) {
			return nil, errors.New("Spawn() argument parameter is not ExecutionContext")
		}
		if fType.NumOut() == 0 { // TypeOf(f) == func(ExecutionContext)
			callFuncValue := reflect.ValueOf(f)
			thunk = func(ExecutionContext) error {
				defer parachute(nil)
				startup()
				callFuncValue.Call([]reflect.Value{reflect.ValueOf(ctx)})
				return nil
			}
		} else { // TypeOf(f) == func(ExecutionContext) error
			if fType.NumOut() > 1 {
					return nil, errors.New("Spawn() argument has multiple return values")
			}
			if !fType.Out(0).AssignableTo(reflect.TypeOf([]error{}).Elem()) {
				return nil, errors.New("Spawn() argument return type was not error")
			}
			callFuncValue := reflect.ValueOf(f)
			thunk = func(ExecutionContext) (err error) {
				defer parachute(err)
				startup()
				rv := callFuncValue.Call([]reflect.Value{reflect.ValueOf(ctx)})
				err = rv[0].Interface().(error)
				return err
			}
		}
	}
	log.Trace("prepared execution wrapper, launching")
	
	// Start up the execution and return.
	go thunk(ctx)
	return execution, nil
}

/*
func (rg *stdRunGroup) Spawn(f interface{}, keys ...*MutexKey) (Execution,error) {
	// The call parameter /f/ must have one of the following types:
	//
	// func()
	// func() error
	// func(ExecutionContext)
	// func(ExecutionContext) error
	var key *MutexKey
	if len(keys) > 1 {
		panic("more than one key provided to Spawn()")
	}
	if len(keys) == 1 {
		key = keys[0]
	}
	log.Trace("In Spawn()")
	execution := &stdExecution{runGroup: rg}
	ctx := &stdExecutionContext{}
	fType := reflect.TypeOf(f)
	if fType.Kind() != reflect.Func {
		return nil, errors.New("Spawn() argument was not a function")
	}
	var thunk func(ExecutionContext) error
	startup := func() {
		execution.state = ExecutionStatusRunning		
		key := rg.m.Lock(NewKey)
		defer rg.m.Unlock(key)
		rg.executionsById[execution.id] = execution
		n := &notify{	
			activated: true,
			execution: execution,
			currentCount: len(rg.executionsById),
		}
		select {
			case rg.changeNotify <- n:
			default:
		}
	}
	parachute := func(err error) {
		execution.stop = time.Now()
		log.Debugf("Execution parachute deployed at %s",execution.stop.String())
		r := recover()
		execution.state = ExecutionStatusCompleted
		if err != nil {
			execution.errorObject = err
			execution.didError = true
			execution.state = ExecutionStatusError
		}
		if r != nil {
			execution.panicObject = r
			execution.didPanic = true
			execution.state = ExecutionStatusPanic
		}
		
		log.Debugf(" locking run group to finish execution")
		key := rg.m.Lock(NewKey)
		defer rg.m.Unlock(key)
		log.Debugf(" done locking run group to finish execution")
		delete (rg.executionsById,execution.id)
		n := &notify{	
			activated: false,
			execution: execution,
			currentCount: len(rg.executionsById),
		}
		
		log.Debugf(" sending changeNotify to rg channel")
		select {
			case rg.changeNotify <- n:
			default:
		}
		cl := execution.listeners 
		execution.listeners = nil
		log.Debugf(" sending execNotify to exec channels")
		for cl != nil {
			select {
				case cl.c <- execution.state:
				default:
			}
			cl = cl.nxt
		}
		log.Debugf(" done, parachute closing")
	}
	
	// Construct the proper call wrapper based on the input argument type.
	if fType.NumIn() == 0 {
		if fType.NumOut() == 0 {
			callFunc := f.(func()) 
			thunk = func(ExecutionContext) error { 
				defer parachute(nil)
				startup()
				callFunc()
				return nil
			}
		} else {
			if fType.NumOut() > 1 {
				return nil, errors.New("Spawn() argument has multiple return values")
			}
			if !fType.Out(0).AssignableTo(reflect.TypeOf([]error{}).Elem()) {
				return nil, errors.New("Spawn() argument return type was not error")
			}
			callFuncValue := reflect.ValueOf(f)
			thunk = func(ExecutionContext) (err error) {
				defer parachute(err)
				startup()
				rv := callFuncValue.Call([]reflect.Value{})
				err = rv[0].Interface().(error)
				return err
			}
		}
	} else {
		if fType.NumIn() > 1 {
			return nil, errors.New("Spawn() argument has more than one input parameter")
		}
		if !reflect.TypeOf([]ExecutionContext{}).Elem().AssignableTo(fType.In(0)) {
			return nil, errors.New("Spawn() argument parameter is not ExecutionContext")
		}
		if fType.NumOut() == 0 {
			callFuncValue := reflect.ValueOf(f)
			thunk = func(ExecutionContext) error {
				defer parachute(nil)
				startup()
				callFuncValue.Call([]reflect.Value{reflect.ValueOf(ctx)})
				return nil
			}
		} else {
			if fType.NumOut() > 1 {
					return nil, errors.New("Spawn() argument has multiple return values")
			}
			if !fType.Out(0).AssignableTo(reflect.TypeOf([]error{}).Elem()) {
				return nil, errors.New("Spawn() argument return type was not error")
			}
			callFuncValue := reflect.ValueOf(f)
			thunk = func(ExecutionContext) (err error) {
				defer parachute(err)
				startup()
				rv := callFuncValue.Call([]reflect.Value{reflect.ValueOf(ctx)})
				err = rv[0].Interface().(error)
				return err
			}
		}
	}
	log.Trace("prepared execution: rungroup key")
	// Set up the execution and execution context.
	if key == nil {
		key = rg.m.Lock(NewKey)
	} else {
		rg.m.Lock(key)
	}
	defer rg.m.Unlock(key)
	log.Trace("locked run group mutex")
	execution.id = rg.nextId
	execution.start = time.Now()
	rg.nextId++
	ctx.execution = execution
	
	// Startup the goroutine and return the execution object.
	go thunk(ctx)
	return execution, nil
}

type eslList struct {
	c chan ExecutionStatus
	nxt *eslList
	prv *eslList
}
*/

func (ex *stdExecution) Status() ExecutionStatus {
	return ex.state
}

func (ex *stdExecution) Id() uint64 {
	return ex.id
}

func (ex *stdExecution) RunGroup() RunGroup {
	return ex.runGroup
}

func (ex *stdExecution) Active() bool {
	return ex.state.Active()
}

func (ex *stdExecution) StartTime() time.Time {
	return ex.start
}

func (ex *stdExecution) FinishTime() time.Time {
	return ex.stop
}

func (ex *stdExecution) DidError() bool {
	return ex.didError
}

func (ex *stdExecution) DidPanic() bool {
	return ex.didPanic
}

func (ex *stdExecution) Error() error {
	return ex.errorObject
}

func (ex *stdExecution) Panic() interface{} {
	return ex.panicObject
}

func (ex *stdExecution) StatusCondition() *Cond {
	return ex.statusCond
}

func (ex *stdExecution) RequestYield(status ExecutionStatus, arg interface{}) error {
	return errors.New("stdExecution.RequestYield() unimplemented")
}

func (ctx *stdExecutionContext) Execution() Execution {
	return ctx.execution
}

func (ctx *stdExecutionContext) Yield()  (ExecutionStatus,interface{}) {
	panic("stdExecutionContext.Yield() unimplemented")
}