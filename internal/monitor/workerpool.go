package monitor

import (
	"sync"
	"sync/atomic"
)

// Task 是提交给 WorkerPool 的工作单元。
type Task func()

// WorkerPool 管理固定数量的 goroutine 池，用于并行计算（AOI 计算、碰撞检测等）。
type WorkerPool interface {
	// Submit 将任务加入队列等待执行。队列满时阻塞。
	Submit(task Task)
	// Wait 阻塞直到所有已提交的任务完成。
	Wait()
	// Stop 在排空队列后关闭 Worker 池。
	Stop()
	// ActiveWorkers 返回池中的 goroutine 数量。
	ActiveWorkers() int
	// PendingTasks 返回队列中等待执行的任务数量。
	PendingTasks() int64
}
type workerPool struct {
	queue   chan Task
	wg      sync.WaitGroup
	workers int
	pending atomic.Int64
	once    sync.Once
}

// NewWorkerPool 创建指定 Worker 数量和队列容量的 WorkerPool。
// workers <= 0 时默认为 1。
// NewWorkerPool 创建指定 Worker 数量和队列容量的 WorkerPool。
// workers <= 0 时默认为 1。
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

(task Task) {
	p.pending.Add(1)
	p.wg.Add(1)
	p.queue <- task
}

func (p *workerPool) Wait() {
	p.wg.Wait()
}


	p.once.Do(func() {
		close(p.queue)
	})
}

func (p *workerPool) ActiveWorkers() int {
	return p.workers
}

4 {
	return p.pending.Load()
}
