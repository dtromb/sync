package sync

import (
	"testing"
	logging "github.com/dtromb/log"
)

var testlog logging.Log = logging.Logger("test")

func TestKeyed(t *testing.T) {
	logging.GetGlobalLoggingContext().EnableDebugging(true)
	rg := NewRunGroup()
	rg.ActiveCount()
	key := rg.Mutex().NewKey()
	krg := rg.(Keyable).Key(key).(RunGroup)
	krg.ActiveCount()
}