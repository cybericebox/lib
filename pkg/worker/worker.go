package worker

import (
	"github.com/gofrs/uuid"
	"github.com/rs/zerolog/log"
	"slices"
	"strings"
	"sync"
	"time"
)

type (
	status int32

	taskWorker struct {
		m           sync.RWMutex
		maxWorkers  int
		throttle    time.Duration
		inputTasks  chan task
		queuedTasks []task
		toDoTasks   chan task
		doneTasks   chan task
	}

	Worker interface {
		AddTask(t task)
	}

	task struct {
		key string // key to identify Task
		do  func() error
		// checkIfNeedToDo returns if it needs to do now, not nil timeToDo is time to do Task if not now
		checkIfNeedToDo func() (need bool, nextTimeToDo *time.Time, err error)

		timeToDo       time.Time
		repeatDuration time.Duration

		// checkIfNeedToRetry returns if it needs to retry, not nil timeToRetry is time to retry Task if not now
		checkIfNeedToRetry func(checkIfNeedToDoError, doError error, retriesCount int) (need bool, timeToRetry *time.Time)
		retriesCount       int

		deps   []string
		onDone func(checkIfNeedToDoError, doError error)

		exitStatus status
		result     taskResult
	}

	taskResult struct {
		CheckIfNeedToDoError error
		DoError              error
	}

	TaskCreator interface {
		WithKey(key ...string) TaskCreator
		WithDo(do func() error) TaskCreator
		WithCheckIfNeedToDo(checkIfNeedToDo func() (bool, *time.Time, error)) TaskCreator
		WithCheckIfNeedToRetry(checkIfNeedToRetry func(checkIfNeedToDoError, doError error, retriesCount int) (need bool, timeToRetry *time.Time)) TaskCreator
		WithOnDone(onDone func(checkIfNeedToDoError, doError error)) TaskCreator
		WithTimeToDo(timeToDo time.Time) TaskCreator
		WithRepeatDuration(repeatDuration time.Duration) TaskCreator
		WithDependency(key ...string) TaskCreator
		Create() task
	}
)

const (
	statusSuccessDone status = iota
	statusFailed
	statusSkipped
)

func NewTask() TaskCreator {
	return &task{}
}

func (t *task) WithKey(keys ...string) TaskCreator {
	t.key = strings.Join(keys, "_")
	return t
}

func (t *task) WithDo(do func() error) TaskCreator {
	t.do = do
	return t
}

func (t *task) WithCheckIfNeedToDo(checkIfNeedToDo func() (bool, *time.Time, error)) TaskCreator {
	t.checkIfNeedToDo = checkIfNeedToDo
	return t
}

func (t *task) WithTimeToDo(timeToDo time.Time) TaskCreator {
	t.timeToDo = timeToDo
	return t
}

func (t *task) WithRepeatDuration(repeatDuration time.Duration) TaskCreator {
	t.repeatDuration = repeatDuration
	return t
}

func (t *task) WithDependency(keys ...string) TaskCreator {
	t.deps = append(t.deps, strings.Join(keys, "_"))

	return t
}

func (t *task) WithOnDone(onDone func(checkIfNeedToDoError, doError error)) TaskCreator {
	t.onDone = onDone

	return t
}

func (t *task) WithCheckIfNeedToRetry(checkIfNeedToRetry func(checkIfNeedToDoError, doError error, retriesCount int) (need bool, timeToRetry *time.Time)) TaskCreator {
	t.checkIfNeedToRetry = checkIfNeedToRetry
	return t
}

func (t *task) Create() task {
	// if key is not set, set it unique key based on uuid v7
	if t.key == "" {
		t.key = uuid.Must(uuid.NewV7()).String()
	}
	// if time to do is not set, set it to now
	if t.timeToDo.IsZero() {
		t.timeToDo = time.Now()
	}
	// if checkIfNeedToDo is not set, set it to always need to do
	if t.checkIfNeedToDo == nil {
		t.checkIfNeedToDo = func() (need bool, nextTimeToDo *time.Time, err error) {
			return true, nil, nil
		}
	}

	// create error channel
	t.result = taskResult{}
	return *t
}

func (t *task) incrementRetriesCount() {
	t.retriesCount++
}

func NewWorker(maxWorkers int, throttle time.Duration) Worker {
	w := &taskWorker{
		maxWorkers:  maxWorkers,
		inputTasks:  make(chan task, 10),
		queuedTasks: make([]task, 0),
		toDoTasks:   make(chan task, maxWorkers),
		doneTasks:   make(chan task, maxWorkers),
		throttle:    throttle,
	}

	w.start()

	return w
}

func (w *taskWorker) start() {
	go w.runWorkerPool()
	go w.manageTasks()
}

