package testhelper

import (
	"context"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestExtendT(t *testing.T) {
	mt := new(mockT)

	extended := ExtendT(mt)

	if _, ok := extended.(*tWrapper); !ok {
		t.Errorf("ExtendT should return *tWrapper, got %T", extended)
	}

	wrapper := extended.(*tWrapper)
	if wrapper.T != mt {
		t.Errorf("wrapped T is not the same as input")
	}
	if len(wrapper.stack) != 0 {
		t.Errorf("initial stack should be empty, got %d items", len(wrapper.stack))
	}
}

func TestTWrapper_PushContext(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(mt)

	extended := wrapper.PushContext("key1", "value1")
	wrapper2 := extended.(*tWrapper)

	if len(wrapper.stack) != 0 {
		t.Errorf("original wrapper stack should not be modified")
	}
	if len(wrapper2.stack) != 1 {
		t.Errorf("new wrapper should have 1 stack item, got %d", len(wrapper2.stack))
	}
	if wrapper2.stack[0].K != "key1" || wrapper2.stack[0].V != "value1" {
		t.Errorf("stack item mismatch: expected key1=value1, got %s=%v", wrapper2.stack[0].K, wrapper2.stack[0].V)
	}

	// Test chaining
	extended2 := wrapper2.PushContext("key2", 42)
	wrapper3 := extended2.(*tWrapper)

	if len(wrapper2.stack) != 1 {
		t.Errorf("wrapper2 stack should not be modified")
	}
	if len(wrapper3.stack) != 2 {
		t.Errorf("wrapper3 should have 2 stack items, got %d", len(wrapper3.stack))
	}
	if wrapper3.stack[1].K != "key2" || wrapper3.stack[1].V != 42 {
		t.Errorf("second stack item mismatch: expected key2=42, got %s=%v", wrapper3.stack[1].K, wrapper3.stack[1].V)
	}
}

func TestTWrapper_PushOp(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt))

	extended := wrapper.PushOp("TestOperation")
	wrapper2 := extended.(*tWrapper)

	if len(wrapper2.stack) != 1 {
		t.Errorf("wrapper should have 1 stack item, got %d", len(wrapper2.stack))
	}
	if wrapper2.stack[0].K != "Op" || wrapper2.stack[0].V != "TestOperation" {
		t.Errorf("stack item mismatch: expected Op=TestOperation, got %s=%v", wrapper2.stack[0].K, wrapper2.stack[0].V)
	}
}

func TestTWrapper_PushPath(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt))

	path := "/test/path/file.txt"
	extended := wrapper.PushPath(path)
	wrapper2 := extended.(*tWrapper)

	if len(wrapper2.stack) != 1 {
		t.Errorf("wrapper should have 1 stack item, got %d", len(wrapper2.stack))
	}
	if wrapper2.stack[0].K != "Path" || wrapper2.stack[0].V != path {
		t.Errorf("stack item mismatch: expected Path=%s, got %s=%v", path, wrapper2.stack[0].K, wrapper2.stack[0].V)
	}
}

func TestTWrapper_RunE(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt))
	wrapper = wrapper.PushContext("InitialContext", "value1").(*tWrapper)

	called := false
	var receivedT ExtendedT
	var receivedStack []keyValue

	result := wrapper.RunE("subtest", func(t ExtendedT) {
		called = true
		receivedT = t
		if tw, ok := t.(*tWrapper); ok {
			receivedStack = slices.Clone(tw.stack)
		}
	})

	if !called {
		t.Errorf("RunE function not called")
	}
	if !result {
		t.Errorf("RunE should return true when Run succeeds")
	}
	if !mt.runCalled {
		t.Errorf("underlying T.Run not called")
	}
	if mt.runName != "subtest" {
		t.Errorf("Run called with wrong name: expected subtest, got %s", mt.runName)
	}
	if receivedT == nil {
		t.Errorf("received ExtendedT is nil")
	}
	if len(receivedStack) != 1 || receivedStack[0].K != "InitialContext" {
		t.Errorf("stack not properly cloned to subtest")
	}
}

