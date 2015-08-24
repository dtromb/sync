package sync

type Signal interface{}

type SignalWait interface {
	Target() Signal
	Wait() bool
	WaitFor(millis int) bool
}
