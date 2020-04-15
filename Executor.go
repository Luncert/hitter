package main

import (
	"errors"
)

type Executor struct {
	concurrent    int
	taskQueue     chan task
	stoppedSignal chan bool
	running       bool
}

type task struct {
	callable func(...interface{})
	args     []interface{}
}

// CreateExecutor will return *Executor
func CreateExecutor(concurrentLevel int) (ex *Executor) {
	ex = &Executor{
		concurrent:    concurrentLevel,
		taskQueue:     make(chan task, concurrentLevel),
		stoppedSignal: make(chan bool),
		running:       true,
	}
	ex.init()
	return
}

func (ex *Executor) init() {
	// task dispatcher channel
	go func() {
		running := 0
		wait := make(chan bool, ex.concurrent)

		waitOneFinish := func() {
			<-wait
			running--
		}

		for task := range ex.taskQueue {
			// task channel
			go ex.wrappedCall(task, wait)
			running++
			// if we reaches the concurrency limit (ex.concurrent)
			// we should wait a task to finish
			if running == ex.concurrent {
				waitOneFinish()
			}
		}
		// if there are still some task running
		// wait them
		for running > 0 {
			waitOneFinish()
		}
		close(wait)

		ex.stoppedSignal <- true
	}()
}

func (ex *Executor) wrappedCall(t task, signal chan<- bool) {
	defer func() {
		signal <- true
	}()
	t.callable(t.args...)
}

// Execute a task, if there are too much running task, this function will block
func (ex *Executor) Execute(callable func(...interface{}), args ...interface{}) (err error) {
	if !ex.running {
		err = errors.New("executor has been stopped")
	} else {
		ex.taskQueue <- task{callable: callable, args: args}
	}
	return
}

func (ex *Executor) Stop() {
	close(ex.taskQueue)
	<-ex.stoppedSignal
	ex.running = false
}
