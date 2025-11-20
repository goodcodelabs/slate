package scheduler

import (
	"sync"
)

type Scheduler struct {
	activities chan *Activity
	wg         sync.WaitGroup
}

type Activity struct {
	Job func()
}
