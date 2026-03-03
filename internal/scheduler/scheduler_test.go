package scheduler_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"slate/internal/scheduler"
)

func TestNewScheduler_DefaultWorkers(t *testing.T) {
	// When workers <= 0, defaults to 4.
	s := scheduler.NewScheduler(0)
	s.Start()
	defer s.Stop()

	// Submit a job and verify it runs — this exercises the default worker count.
	done := make(chan struct{})
	s.Schedule(&scheduler.Activity{
		Job: func() { close(done) },
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run within timeout")
	}
}

func TestSchedule_RunsJob(t *testing.T) {
	s := scheduler.NewScheduler(1)
	s.Start()
	defer s.Stop()

	var ran int32
	done := make(chan struct{})
	s.Schedule(&scheduler.Activity{
		Job: func() {
			atomic.StoreInt32(&ran, 1)
			close(done)
		},
	})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not run within timeout")
	}

	if atomic.LoadInt32(&ran) != 1 {
		t.Error("job flag was not set")
	}
}

func TestSchedule_NilActivityIgnored(t *testing.T) {
	s := scheduler.NewScheduler(1)
	s.Start()
	defer s.Stop()

	// Should not panic or block.
	s.Schedule(nil)

	// Follow up with a real job to confirm the scheduler is still operational.
	done := make(chan struct{})
	s.Schedule(&scheduler.Activity{
		Job: func() { close(done) },
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subsequent job did not run after nil schedule")
	}
}

func TestSchedule_MultipleJobs(t *testing.T) {
	const numJobs = 20
	s := scheduler.NewScheduler(4)
	s.Start()
	defer s.Stop()

	var count int64
	var wg sync.WaitGroup
	wg.Add(numJobs)

	for i := 0; i < numJobs; i++ {
		s.Schedule(&scheduler.Activity{
			Job: func() {
				atomic.AddInt64(&count, 1)
				wg.Done()
			},
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("only %d of %d jobs completed", atomic.LoadInt64(&count), numJobs)
	}
}

func TestStop_WaitsForRunningJobs(t *testing.T) {
	s := scheduler.NewScheduler(1)
	s.Start()

	var completed int32
	blocker := make(chan struct{})

	// This job blocks until we release it.
	s.Schedule(&scheduler.Activity{
		Job: func() {
			<-blocker
			atomic.StoreInt32(&completed, 1)
		},
	})

	// Give the job time to start.
	time.Sleep(10 * time.Millisecond)
	close(blocker)

	// Stop should wait for the job to finish.
	s.Stop()

	if atomic.LoadInt32(&completed) != 1 {
		t.Error("Stop returned before job completed")
	}
}

func TestQueueDepth(t *testing.T) {
	// Use 0 workers so no jobs execute — queue fills up.
	s := scheduler.NewScheduler(0)
	// Don't call Start() so nothing drains the channel.

	depth := s.QueueDepth()
	if depth != 0 {
		t.Errorf("initial QueueDepth = %d, want 0", depth)
	}
}