func (w *taskWorker) manageTasks() {
	nextIndex := 0

	for {
		select {
		case t := <-w.inputTasks:
			log.Debug().Str("key", t.key).Msg("New Task added")
			// delete Task with same key if exists
			w.deleteTasksByKey(t.key)
			w.m.Lock()
			w.queuedTasks = append(w.queuedTasks, t)

			slices.SortFunc(w.queuedTasks, func(i, j task) int {
				return i.timeToDo.Compare(j.timeToDo)
			})
			w.m.Unlock()
		case t := <-w.doneTasks:
			if t.exitStatus == statusSkipped {
				log.Info().Str("key", t.key).Msg("Task skipped")
			}
			if t.exitStatus == statusSuccessDone {
				log.Info().Str("key", t.key).Msg("Task done")
			}
			if t.exitStatus == statusFailed {
				if t.result.CheckIfNeedToDoError != nil {
					log.Error().Str("key", t.key).Err(t.result.CheckIfNeedToDoError).Msg("Task failed on checkIfNeedToDo")
				}
				if t.result.DoError != nil {
					log.Error().Str("key", t.key).Err(t.result.DoError).Msg("Task failed on do")
				}
			}
			w.m.Lock()
			for i := 0; i < len(w.queuedTasks); i++ {
				// if other Task depends on this Task, remove this Task from dependencies
				index := slices.Index(w.queuedTasks[i].deps, t.key)
				if index != -1 {
					w.queuedTasks[i].deps = slices.Delete(w.queuedTasks[i].deps, index, index+1)
				}
			}
			w.m.Unlock()

			if t.onDone != nil {
				t.onDone(t.result.CheckIfNeedToDoError, t.result.DoError)
			}

		case <-time.Tick(w.throttle):
			w.m.Lock()
			if len(w.queuedTasks) > 0 {
				if w.queuedTasks[nextIndex].timeToDo.Before(time.Now()) {
					// if task has no dependencies, add to toDoTasks
					if len(w.queuedTasks[nextIndex].deps) == 0 {
						w.toDoTasks <- w.queuedTasks[nextIndex]
						w.queuedTasks = slices.Delete(w.queuedTasks, nextIndex, nextIndex+1)

						// reset nextIndex if task to add is found
						nextIndex = 0
					} else {
						// if task has dependencies, check next task
						nextIndex++

						// if nextIndex is out of range, reset it
						if nextIndex >= len(w.queuedTasks) {
							nextIndex = 0
						}
					}
				} else {
					// reset nextIndex if no tasks ready to do
					nextIndex = 0
				}
			}
			w.m.Unlock()
		}
	}
}

func (w *taskWorker) runWorkerPool() {
	for i := 0; i < w.maxWorkers; i++ {
		go func(workerID int) {
			for t := range w.toDoTasks {
				w.doTask(t, workerID)
			}
		}(i + 1)
	}
}

func (w *taskWorker) deleteTasksByKey(key string) {
	w.m.Lock()
	defer w.m.Unlock()
	newQueuedTasks := make([]task, 0)
	for i := 0; i < len(w.queuedTasks); i++ {
		if w.queuedTasks[i].key != key {
			newQueuedTasks = append(newQueuedTasks, w.queuedTasks[i])
		}
	}
	w.queuedTasks = newQueuedTasks
}

// AddTask adds Task, that not returns errors
func (w *taskWorker) AddTask(t task) {
	t.retriesCount = 0
	w.inputTasks <- t
}

func (w *taskWorker) addTaskToRetry(t task) {
	t.incrementRetriesCount()
	w.inputTasks <- t
}

func (w *taskWorker) doTask(t task, workerID int) {
	log.Debug().Str("key", t.key).Msgf("Worker %d started Task", workerID)

	// check if Task is repeatable then add to queue with new time and do Task
	if t.repeatDuration > 0 {
		log.Debug().Str("key", t.key).Msgf("Worker %d Task is repeatable", workerID)
		// if Task is repeatable, add to queue with new time
		t.timeToDo = t.timeToDo.Add(t.repeatDuration)
		w.AddTask(t)
	}
	// if Task is not repeatable, do Task and check if it's needed to do again
	// check if Task is needed to be done
	need, nextTimeToDo, err := t.checkIfNeedToDo()
	if err != nil {
		t.result.CheckIfNeedToDoError = err
		w.tryTaskToDone(t)
		return
	}

	log.Debug().Str("key", t.key).Msgf("Worker %d need: %t, nextTimeToDo: %v", workerID, need, nextTimeToDo)
	// do Task if it's needed
	if need {
		log.Debug().Str("key", t.key).Msgf("Worker %d does Task", workerID)
		if err = t.do(); err != nil {
			t.result.DoError = err
			w.tryTaskToDone(t)
			return
		}
	} else {
		// if Task is not needed to do now, but new time, update Task time and add to queue
		if !(nextTimeToDo == nil) {
			log.Debug().Str("key", t.key).Msgf("Worker %d Task is not needed to do now, but new time, update Task time and add to queue", workerID)
			t.timeToDo = *nextTimeToDo
			w.AddTask(t)
		} else {
			t.exitStatus = statusSkipped
		}
	}

	w.tryTaskToDone(t)
}

func (w *taskWorker) tryTaskToDone(t task) {
	log.Debug().Str("key", t.key).Msg("Trying to add Task to done")

	if t.result.CheckIfNeedToDoError == nil && t.result.DoError == nil {
		w.doneTasks <- t
		return
	}

	if t.checkIfNeedToRetry == nil {
		t.exitStatus = statusFailed
		w.doneTasks <- t
		return
	}

	need, timeToRetry := t.checkIfNeedToRetry(t.result.CheckIfNeedToDoError, t.result.DoError, t.retriesCount)
	if need {
		// if time to retry is not set, set it to now
		if timeToRetry == nil {
			now := time.Now()
			timeToRetry = &now
		}

		t.timeToDo = *timeToRetry
		w.addTaskToRetry(t)

	} else {
		w.doneTasks <- t
	}
}
