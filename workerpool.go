package workerpool

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
)

var capasity string = "Thread Pool Capasity Full"

type (
	// workChannel is passed into the queue for work to be performed.
	workChannel struct {
		work          Worker      // The Work to be performed.
		resultChannel chan string // Used to inform the queue operaion is complete.
	}

	// Pool implements a pool with the specified concurrency level and queue capacity.
	Pool struct {
		shutdownQueueChannel chan string      // Channel used to shut down the queue routine.
		shutdownWorkChannel  chan struct{}    // Channel used to shut down the work routines.
		shutdownWaitGroup    sync.WaitGroup   // The WaitGroup for shutting down existing routines.
		queueChannel         chan workChannel // Channel used to sync access to the queue.
		workChannel          chan Worker      // Channel used to process work.
		queuedWork           int32            // The number of work items queued.
		activeRoutines       int32            // The number of routines active.
		queueCapacity        int32            // The max number of items we can store in the queue.
	}
)

// Worker must be implemented by the object we will perform work on, now.
type Worker interface {
	DoWork(routine int)
}

// init is called when the system is initiated
func init() {
	log.SetPrefix("TRACE: ")
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

// New creates a new Pool
func New(numberOfRoutines int, queueCapacity int32) *Pool {
	pool := Pool{
		shutdownQueueChannel: make(chan string),
		shutdownWorkChannel:  make(chan struct{}),
		queueChannel:         make(chan workChannel),
		workChannel:          make(chan Worker, queueCapacity),
		queuedWork:           0,
		activeRoutines:       0,
		queueCapacity:        queueCapacity,
	}

	// Add the total number of routines to the wait group
	pool.shutdownWaitGroup.Add(numberOfRoutines)

	// Launch the work routines to process work
	for workRoutine := 0; workRoutine < numberOfRoutines; workRoutine++ {
		go pool.workRoutine(workRoutine)
	}

	// Start the queue routine to capture and provide work
	go pool.queueRoutine()

	return &pool
}

// Shutdown will release resources and shutdown all processing.
func (pool *Pool) Shutdown(goRoutine string) (err error) {
	defer catchPanic(&err, goRoutine, "Shutdown")

	logMsg(goRoutine, "Shutdown", "Started")
	logMsg(goRoutine, "Shutdown", "Queue Routine")

	pool.shutdownQueueChannel <- "Down"
	<-pool.shutdownQueueChannel

	close(pool.queueChannel)
	close(pool.shutdownQueueChannel)

	logMsg(goRoutine, "Shutdown", "Shutting Down Work Routines")

	// Close the channel to shut things down.
	close(pool.shutdownWorkChannel)
	pool.shutdownWaitGroup.Wait()

	close(pool.workChannel)

	logMsg(goRoutine, "Shutdown", "Completed")
	return err
}

// PostWork will post work into the WorkPool. This call will block until the Queue routine reports back
// success or failure that the work is in queue.
func (pool *Pool) PostWork(goRoutine string, work Worker) (result string, err error) {
	defer catchPanic(&err, goRoutine, "PostWork")

	workChannel := workChannel{work, make(chan string)}

	defer close(workChannel.resultChannel)

	pool.queueChannel <- workChannel
	result = <-workChannel.resultChannel
	return result, err
}

// QueuedWork will return the number of work items in queue.
func (workPool *Pool) QueuedWork() int32 {
	return atomic.AddInt32(&workPool.queuedWork, 0)
}

// ActiveRoutines will return the number of routines performing work.
func (workPool *Pool) ActiveRoutines() int32 {
	return atomic.AddInt32(&workPool.activeRoutines, 0)
}

// CatchPanic is used to catch any Panic and log exceptions to Stdout. It will also write the stack trace.
func catchPanic(err *error, goRoutine string, functionName string) {
	if r := recover(); r != nil {
		// Capture the stack trace
		buf := make([]byte, 10000)
		runtime.Stack(buf, false)

		logMsgf(goRoutine, functionName, "PANIC Defered [%v] : Stack Trace : %v", r, string(buf))

		if err != nil {
			*err = fmt.Errorf("%v", r)
		}
	}
}

// logMsg is used to write a system message directly to stdout.
func logMsg(goRoutine string, functionName string, message string) {
	log.Printf("%s : %s : %s\n", goRoutine, functionName, message)
}

// logMsgf is used to write a formatted system message directly stdout.
func logMsgf(goRoutine string, functionName string, format string, a ...interface{}) {
	logMsg(goRoutine, functionName, fmt.Sprintf(format, a...))
}

// workRoutine performs the work required by the work pool
func (pool *Pool) workRoutine(workRoutine int) {
	for {
		select {
		// Shutdown the WorkRoutine.
		case <-pool.shutdownWorkChannel:
			logMsg(fmt.Sprintf("WorkRoutine %d", workRoutine), "workRoutine", "Going Down")
			pool.shutdownWaitGroup.Done()
			return

		// There is work in the queue.
		case worker := <-pool.workChannel:
			fmt.Printf("************   Executing work on routine %d ************\n", workRoutine)
			pool.safelyDoWork(workRoutine, worker)
			continue
		}
	}
}

// safelyDoWork executes the user DoWork method.
func (pool *Pool) safelyDoWork(workRoutine int, Worker Worker) {
	defer catchPanic(nil, "WorkRoutine", "SafelyDoWork")
	defer atomic.AddInt32(&pool.activeRoutines, -1)

	// Update the counts
	atomic.AddInt32(&pool.queuedWork, -1)
	atomic.AddInt32(&pool.activeRoutines, 1)

	// Perform the work
	Worker.DoWork(workRoutine)
}

// queueRoutine captures and provides work.
func (pool *Pool) queueRoutine() {
	for {
		select {
		// Shutdown the QueueRoutine.
		case <-pool.shutdownQueueChannel:
			logMsg("Queue", "queueRoutine", "Going Down")
			pool.shutdownQueueChannel <- "Down"
			return

		// Post work to be processed.
		case queueItem := <-pool.queueChannel:
			// If the queue is at capacity don't add it.
			if atomic.AddInt32(&pool.queuedWork, 0) == pool.queueCapacity {
				queueItem.resultChannel <- capasity
				continue
			}

			// Increment the queued work count.
			atomic.AddInt32(&pool.queuedWork, 1)

			// Queue the work for the WorkRoutine to process.
			pool.workChannel <- queueItem.work

			// Tell the caller the work is queued.
			queueItem.resultChannel <- "Queued"
			break
		}
	}
}
