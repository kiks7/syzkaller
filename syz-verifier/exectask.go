// Copyright 2021 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/syzkaller/pkg/rpctype"
	"github.com/google/syzkaller/prog"
)

type EnvDescr int64

const (
	AnyEnvironment EnvDescr = iota
	NewEnvironment
	// TODO: add CleanVMEnvironment support.

	EnvironmentsCount
)

// ExecTask is the atomic analysis entity. Once executed, it could trigger the
// pipeline propagation fof the program.
type ExecTask struct {
	CreationTime   time.Time
	Program        *prog.Prog
	ID             int64
	ExecResultChan ExecResultChan

	priority int // The priority of the item in the queue.
	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

func (t *ExecTask) ToRPC() *rpctype.ExecTask {
	return &rpctype.ExecTask{
		Prog: t.Program.Serialize(),
		ID:   t.ID,
	}
}

var (
	ChanMapMutex           = sync.Mutex{}
	TaskIDToExecResultChan = map[int64]ExecResultChan{}
	TaskCounter            = int64(-1)
)

type ExecResultChan chan *ExecResult

func MakeExecTask(prog *prog.Prog) *ExecTask {
	task := &ExecTask{
		CreationTime:   time.Now(),
		Program:        prog,
		ExecResultChan: make(ExecResultChan),
		ID:             atomic.AddInt64(&TaskCounter, 1),
	}

	ChanMapMutex.Lock()
	defer ChanMapMutex.Unlock()
	TaskIDToExecResultChan[task.ID] = task.ExecResultChan

	return task
}

func DeleteExecTask(task *ExecTask) {
	ChanMapMutex.Lock()
	defer ChanMapMutex.Unlock()
	delete(TaskIDToExecResultChan, task.ID)
}

func GetExecResultChan(taskID int64) ExecResultChan {
	ChanMapMutex.Lock()
	defer ChanMapMutex.Unlock()

	return TaskIDToExecResultChan[taskID]
}

func MakeExecTaskQueue() *ExecTaskQueue {
	return &ExecTaskQueue{
		pq: make(ExecTaskPriorityQueue, 0),
	}
}

// ExecTaskQueue respects the pq.priority. Internally it is a thread-safe PQ.
type ExecTaskQueue struct {
	pq ExecTaskPriorityQueue
}

// PopTask return false if no tasks are available.
func (q *ExecTaskQueue) PopTask() (*ExecTask, bool) {
	if q.pq.Len() == 0 {
		return nil, false
	}

	return heap.Pop(&q.pq).(*ExecTask), true
}

func (q *ExecTaskQueue) PushTask(task *ExecTask) {
	heap.Push(&q.pq, task)
}

func (q *ExecTaskQueue) Len() int {
	return q.pq.Len()
}

// ExecTaskPriorityQueue reused example from https://pkg.go.dev/container/heap
type ExecTaskPriorityQueue []*ExecTask

func (pq ExecTaskPriorityQueue) Len() int { return len(pq) }

func (pq ExecTaskPriorityQueue) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return pq[i].priority > pq[j].priority
}

func (pq ExecTaskPriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *ExecTaskPriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*ExecTask)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *ExecTaskPriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}
