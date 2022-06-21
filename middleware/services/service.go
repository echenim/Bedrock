package services

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/echenim/ibu/middleware/models"
	"github.com/valyala/fasthttp"
)

var (
	requests         int64
	period           int64
	klients          int
	url              string
	urlsFilePath     string
	keepAlive        bool
	postDataFilePath string
	writeTimeout     int
	readTimeout      int
	authHeader       string
)

func init() {
	flag.Int64Var(&requests, "r", -1, "Number of requests per client")
	flag.IntVar(&klients, "c", 100, "Number of concurrent clients")
	flag.StringVar(&url, "u", "", "URL")
	flag.StringVar(&urlsFilePath, "f", "", "URL's file path (line seperated)")
	flag.BoolVar(&keepAlive, "k", true, "Do HTTP keep-alive")
	flag.StringVar(&postDataFilePath, "d", "", "HTTP POST data file path")
	flag.Int64Var(&period, "t", -1, "Period of time (in seconds)")
	flag.IntVar(&writeTimeout, "tw", 5000, "Write timeout (in milliseconds)")
	flag.IntVar(&readTimeout, "tr", 5000, "Read timeout (in milliseconds)")
	flag.StringVar(&authHeader, "auth", "", "Authorization header")
}

func echoResults(results map[int]*models.Result, startTime time.Time) {
	var requests int64
	var success int64
	var networkFailed int64
	var badFailed int64

	for _, result := range results {
		requests += result.Success
		networkFailed += result.NetworkFailed
		badFailed += result.BadFailed
	}

	elapsed := int64(time.Since(startTime).Seconds())

	if elapsed == 0 {
		elapsed = 1
	}

	fmt.Println()
	fmt.Printf("Requests:                       %10d hits\n", requests)
	fmt.Printf("Successful requests:            %10d hits\n", success)
	fmt.Printf("Network failed:                 %10d hits\n", networkFailed)
	fmt.Printf("Bad requests failed (!2xx):     %10d hits\n", badFailed)
	fmt.Printf("Successful requests rate:       %10d hits/sec\n", success/elapsed)
	fmt.Printf("Read throughput:                %10d bytes/sec\n", models.ReadThroughput/elapsed)
	fmt.Printf("Write throughput:               %10d bytes/sec\n", models.WriteThroughput/elapsed)
	fmt.Printf("Test time:                      %10d sec\n", elapsed)
}

func readLines(path string) (lines []string, err error) {
	var file *os.File
	var part []byte
	var prefix bool

	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	buffer := bytes.NewBuffer(make([]byte, 0))
	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			break
		}
		buffer.Write(part)
		if !prefix {
			lines = append(lines, buffer.String())
			buffer.Reset()
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

func newConfig() *models.Configuration {
	if urlsFilePath == "" && url == "" {
		flag.Usage()
		os.Exit(1)
	}

	if requests == -1 && period == -1 {
		fmt.Println("Requests or period must be provided")
		flag.Usage()
		os.Exit(1)
	}

	if requests != -1 && period != -1 {
		fmt.Println("Only one should be provided: [requests|period]")
		flag.Usage()
		os.Exit(1)
	}

	configuration := &models.Configuration{
		Urls:       make([]string, 0),
		Method:     "GET",
		PostData:   nil,
		KeepAlive:  keepAlive,
		Requests:   int64((1 << 63) - 1),
		AuthHeader: authHeader,
	}

	if period != -1 {
		configuration.Period = period

		timeout := make(chan bool, 1)
		go func() {
			<-time.After(time.Duration(period) * time.Second)
			timeout <- true
		}()

		go func() {
			<-timeout
			pid := os.Getpid()
			proc, _ := os.FindProcess(pid)
			err := proc.Signal(os.Interrupt)
			if err != nil {
				log.Println(err)
				return
			}
		}()
	}

	if requests != -1 {
		configuration.Requests = requests
	}

	if urlsFilePath != "" {
		fileLines, err := readLines(urlsFilePath)
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: ", urlsFilePath, err)
		}

		configuration.Urls = fileLines
	}

	if url != "" {
		configuration.Urls = append(configuration.Urls, url)
	}

	if postDataFilePath != "" {
		configuration.Method = "POST"

		data, err := ioutil.ReadFile(postDataFilePath)
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file path: %s Error: ", postDataFilePath, err)
		}

		configuration.PostData = data
	}

	configuration.Client.ReadTimeout = time.Duration(readTimeout) * time.Millisecond
	configuration.Client.WriteTimeout = time.Duration(writeTimeout) * time.Millisecond
	configuration.Client.MaxConnsPerHost = klients

	configuration.Client.Dial = dialer()

	return configuration
}

func dialer() func(address string) (conn net.Conn, err error) {
	return func(address string) (net.Conn, error) {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			return nil, err
		}

		con := &models.Conn{Conn: conn}

		return con, nil
	}
}

func client(configuration *models.Configuration, result *models.Result, done *sync.WaitGroup) {
	for result.Requests < configuration.Requests {
		for _, tmpUrl := range configuration.Urls {

			req := fasthttp.AcquireRequest()

			req.SetRequestURI(tmpUrl)
			req.Header.SetMethodBytes([]byte(configuration.Method))

			if configuration.KeepAlive == true {
				req.Header.Set("Connection", "keep-alive")
			} else {
				req.Header.Set("Connection", "close")
			}

			if len(configuration.AuthHeader) > 0 {
				req.Header.Set("Authorization", configuration.AuthHeader)
			}

			req.SetBody(configuration.PostData)

			resp := fasthttp.AcquireResponse()
			err := configuration.Client.Do(req, resp)
			statusCode := resp.StatusCode()
			result.Requests++
			fasthttp.ReleaseRequest(req)
			fasthttp.ReleaseResponse(resp)

			if err != nil {
				result.NetworkFailed++
				continue
			}

			if statusCode == fasthttp.StatusOK {
				result.Success++
			} else {
				result.BadFailed++
			}
		}
	}

	done.Done()
}
