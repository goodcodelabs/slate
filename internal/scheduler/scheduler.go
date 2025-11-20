package scheduler

func NewScheduler() *Scheduler {
	return &Scheduler{
		activities: make(chan *Activity, 64),
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
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.Run()
	}()
}

func (s *Scheduler) Stop() {
	close(s.activities)
	s.wg.Wait()
}
