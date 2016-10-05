package supervisor

import (
	"fmt"
	"io/ioutil"
	"log"
	"sync"
	"testing"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func TestString(t *testing.T) {
	t.Parallel()

	const expected = "test"
	var supervisor Supervisor
	supervisor.Name = expected

	if got := fmt.Sprintf("%s", &supervisor); got != expected {
		t.Errorf("error getting supervisor name: %s", got)
	}
}

func TestStringDefaultName(t *testing.T) {
	t.Parallel()

	const expected = "supervisor"
	var supervisor Supervisor
	supervisor.prepare()

	if got := fmt.Sprintf("%s", &supervisor); got != expected {
		t.Errorf("error getting supervisor name: %s", got)
	}
}

func TestCascaded(t *testing.T) {
	t.Parallel()

	var supervisor Supervisor
	supervisor.Name = "TestCascaded root"
	svc1 := waitservice{id: 1}
	supervisor.Add(&svc1)
	svc2 := waitservice{id: 2}
	supervisor.Add(&svc2)

	var childSupervisor Supervisor
	childSupervisor.Name = "TestCascaded child"
	svc3 := waitservice{id: 3}
	childSupervisor.Add(&svc3)
	svc4 := waitservice{id: 4}
	childSupervisor.Add(&svc4)
	supervisor.Add(&childSupervisor)

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)
	for svc1.Count() == 0 || svc3.Count() == 0 {
	}

	cancel()

	if count := getServiceCount(&supervisor); count != 3 {
		t.Errorf("unexpected service count: %v", count)
	}

	switch {
	case svc1.count != 1, svc2.count != 1, svc3.count != 1, svc4.count != 1:
		t.Errorf("services should have been executed only once. %d %d %d %d",
			svc1.count, svc2.count, svc3.count, svc4.count)
	}
}

func TestLog(t *testing.T) {
	t.Parallel()

	supervisor := Supervisor{
		Name: "TestLog",
	}
	svc1 := panicservice{id: 1}
	supervisor.Add(&svc1)

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)
	for svc1.Count() == 0 {
	}

	cancel()

}

func TestPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Supervisor{
		Name: "TestPanic supervisor",
		Log: func(msg interface{}) {
			t.Log("supervisor log (panic):", msg)
		},
	}
	svc1 := panicservice{id: 1}
	supervisor.Add(&svc1)

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)
	for svc1.Count() == 0 {
	}
	cancel()
	if svc1.count == 0 {
		t.Errorf("the failed service should have been started at least once. Got: %d", svc1.count)
	}
}

func TestRemovePanicService(t *testing.T) {
	t.Parallel()

	supervisor := Group{
		Supervisor: &Supervisor{
			Name: "TestRemovePanicService supervisor",
			Log: func(msg interface{}) {
				t.Log("supervisor log (panic bug):", msg)
			},
		},
	}

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)

	svc1 := waitservice{id: 1}
	supervisor.Add(&svc1)
	svc2 := quickpanicservice{id: 2}
	supervisor.Add(&svc2)

	supervisor.Remove(svc2.String())
	cancel()

	svcs := supervisor.Services()
	if _, ok := svcs[svc2.String()]; ok {
		t.Errorf("%s should have been removed.", &svc2)
	}
}

func TestFailing(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Supervisor{
		Name: "TestFailing supervisor",
		Log: func(msg interface{}) {
			t.Log("supervisor log (failing):", msg)
		},
	}
	svc1 := failingservice{id: 1}
	supervisor.Add(&svc1)

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)

	for svc1.Count() == 0 {
	}

	cancel()

	if svc1.count != 1 {
		t.Errorf("the failed service should have been started just once. Got: %d", svc1.count)
	}
}

func TestAlwaysRestart(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Supervisor{
		Name:        "TestAlwaysRestart supervisor",
		MaxRestarts: AlwaysRestart,
		Log: func(msg interface{}) {
			t.Log("supervisor log (always restart):", msg)
		},
	}
	svc1 := failingservice{id: 1}
	supervisor.Add(&svc1)

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)

	for svc1.Count() < 2 {
	}

	cancel()
}

func TestHaltAfterFailure(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Supervisor{
		Name:        "TestHaltAfterFailure supervisor",
		MaxRestarts: 1,
		Log: func(msg interface{}) {
			t.Log("supervisor log (halt after failure):", msg)
		},
	}
	svc1 := failingservice{id: 1}
	supervisor.Add(&svc1)

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)

	for svc1.Count() == 0 {
	}

	cancel()

	if svc1.count != 1 {
		t.Errorf("the failed service should have been started just once. Got: %d", svc1.count)
	}
}

func TestHaltAfterPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Supervisor{
		Name:        "TestHaltAfterPanic supervisor",
		MaxRestarts: AlwaysRestart,
		Log: func(msg interface{}) {
			t.Log("supervisor log (halt after panic):", msg)
		},
	}

	ctx, cancel := contextWithCancel()

	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()
	svc1.Wait()

	svc2 := &panicabortsupervisorservice{id: 2, cancel: cancel, supervisor: &supervisor}
	supervisor.Add(svc2)

	wg.Wait()

	if svc1.count > 1 {
		t.Error("the holding service should have not been started more than once.")
	}
}

func TestTerminationAfterPanic(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Supervisor{
		Name:        "TestTerminationAfterPanic supervisor",
		MaxRestarts: AlwaysRestart,
		Log: func(msg interface{}) {
			t.Log("supervisor log (termination after panic):", msg)
		},
	}
	svc1 := &triggerpanicservice{id: 1}
	supervisor.Add(svc1)
	svc2 := &holdingservice{id: 2}
	svc2.Add(1)
	supervisor.Add(svc2)

	ctx, cancel := contextWithCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()
	svc2.Wait()

	svc3 := &holdingservice{id: 3}
	svc3.Add(1)
	supervisor.Add(svc3)
	svc3.Wait()

	supervisor.Remove(svc1.String())

	svc4 := &holdingservice{id: 4}
	svc4.Add(1)
	supervisor.Add(svc4)
	svc4.Wait()

	cancel()

	wg.Wait()

	if svc1.count > 1 {
		t.Error("the panic service should have not been started more than once.")
	}
}

func TestMaxRestart(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := &Supervisor{
		Name:        "TestMaxRestart supervisor",
		MaxRestarts: 1,
		Log: func(msg interface{}) {
			t.Log("supervisor log (max restart):", msg)
		},
	}
	svc1 := failingservice{id: 1}
	supervisor.Add(&svc1)

	ctx, _ := contextWithCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()

	wg.Wait()

	if svc1.count > 1 {
		t.Error("the panic service should have not been started more than once.")
	}
}

func TestGroupMaxRestart(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Group{
		Supervisor: &Supervisor{
			Name:        "TestMaxRestartGroup supervisor",
			MaxRestarts: 1,
			Log: func(msg interface{}) {
				t.Log("supervisor log (max restart group):", msg)
			},
		},
	}
	svc1 := failingservice{id: 1}
	supervisor.Add(&svc1)

	ctx, _ := contextWithCancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()

	wg.Wait()

	if svc1.count > 1 {
		t.Error("the panic service should have not been started more than once.")
	}
}

func TestAddServiceAfterServe(t *testing.T) {
	t.Parallel()

	var supervisor Supervisor
	supervisor.Name = "TestAddServiceAfterServe supervisor"
	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)

	ctx, cancel := contextWithCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()

	svc1.Wait()

	svc2 := &holdingservice{id: 2}
	svc2.Add(1)
	supervisor.Add(svc2)
	svc2.Wait()

	cancel()
	<-ctx.Done()
	wg.Wait()

	if count := getServiceCount(&supervisor); count != 2 {
		t.Errorf("unexpected service count: %v", count)
	}
}

func TestRemoveServiceAfterServe(t *testing.T) {
	t.Parallel()

	var supervisor Supervisor
	supervisor.Name = "TestRemoveServiceAfterServe supervisor"
	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)
	svc2 := &holdingservice{id: 2}
	svc2.Add(1)
	supervisor.Add(svc2)

	ctx, cancel := contextWithCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()

	lbefore := getServiceCount(&supervisor)
	supervisor.Remove("unknown service")
	lafter := getServiceCount(&supervisor)

	if lbefore != lafter {
		t.Error("the removal of an unknown service shouldn't happen")
	}

	svc1.Wait()
	svc2.Wait()

	supervisor.Remove(svc1.String())
	lremoved := getServiceCount(&supervisor)
	if lbefore == lremoved {
		t.Error("the removal of a service should have affected the supervisor:", lbefore, lremoved)
	}

	cancel()
	<-ctx.Done()
	wg.Wait()
}

func TestRemoveServiceAfterServeBug(t *testing.T) {
	t.Parallel()

	var supervisor Supervisor
	supervisor.Name = "TestRemoveServiceAfterServeBug supervisor"
	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)

	svc1.Wait()
	supervisor.Remove(svc1.String())
	cancel()

	if svc1.count > 1 {
		t.Errorf("the removal of a service should have terminated it. It was started %v times", svc1.count)
	}
}

func TestServices(t *testing.T) {
	t.Parallel()

	var supervisor Supervisor
	supervisor.Name = "TestServices supervisor"
	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)
	svc2 := &holdingservice{id: 2}
	svc2.Add(1)
	supervisor.Add(svc2)

	ctx, cancel := contextWithCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
	}()
	svc1.Wait()
	svc2.Wait()

	svcs := supervisor.Services()
	for _, svcname := range []string{svc1.String(), svc2.String()} {
		if _, ok := svcs[svcname]; !ok {
			t.Errorf("expected service not found: %s", svcname)
		}
	}

	cancel()
	<-ctx.Done()
	wg.Done()
}

