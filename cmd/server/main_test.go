package main

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/riverqueue/river"
)

func TestRegisterPreparedDeckWorkersRegistersDurableKinds(t *testing.T) {
	workers := river.NewWorkers()
	registerPreparedDeckWorkers(workers, nil, nil, nil, nil, nil, 0, nil)

	for _, test := range []struct {
		name string
		add  func() error
	}{
		{name: "batch submit", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.BatchSubmitWorker{}) }},
		{name: "batch poll", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.BatchPollWorker{}) }},
		{name: "finalize", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.FinalizeWorker{}) }},
		{name: "batch cleanup", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.BatchCleanupWorker{}) }},
		{name: "recovery", add: func() error { return river.AddWorkerSafely(workers, &prepareddeck.RecoveryWorker{}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.add(); err == nil {
				t.Fatal("worker kind was not registered")
			}
		})
	}
}
