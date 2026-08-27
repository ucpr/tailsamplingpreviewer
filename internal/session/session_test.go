package session

import "testing"

func TestManager_Lifecycle(t *testing.T) {
	m := NewManager()

	if got := m.Snapshot().State; got != StateIdle {
		t.Fatalf("initial state = %v, want IDLE", got)
	}

	if _, err := m.Pause(); err != ErrNotStream {
		t.Fatalf("Pause from IDLE: err = %v, want ErrNotStream", err)
	}
	if _, err := m.Stop(); err != ErrNoneActive {
		t.Fatalf("Stop from IDLE: err = %v, want ErrNoneActive", err)
	}

	snap, err := m.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if snap.State != StateStreaming || snap.SessionID == "" {
		t.Fatalf("unexpected snapshot after Start: %+v", snap)
	}

	if _, err := m.Start(); err != ErrNotIdle {
		t.Fatalf("double Start: err = %v, want ErrNotIdle", err)
	}

	snap, err = m.Pause()
	if err != nil || snap.State != StatePaused {
		t.Fatalf("Pause: snap=%+v err=%v", snap, err)
	}

	if _, err := m.Pause(); err != ErrNotStream {
		t.Fatalf("double Pause: err = %v, want ErrNotStream", err)
	}

	snap, err = m.Resume()
	if err != nil || snap.State != StateStreaming {
		t.Fatalf("Resume: snap=%+v err=%v", snap, err)
	}

	snap, err = m.Stop()
	if err != nil || snap.State != StateIdle {
		t.Fatalf("Stop: snap=%+v err=%v", snap, err)
	}
}

func TestManager_OnChangeNotified(t *testing.T) {
	m := NewManager()
	var got []State
	m.OnChange(func(s Snapshot) { got = append(got, s.State) })

	if _, err := m.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Pause(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Resume(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Stop(); err != nil {
		t.Fatal(err)
	}

	want := []State{StateStreaming, StatePaused, StateStreaming, StateIdle}
	if len(got) != len(want) {
		t.Fatalf("got %v transitions, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("transition[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
