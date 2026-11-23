package jsrt

import (
	"errors"

	"github.com/edge-js/edge-go/js"
)

// ErrNeverSettled is returned by Await when the job queue runs dry while the
// awaited promise is still pending: nothing left can ever settle it.
var ErrNeverSettled = errors.New("the awaited promise never settled")

// asyncState is the realm's share of the asynchronous machinery: the job queue,
// the promises' internal slots and the coroutines of suspended async functions.
type asyncState struct {
	// jobs is the queue of pending jobs (PromiseReactionJob and
	// PromiseResolveThenableJob), run first in, first out.
	jobs []func()
	// current is the coroutine whose goroutine is running interpreter code, nil
	// when it is the host's.
	current *coroutine
	// suspended holds the coroutines parked at an await.
	suspended map[*coroutine]bool
	// hosting counts the Await calls in progress.
	hosting int

	promiseProto, asyncFunctionProto *js.Object
	promiseCtor                      *js.Function
}

func newAsyncState() asyncState {
	return asyncState{
		suspended: map[*coroutine]bool{},
	}
}

// enqueueJob is HostEnqueuePromiseJob: job runs after every job queued before
// it, once the code running now has returned to the job loop.
func (r *Realm) enqueueJob(job func()) {
	r.async.jobs = append(r.async.jobs, job)
}

// runJobs runs queued jobs, including the ones they queue, until none is left.
func (r *Realm) runJobs() {
	for len(r.async.jobs) > 0 {
		job := r.async.jobs[0]
		r.async.jobs[0] = nil
		r.async.jobs = r.async.jobs[1:]
		job()
	}
}
