package services

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/echenim/ibu/core/models"
)

func NewLoadManager() {
	startTime := time.Now()
	var done sync.WaitGroup
	results := make(map[int]*models.Result)

	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt)

	go func() {
		_ = <-signalChannel
		PrintResults(results, startTime)
		os.Exit(0)
	}()

	flag.Parse()

	configuration := NewConfiguration()

	goMaxProcs := os.Getenv("GOMAXPROCS")

	if goMaxProcs == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	fmt.Printf("Dispatching %d clients\n", Klients)

	done.Add(Klients)
	for i := 0; i < Klients; i++ {
		result := &models.Result{}
		results[i] = result
		go Client(configuration, result, &done)
	}
	fmt.Println("Waiting for results...")
	done.Wait()
	PrintResults(results, startTime)
}
