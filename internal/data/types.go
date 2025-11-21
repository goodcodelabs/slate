package data

import "sync"

type Database struct {
	mu sync.Mutex

	name    string
	kvStore map[string]string
}

type Core struct {
	Databases []*Database
}
