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
	taskWorker struct {
		m           sync.RWMutex
		maxWorkers  int
		inputTasks  chan task
		queuedTasks []task
		toDoTasks   chan task
	}

	Worker interface {
		AddTaskWithResult(t task) <-chan TaskResult
		AddTask(t task)
	}

	task struct {
		key string // key to identify Task
		do  func() error
		// checkIfNeedToDo returns if it needs to do now, not nil timeToDo is time to do Task if not now
		checkIfNeedToDo func() (need bool, nextTimeToDo *time.Time, err error)
		timeToDo        time.Time
		repeatDuration  time.Duration

		result     chan TaskResult
		withResult bool
	}

	TaskResult struct {
		CheckIfNeedToDoError error
		DoError              error
	}

	TaskCreator interface {
		WithKey(key ...string) TaskCreator
		WithDo(do func() error) TaskCreator
		WithCheckIfNeedToDo(checkIfNeedToDo func() (bool, *time.Time, error)) TaskCreator
		WithTimeToDo(timeToDo time.Time) TaskCreator
		WithRepeatDuration(repeatDuration time.Duration) TaskCreator
		Create() task
	}
)

func NewTask() TaskCreator {
	return task{}
}

func (t task) WithKey(keys ...string) TaskCreator {
	t.key = strings.Join(keys, "_")
	return t
}

func (t task) WithDo(do func() error) TaskCreator {
	t.do = do
	return t
}

func (t task) WithCheckIfNeedToDo(checkIfNeedToDo func() (bool, *time.Time, error)) TaskCreator {
	t.checkIfNeedToDo = checkIfNeedToDo
	return t
}

func (t task) WithTimeToDo(timeToDo time.Time) TaskCreator {
	t.timeToDo = timeToDo
	return t
}

func (t task) WithRepeatDuration(repeatDuration time.Duration) TaskCreator {
	t.repeatDuration = repeatDuration
	return t
}

func (t task) Create() task {
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
	t.result = make(chan TaskResult)
	return t
}

func NewWorker(maxWorkers int) Worker {
	w := &taskWorker{
		maxWorkers:  maxWorkers,
		inputTasks:  make(chan task, 10),
		toDoTasks:   make(chan task, maxWorkers),
		queuedTasks: make([]task, 0),
	}

	w.start()

	return w
}

func (w *taskWorker) start() {
	go w.manageTasks()
	go w.manageToDoTasks()
	go w.runWorkerPool()
}

// AddTaskWithResult adds Task, that returns errors to result channel and blocks doing next Task if channel is not read
func (w *taskWorker) AddTaskWithResult(t task) <-chan TaskResult {
	t.withResult = true

	w.addTask(t)

	// return error channel
	return t.result
}

// AddTask adds Task, that not returns errors
func (w *taskWorker) AddTask(t task) {
	close(t.result)
	w.addTask(t)
}

func (w *taskWorker) addTask(t task) {
	w.inputTasks <- t
}

func (w *taskWorker) manageTasks() {
	for t := range w.inputTasks {
		log.Debug().Str("key", t.key).Msg("New Task added")
		// delete Task with same key if exists
		w.deleteTasksByKey(t.key)
		w.m.Lock()
		w.queuedTasks = append(w.queuedTasks, t)

		slices.SortFunc(w.queuedTasks, func(i, j task) int {
			return i.timeToDo.Compare(j.timeToDo)
		})

		w.m.Unlock()
	}
}

func (w *taskWorker) manageToDoTasks() {
	for {
		w.m.Lock()
		if len(w.queuedTasks) > 0 && w.queuedTasks[0].timeToDo.Before(time.Now()) {
			log.Debug().Str("key", w.queuedTasks[0].key).Msg("Task moved to toDoTasks")
			// move Task to toDoTasks
			w.toDoTasks <- w.queuedTasks[0]
			// remove Task from queuedTasks
			w.queuedTasks = w.queuedTasks[1:]
		}
		w.m.Unlock()
	}
}

func (w *taskWorker) runWorkerPool() {
	for i := 0; i < w.maxWorkers; i++ {
		go func(workerID int) {
			for t := range w.toDoTasks {
				log.Debug().Str("key", t.key).Msgf("Worker %d started Task", workerID)
				// check if Task is repeatable then add to queue with new time and do Task
				if t.repeatDuration > 0 {
					log.Debug().Str("key", t.key).Msgf("Worker %d Task is repeatable", workerID)
					// if Task is repeatable, add to queue with new time
					t.timeToDo = t.timeToDo.Add(t.repeatDuration)
					w.addTask(t)
				}
				// if Task is not repeatable, do Task and check if it's needed to do again
				// check if Task is needed to be done
				need, nextTimeToDo, err := t.checkIfNeedToDo()
				if err != nil {
					// if error occurred, send it to Task result channel
					if t.withResult {
						t.result <- TaskResult{
							CheckIfNeedToDoError: err,
						}
					}

					continue
				}

				log.Debug().Str("key", t.key).Msgf("Worker %d need: %t, nextTimeToDo: %v", workerID, need, nextTimeToDo)
				// do Task if it's needed
				if need {
					log.Debug().Str("key", t.key).Msgf("Worker %d does Task", workerID)
					if err = t.do(); err != nil {
						if t.withResult {
							t.result <- TaskResult{
								DoError: err,
							}
						}
						continue
					}
					log.Debug().Str("key", t.key).Msgf("Worker %d Task done", workerID)
				} else {
					// if Task is not needed to do now, but new time, update Task time and add to queue
					if !(nextTimeToDo == nil) {
						log.Debug().Str("key", t.key).Msgf("Worker %d Task is not needed to do now, but new time, update Task time and add to queue", workerID)
						t.timeToDo = *nextTimeToDo
						w.addTask(t)
					}
				}

				if t.withResult {
					t.result <- TaskResult{}
				}
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
