package monitor

import (
	"sync"
	"sync/atomic"
)

// Task is a unit of work submitted to the WorkerPool.
type Task func()

// WorkerPool manages a fixed pool of goroutines for parallel computation
// (AOI calculation, collision detection, etc.).
type WorkerPool interface {
	// Submit enqueues a task for execution. Blocks if the queue is full.
	Submit(task Task)
	// Wait blocks until all submitted tasks have completed.
	Wait()
	// Stop shuts down the pool after draining the queue.
	Stop()
	// ActiveWorkers returns the number of goroutines in the pool.
	ActiveWorkers() int
	// PendingTasks returns the number of tasks waiting in the queue.
	PendingTasks() int64
}

type workerPool struct {
	queue   chan Task
	wg      sync.WaitGroup
	workers int
	pending atomic.Int64
	once    sync.Once
}

// NewWorkerPool creates a WorkerPool with the given number of workers and
// queue capacity. If workers <= 0, defaults to 1.
func NewWorkerPool(workers, queueSize int) WorkerPool {
	if workers <= 0 {
		workers = 1
	}
	if queueSize <= 0 {
		queueSize = workers * 4
	}
	p := &workerPool{
		queue:   make(chan Task, queueSize),
		workers: workers,
	}
	for i := 0; i < workers; i++ {
		go p.run()
	}
	return p
}

func (p *workerPool) run() {
	for task := range p.queue {
		task()
		p.pending.Add(-1)
		p.wg.Done()
	}
}

func (p *workerPool) Submit(task Task) {
	p.pending.Add(1)
	p.wg.Add(1)
	p.queue <- task
}

func (p *workerPool) Wait() {
	p.wg.Wait()
}

func (p *workerPool) Stop() {
	p.once.Do(func() {
		close(p.queue)
	})
}

func (p *workerPool) ActiveWorkers() int {
	return p.workers
}

func (p *workerPool) PendingTasks() int64 {
	return p.pending.Load()
}