func TestTWrapper_msgPrefix(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt))

	// Empty stack
	if prefix := wrapper.msgPrefix(); prefix != "\n\n" {
		t.Errorf("empty stack should produce \\n\\n, got %q", prefix)
	}

	// Single item
	wrapper = wrapper.PushContext("%Key1%", "%Value1%").(*tWrapper)
	prefix := wrapper.msgPrefix()
	expected := `
%%Key1%%: %%Value1%%

`
	if prefix != expected {
		t.Errorf("single item prefix mismatch: expected %q, got %q", expected, prefix)
	}

	// Multiple items
	wrapper = wrapper.PushContext("Key2", 42).PushPath("/some/path").(*tWrapper)
	prefix = wrapper.msgPrefix()
	expected = `
%%Key1%%: %%Value1%%
Key2: 42
Path: /some/path

`
	if prefix != expected {
		t.Errorf("multiple items prefix mismatch: expected %q, got %q", expected, prefix)
	}
}

func TestTWrapper_Error(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Error").(*tWrapper)

	wrapper.Error("test error message")

	if !mt.errorCalled {
		t.Errorf("Error not called on underlying T")
	}
	// The tWrapper passes arguments as a slice, so we should have exactly 1 arg containing a slice
	if len(mt.errorArgs) != 1 {
		t.Errorf("Error should have 1 arg (slice), got %d: %v", len(mt.errorArgs), mt.errorArgs)
		return
	}
	argsSlice, ok := mt.errorArgs[0].([]any)
	if !ok {
		t.Errorf("Error arg should be a slice, got %T: %v", mt.errorArgs[0], mt.errorArgs[0])
		return
	}
	if len(argsSlice) != 2 {
		t.Errorf("Error args slice should have 2 elements (prefix + message), got %d: %v", len(argsSlice), argsSlice)
		return
	}
	if !strings.Contains(argsSlice[0].(string), "Test: Error") {
		t.Errorf("Error prefix not included: %v", argsSlice[0])
	}
	if argsSlice[1] != "test error message" {
		t.Errorf("Error message mismatch: expected 'test error message', got %v", argsSlice[1])
	}
}

func TestTWrapper_Errorf(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Errorf").(*tWrapper)

	wrapper.Errorf("error %d: %s", 42, "test")

	if !mt.errorfCalled {
		t.Errorf("Errorf not called on underlying T")
	}
	expectedFormat := `
Test: Errorf

error %d: %s`
	if mt.errorfFormat != expectedFormat {
		t.Errorf("Errorf format mismatch: expected %q, got %q", expectedFormat, mt.errorfFormat)
	}
	if len(mt.errorfArgs) != 2 || mt.errorfArgs[0] != 42 || mt.errorfArgs[1] != "test" {
		t.Errorf("Errorf args mismatch: %v", mt.errorfArgs)
	}
}

func TestTWrapper_Fatal(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Fatal").(*tWrapper)

	// Run Fatal in a separate goroutine to handle runtime.Goexit()
	done := make(chan bool)
	go func() {
		defer func() {
			done <- true
		}()
		wrapper.Fatal("fatal error")
	}()

	<-done

	if !mt.fatalCalled {
		t.Errorf("Fatal not called on underlying T")
	}
	if len(mt.fatalArgs) != 1 {
		t.Errorf("Fatal should have 1 arg (slice), got %d: %v", len(mt.fatalArgs), mt.fatalArgs)
		return
	}
	argsSlice, ok := mt.fatalArgs[0].([]any)
	if !ok {
		t.Errorf("Fatal arg should be a slice, got %T: %v", mt.fatalArgs[0], mt.fatalArgs[0])
		return
	}
	if len(argsSlice) != 2 {
		t.Errorf("Fatal args slice should have 2 elements (prefix + message), got %d: %v", len(argsSlice), argsSlice)
		return
	}
	if !strings.Contains(argsSlice[0].(string), "Test: Fatal") {
		t.Errorf("Fatal prefix not included: %v", argsSlice[0])
	}
	if argsSlice[1] != "fatal error" {
		t.Errorf("Fatal message mismatch: expected 'fatal error', got %v", argsSlice[1])
	}
}

