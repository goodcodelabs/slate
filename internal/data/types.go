package data

import "sync"

type Data struct {
	mu sync.Mutex

	store map[string]string
}
