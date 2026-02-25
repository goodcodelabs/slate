package scheduler

import (
	"sync"
)

type Scheduler struct {
	activities chan *Activity
	workers    int
	wg         sync.WaitGroup
}

type Activity struct {
	Job func()
}