func TestTWrapper_Fatalf(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Fatalf").(*tWrapper)

	// Run Fatalf in a separate goroutine to handle runtime.Goexit()
	done := make(chan bool)
	go func() {
		defer func() {
			done <- true
		}()
		wrapper.Fatalf("fatal %d: %s", 99, "error")
	}()

	<-done

	if !mt.fatalfCalled {
		t.Errorf("Fatalf not called on underlying T")
	}
	expectedFormat := `
Test: Fatalf

fatal %d: %s`
	if mt.fatalfFormat != expectedFormat {
		t.Errorf("Fatalf format mismatch: expected %q, got %q", expectedFormat, mt.fatalfFormat)
	}
	if len(mt.fatalfArgs) != 2 || mt.fatalfArgs[0] != 99 || mt.fatalfArgs[1] != "error" {
		t.Errorf("Fatalf args mismatch: %v", mt.fatalfArgs)
	}
}

func TestTWrapper_Log(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Log").(*tWrapper)

	wrapper.Log("log message")

	if !mt.logCalled {
		t.Errorf("Log not called on underlying T")
	}
	if len(mt.logArgs) != 1 {
		t.Errorf("Log should have 1 arg (slice), got %d: %v", len(mt.logArgs), mt.logArgs)
		return
	}
	argsSlice, ok := mt.logArgs[0].([]any)
	if !ok {
		t.Errorf("Log arg should be a slice, got %T: %v", mt.logArgs[0], mt.logArgs[0])
		return
	}
	if len(argsSlice) != 2 {
		t.Errorf("Log args slice should have 2 elements (prefix + message), got %d: %v", len(argsSlice), argsSlice)
		return
	}
	if !strings.Contains(argsSlice[0].(string), "Test: Log") {
		t.Errorf("Log prefix not included: %v", argsSlice[0])
	}
	if argsSlice[1] != "log message" {
		t.Errorf("Log message mismatch: expected 'log message', got %v", argsSlice[1])
	}
}

func TestTWrapper_Logf(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Logf").(*tWrapper)

	wrapper.Logf("log %d: %s", 123, "info")

	if !mt.logfCalled {
		t.Errorf("Logf not called on underlying T")
	}
	expectedFormat := `
Test: Logf

log %d: %s`
	if mt.logfFormat != expectedFormat {
		t.Errorf("Logf format mismatch: expected %q, got %q", expectedFormat, mt.logfFormat)
	}
	if len(mt.logfArgs) != 2 || mt.logfArgs[0] != 123 || mt.logfArgs[1] != "info" {
		t.Errorf("Logf args mismatch: %v", mt.logfArgs)
	}
}

func TestTWrapper_Skip(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Skip").(*tWrapper)

	// Run Skip in a separate goroutine to handle runtime.Goexit()
	done := make(chan bool)
	go func() {
		defer func() {
			done <- true
		}()
		wrapper.Skip("skip reason")
	}()

	<-done

	if !mt.skipCalled {
		t.Errorf("Skip not called on underlying T")
	}
	if len(mt.skipArgs) != 1 {
		t.Errorf("Skip should have 1 arg (slice), got %d: %v", len(mt.skipArgs), mt.skipArgs)
		return
	}
	argsSlice, ok := mt.skipArgs[0].([]any)
	if !ok {
		t.Errorf("Skip arg should be a slice, got %T: %v", mt.skipArgs[0], mt.skipArgs[0])
		return
	}
	if len(argsSlice) != 2 {
		t.Errorf("Skip args slice should have 2 elements (prefix + message), got %d: %v", len(argsSlice), argsSlice)
		return
	}
	if !strings.Contains(argsSlice[0].(string), "Test: Skip") {
		t.Errorf("Skip prefix not included: %v", argsSlice[0])
	}
	if argsSlice[1] != "skip reason" {
		t.Errorf("Skip message mismatch: expected 'skip reason', got %v", argsSlice[1])
	}
}

func TestTWrapper_Skipf(t *testing.T) {
	mt := new(mockT)
	wrapper := wrapT(T(mt)).PushContext("Test", "Skipf").(*tWrapper)

	// Run Skipf in a separate goroutine to handle runtime.Goexit()
	done := make(chan bool)
	go func() {
		defer func() {
			done <- true
		}()
		wrapper.Skipf("skip %d: %s", 456, "test")
	}()

	<-done

	if !mt.skipfCalled {
		t.Errorf("Skipf not called on underlying T")
	}
	expectedFormat := `
Test: Skipf

skip %d: %s`
	if mt.skipfFormat != expectedFormat {
		t.Errorf("Skipf format mismatch: expected %q, got %q", expectedFormat, mt.skipfFormat)
	}
	if len(mt.skipfArgs) != 2 || mt.skipfArgs[0] != 456 || mt.skipfArgs[1] != "test" {
		t.Errorf("Skipf args mismatch: %v", mt.skipfArgs)
	}
}

