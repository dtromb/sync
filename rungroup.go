package sync

import (
	"errors"
	"reflect"
	"time"
)


type RunGroup interface {
	ActiveCount() int
	Execution(execId uint64) (Execution,bool)
	ActiveExecutions() []Execution
	//OpenNotificationChannel(min, max int) <-chan RunGroup
	OpenNotificationCondition(min, max int) (*Cond, *MutexKey)
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
	StartTime() time.Time
	FinishTime() time.Time
	DidError() bool
	DidPanic() bool
	Error() error
	Panic() interface{}
	//OpenNotificationChannel() <-chan ExecutionStatus
	OpenNotificationCondition() (*Cond, *MutexKey)
	RequestYield(status ExecutionStatus, arg interface{}) error
}

type ExecutionContext interface {
	Execution() Execution
	Yield()  (ExecutionStatus,interface{})
}

///

type notify struct {
	activated bool
	execution Execution
	currentCount int
}

type stdRunGroup struct {
	m *Mutex
	//executions []*stdExecution
	executionsById map[uint64]*stdExecution
	notifyMin map[int][]chan RunGroup
	notifyMax map[int][]chan RunGroup
	changeNotify chan *notify 
	nextId uint64
}

func (rg *stdRunGroup) ActiveCount() int {
	key := rg.m.Lock(NewKey)
	defer func() { rg.m.Unlock(key) }()
	return len(rg.executionsById)
}

func (rg *stdRunGroup) Execution(execId uint64) (Execution,bool) {
	key := rg.m.Lock(NewKey)
	defer func() { rg.m.Unlock(key) }()
	if ex, has := rg.executionsById[execId]; has {
		return ex, true
	}
	return nil, false
}

func (rg *stdRunGroup) ActiveExecutions() []Execution {
	key := rg.m.Lock(NewKey)
	defer func() { rg.m.Unlock(key) }()
	res := make([]Execution, 0, len(rg.executionsById))
	for _, x := range rg.executionsById {
		res = append(res, x)
	}
	return res
}

func (rg *stdRunGroup) OpenNotificationCondition(min, max int) (*Cond,*MutexKey) {
	panic("stdRunGroup.OpenNotificationCondition() unimplemented")
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
							case rg.changeNotify <- n:
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
func (rg *stdRunGroup) Spawn(f interface{}) (Execution,error) {
	// The call parameter /f/ must have one of the following types:
	//
	// func()
	// func() error
	// func(ExecutionContext)
	// func(ExecutionContext) error
	
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
		key := rg.m.Lock(NewKey)
		defer rg.m.Unlock(key)
		delete (rg.executionsById,execution.id)
		n := &notify{	
			activated: false,
			execution: execution,
			currentCount: len(rg.executionsById),
		}
		select {
			case rg.changeNotify <- n:
			default:
		}
		cl := execution.listeners 
		execution.listeners = nil
		for cl != nil {
			select {
				case cl.c <- execution.state:
				default:
			}
			cl = cl.nxt
		}
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
	
	// Set up the execution and execution context.
	key := rg.m.Lock(NewKey)
	defer rg.m.Unlock(key)
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

type stdExecution struct {
	id uint64
	start time.Time
	stop time.Time
	state ExecutionStatus
	runGroup *stdRunGroup
	panicObject interface{}
	didPanic bool
	errorObject error
	didError bool
	listeners *eslList
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

func (ex *stdExecution) OpenNotificationCondition() (*Cond, *MutexKey) {
	panic("stdExecution.OpenNotificationCondition() unimplemented")
}

func (ex *stdExecution) RequestYield(status ExecutionStatus, arg interface{}) error {
	return errors.New("stdExecution.RequestYield() unimplemented")
}
	
type stdExecutionContext struct {
	execution Execution
}

func (ctx *stdExecutionContext) Execution() Execution {
	return ctx.execution
}

func (ctx *stdExecutionContext) Yield()  (ExecutionStatus,interface{}) {
	panic("stdExecutionContext.Yield() unimplemented")
}