package data

import "errors"

func New() *Data {
	return &Data{
		store: make(map[string]string),
	}
}

func (d *Data) Set(key string, val string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.store[key] = val

	return nil
}

func (d *Data) Get(key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if val, ok := d.store[key]; ok {
		return val, nil
	}

	return "", errors.New("key not found")
}

func (d *Data) Del(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.store[key]; !ok {
		return errors.New("key not found")
	}

	delete(d.store, key)

	return nil
}