// mockT implements T interface for testing
type mockT struct {
	// Track method calls
	chdirCalled   bool
	cleanupCalled bool
	errorCalled   bool
	errorfCalled  bool
	fatalCalled   bool
	fatalfCalled  bool
	logCalled     bool
	logfCalled    bool
	runCalled     bool
	skipCalled    bool
	skipfCalled   bool

	// Capture arguments
	chdirDir     string
	cleanupFuncs []func()
	errorArgs    []any
	errorfFormat string
	errorfArgs   []any
	fatalArgs    []any
	fatalfFormat string
	fatalfArgs   []any
	logArgs      []any
	logfFormat   string
	logfArgs     []any
	runName      string
	runFunc      func(*testing.T)
	skipArgs     []any
	skipfFormat  string
	skipfArgs    []any
	setenvKey    string
	setenvValue  string

	// Return values
	contextReturn  context.Context
	deadlineReturn time.Time
	deadlineOk     bool
	failed         bool
	name           string
	skipped        bool
	tempDir        string
}

func (m *mockT) Chdir(dir string) {
	m.chdirCalled = true
	m.chdirDir = dir
}

func (m *mockT) Cleanup(f func()) {
	m.cleanupCalled = true
	m.cleanupFuncs = append(m.cleanupFuncs, f)
}

func (m *mockT) Context() context.Context {
	if m.contextReturn == nil {
		return context.Background()
	}
	return m.contextReturn
}

func (m *mockT) Deadline() (deadline time.Time, ok bool) {
	return m.deadlineReturn, m.deadlineOk
}

func (m *mockT) Error(args ...any) {
	m.errorCalled = true
	m.errorArgs = args
}

func (m *mockT) Errorf(format string, args ...any) {
	m.errorfCalled = true
	m.errorfFormat = format
	m.errorfArgs = args
}

func (m *mockT) Fail() {
	m.failed = true
}

func (m *mockT) FailNow() {
	m.failed = true
	runtime.Goexit()
}

func (m *mockT) Failed() bool {
	return m.failed
}

func (m *mockT) Fatal(args ...any) {
	m.fatalCalled = true
	m.fatalArgs = args
	m.failed = true
	runtime.Goexit()
}

func (m *mockT) Fatalf(format string, args ...any) {
	m.fatalfCalled = true
	m.fatalfFormat = format
	m.fatalfArgs = args
	m.failed = true
	runtime.Goexit()
}

func (m *mockT) Helper() {}

func (m *mockT) Log(args ...any) {
	m.logCalled = true
	m.logArgs = args
}

func (m *mockT) Logf(format string, args ...any) {
	m.logfCalled = true
	m.logfFormat = format
	m.logfArgs = args
}

func (m *mockT) Name() string {
	if m.name == "" {
		return "MockTest"
	}
	return m.name
}

func (m *mockT) Parallel() {}

func (m *mockT) Run(name string, f func(t *testing.T)) bool {
	m.runCalled = true
	m.runName = name
	m.runFunc = f
	// Create a minimal testing.T-like object for the callback
	subT := &testing.T{}
	f(subT)
	return true
}

func (m *mockT) Setenv(key string, value string) {
	m.setenvKey = key
	m.setenvValue = value
}

func (m *mockT) Skip(args ...any) {
	m.skipCalled = true
	m.skipArgs = args
	m.skipped = true
	runtime.Goexit()
}

func (m *mockT) SkipNow() {
	m.skipped = true
	runtime.Goexit()
}

func (m *mockT) Skipf(format string, args ...any) {
	m.skipfCalled = true
	m.skipfFormat = format
	m.skipfArgs = args
	m.skipped = true
	runtime.Goexit()
}

func (m *mockT) Skipped() bool {
	return m.skipped
}

func (m *mockT) TempDir() string {
	if m.tempDir == "" {
		return "/tmp/mocktest"
	}
	return m.tempDir
}
