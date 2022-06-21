package services

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"time"

	"github.com/echenim/ibu/middleware/models"
)

func NewLoadManager() {
	startTime := time.Now()
	var wg sync.WaitGroup
	results := make(map[int]*models.Result)

	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt)

	go func() {
		_ = <-ch
		echoResults(results, startTime)
		os.Exit(0)
	}()

	flag.Parse()

	configuration := newConfig()

	goMaxProcs := os.Getenv("GOMAXPROCS")

	if goMaxProcs == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	fmt.Printf("Dispatching %d clients\n", klients)

	wg.Add(klients)
	for i := 0; i < klients; i++ {
		result := &models.Result{}
		results[i] = result
		go client(configuration, result, &wg)
	}
	fmt.Println("Waiting for results...")
	wg.Wait()
	echoResults(results, startTime)
}
