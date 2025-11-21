package data

import "errors"

func New(name string) *Database {
	return &Database{
		name:    name,
		kvStore: make(map[string]string),
	}
}

func (d *Database) Set(key string, val string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.kvStore[key] = val

	return nil
}

func (d *Database) Get(key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if val, ok := d.kvStore[key]; ok {
		return val, nil
	}

	return "", errors.New("key_not_found")
}

func (d *Database) Del(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.kvStore[key]; !ok {
		return errors.New("key_not_found")
	}

	delete(d.kvStore, key)

	return nil
}