func TestManualCancelation(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()

	supervisor := Supervisor{
		Name: "TestManualCancelation supervisor",
		Log: func(msg interface{}) {
			t.Log("supervisor log (restartable):", msg)
		},
	}
	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)
	svc2 := restartableservice{id: 2, restarted: make(chan struct{})}
	supervisor.Add(&svc2)

	ctx, cancel := contextWithCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()

	svc1.Wait()

	// Testing restart
	<-svc2.restarted
	svcs := supervisor.Cancelations()
	svcancel := svcs[svc2.String()]
	svcancel()
	<-svc2.restarted

	cancel()
	<-ctx.Done()
	wg.Wait()
}

func TestServiceList(t *testing.T) {
	t.Parallel()

	var supervisor Supervisor
	supervisor.Name = "TestServiceList supervisor"
	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)

	ctx, cancel := contextWithCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
	}()
	svc1.Wait()

	svcs := supervisor.Services()
	if svc, ok := svcs[svc1.String()]; !ok || svc1 != svc.(*holdingservice) {
		t.Errorf("could not find service when listing them. %s missing", svc1.String())
	}

	cancel()
	<-ctx.Done()
	wg.Done()
}

func TestValidGroup(t *testing.T) {
	t.Parallel()

	supervisor := &Group{
		Supervisor: &Supervisor{
			Name: "TestValidGroup supervisor",
			Log: func(msg interface{}) {
				t.Log("group log:", msg)
			},
		},
	}
	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)
	t.Log("supervisor started")

	trigger1 := make(chan struct{})
	listening1 := make(chan struct{})
	svc1 := &triggerfailservice{id: 1, trigger: trigger1, listening: listening1, log: t.Logf}
	supervisor.Add(svc1)
	t.Log("svc1 added")

	trigger2 := make(chan struct{})
	listening2 := make(chan struct{})
	svc2 := &triggerfailservice{id: 2, trigger: trigger2, listening: listening2, log: t.Logf}
	supervisor.Add(svc2)
	t.Log("svc2 added")

	<-listening1
	<-listening2
	trigger1 <- struct{}{}

	<-listening1
	<-listening2

	if !(svc1.count == svc2.count && svc1.count == 1) {
		t.Errorf("both services should have the same start count")
	}

	t.Log("stopping supervisor")
	cancel()
}

func TestInvalidGroup(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("defer called, but not because of panic")
		}
	}()
	var group Group
	ctx, _ := contextWithCancel()
	group.Serve(ctx)
	t.Error("this group is invalid and should have had panic()'d")
}

func TestSupervisorAbortRestart(t *testing.T) {
	t.Parallel()
	supervisor := Supervisor{
		Name: "TestAbortRestart supervisor",
		Log: func(msg interface{}) {
			t.Log("supervisor log (abort restart):", msg)
		},
	}

	svc1 := &holdingservice{id: 1}
	svc1.Add(1)
	supervisor.Add(svc1)
	svc2 := &restartableservice{id: 2}
	supervisor.Add(svc2)

	ctx, cancel := contextWithCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()
	svc1.Wait()

	for svc2.Count() < 3 {
	}

	cancel()
	wg.Wait()

	if svc2.count < 3 {
		t.Errorf("the restartable service should have been started twice. Got: %d", svc2.count)
	}
}

func TestTemporaryService(t *testing.T) {
	t.Parallel()
	supervisor := Supervisor{
		Name: "TestTemporaryService supervisor",
		Log: func(msg interface{}) {
			t.Log("supervisor log (termination abort restart):", msg)
		},
	}

	ctx, cancel := contextWithCancel()
	go supervisor.Serve(ctx)
	svc1 := &temporaryservice{id: 1}
	supervisor.AddService(svc1, Temporary)

	for svc1.Count() < 1 {
	}

	svc2 := &temporaryservice{id: 2}
	supervisor.AddService(svc2, Temporary)
	cancel()

	if svc1.count != 1 {
		t.Error("the temporary service should have been started just once.", svc1.count)
	}
}

func TestTransientService(t *testing.T) {
	t.Parallel()
	supervisor := Supervisor{
		Name: "TestTemporaryService supervisor",
		Log: func(msg interface{}) {
			t.Log("supervisor log (termination abort restart):", msg)
		},
	}

	svc1 := &transientservice{id: 1}
	svc1.Add(1)
	supervisor.AddService(svc1, Transient)
	svc2 := &holdingservice{id: 2}
	svc2.Add(1)
	supervisor.Add(svc2)

	ctx, cancel := contextWithCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		supervisor.Serve(ctx)
		wg.Done()
	}()
	svc1.Wait()
	svc2.Wait()

	cancel()
	wg.Wait()

	if svc1.count != 2 {
		t.Error("the transient service should have been started just twice.")
	}
}

func getServiceCount(s *Supervisor) int {
	s.mu.Lock()
	l := len(s.services)
	s.mu.Unlock()
	return l
}
