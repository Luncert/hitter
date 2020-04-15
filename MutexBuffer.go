package main

import (
	"bytes"
	"sync"
)

// MutexBuffer ...
type MutexBuffer struct {
	b  *bytes.Buffer
	rw *sync.RWMutex
}

func NewMutexBuffer() *MutexBuffer {
	return &MutexBuffer{&bytes.Buffer{}, &sync.RWMutex{}}
}

func (mb MutexBuffer) Read(p []byte) (int, error) {
	mb.rw.RLock()
	defer mb.rw.RUnlock()
	return mb.b.Read(p)
}

func (mb MutexBuffer) Write(p []byte) (int, error) {
	mb.rw.Lock()
	defer mb.rw.Unlock()
	return mb.b.Write(p)
}
