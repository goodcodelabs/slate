package scheduler

// NewScheduler creates a scheduler with a bounded worker pool.
// workers specifies the number of concurrent goroutines; defaults to 4.
func NewScheduler(workers int) *Scheduler {
	if workers <= 0 {
		workers = 4
	}
	return &Scheduler{
		activities: make(chan *Activity, 64),
		workers:    workers,
	}
}

func (s *Scheduler) Schedule(activity *Activity) {
	if activity == nil {
		return
	}

	s.activities <- activity
}

func (s *Scheduler) Run() {
	for activity := range s.activities {
		if activity == nil || activity.Job == nil {
			return
		}
		activity.Job()
	}
}

func (s *Scheduler) Start() {
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.Run()
		}()
	}
}

func (s *Scheduler) Stop() {
	close(s.activities)
	s.wg.Wait()
}
