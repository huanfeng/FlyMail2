package task

import (
	"container/heap"
	"sync"
	"time"
)

// queueItem represents an item in the priority queue
type queueItem struct {
	task     *Task
	priority int
	index    int
}

// priorityQueue implements heap.Interface
type priorityQueue []*queueItem

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// Higher priority value means higher priority
	return pq[i].priority > pq[j].priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*queueItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// Queue represents a priority task queue
type Queue struct {
	mu       sync.Mutex
	items    priorityQueue
	notEmpty *sync.Cond
	closed   bool
}

// NewQueue creates a new task queue
func NewQueue() *Queue {
	q := &Queue{
		items: make(priorityQueue, 0),
	}
	q.notEmpty = sync.NewCond(&q.mu)
	heap.Init(&q.items)
	return q
}

// Push adds a task to the queue
func (q *Queue) Push(task *Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrQueueClosed
	}

	priority := getPriorityValue(task.Priority)
	item := &queueItem{
		task:     task,
		priority: priority,
	}

	heap.Push(&q.items, item)
	q.notEmpty.Signal()
	return nil
}

// Pop removes and returns a task from the queue
func (q *Queue) Pop(timeout time.Duration) (*Task, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	deadline := time.Now().Add(timeout)
	for q.items.Len() == 0 && !q.closed {
		if timeout > 0 {
			q.notEmpty.Wait()
			if time.Now().After(deadline) {
				return nil, ErrTimeout
			}
		} else {
			q.notEmpty.Wait()
		}
	}

	if q.closed && q.items.Len() == 0 {
		return nil, ErrQueueClosed
	}

	item := heap.Pop(&q.items).(*queueItem)
	return item.task, nil
}

// Size returns the number of tasks in the queue
func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.items.Len()
}

// Close closes the queue
func (q *Queue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.notEmpty.Broadcast()
}

// getPriorityValue converts Priority to numeric value
func getPriorityValue(p Priority) int {
	switch p {
	case PriorityHigh:
		return 3
	case PriorityNormal:
		return 2
	case PriorityLow:
		return 1
	default:
		return 2
	}
}
