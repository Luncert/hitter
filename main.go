package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

var (
	totalTaskNum int
	finished     int
	failed       int

	lock   = sync.Mutex{}
	logBuf = NewMutexBuffer()
)

// progress animation
var (
	cr  = [4]rune{'-', '\\', '|', '/'}
	crI = 0
)

func main() {
	reqNum, concurrency, url := parseArguments()
	totalTaskNum = reqNum

	fmt.Println("request num:", reqNum)
	fmt.Println("concurrency:", concurrency)

	// create Client
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				deadline := time.Now().Add(120 * time.Second)
				c, err := net.DialTimeout(network, addr, time.Second*20)
				if err != nil {
					return nil, err
				}
				_ = c.SetDeadline(deadline)
				return c, nil
			},
		},
	}

	// start executing task
	ex := CreateExecutor(concurrency)
	fmt.Println("----------------")

	start := time.Now()
	for i := 0; i < reqNum; i++ {
		_ = ex.Execute(makeRequest, client, url)
	}
	ex.Stop()

	// print statistics
	timeUsage := time.Since(start).Seconds()
	printStatistics(timeUsage)

	// save failed log
	if err := ioutil.WriteFile("failed.log", logBuf.b.Bytes(), 0777); nil != err {
		fmt.Println(err)
	}
}

func parseArguments() (reqNum int, concurrency int, url string) {
	if os.Args == nil || len(os.Args) != 4 {
		fmt.Println("usage: reqNum concurrency apiUrl")
		return
	}

	// read command arguments
	var err error
	reqNum, err = strconv.Atoi(os.Args[1])
	concurrency, err = strconv.Atoi(os.Args[2])
	url = os.Args[3]
	if nil != err {
		panic(err)
	}

	if reqNum <= 0 {
		panic(fmt.Errorf("reqNum must positive"))
	}
	if concurrency <= 0 {
		panic(fmt.Errorf("concurrency must positive"))
	}

	return
}

func makeRequest(args ...interface{}) {
	client := args[0].(*http.Client)
	url := args[1].(string)

	// FIXME: provide as command arguments
	var rep *http.Response
	req, err := http.NewRequest("GET", url, nil)
	if err == nil {
		// FIXME: provide as command arguments
		//req.Header.Add("Authorization", "Basic U0hFTkpJQU4wOlNqOTQwNTE1")

		rep, err = client.Do(req)
	}

	// submit execution result
	if err == nil && rep.StatusCode == 200 {
		submit(true)
	} else {
		// log error info
		var repInfo interface{} = nil
		if rep != nil {
			repInfo = rep.StatusCode
		}
		_, _ = fmt.Fprintf(logBuf, "[%v] %v\n", repInfo, err)

		submit(false)
	}
}

func submit(success bool) {
	lock.Lock()
	defer lock.Unlock()

	// update progress
	if !success {
		failed++
	}
	finished++

	// print progress
	printProgress()
}

func printProgress() {
	fmt.Printf("%c hitting %.2f%%\r", cr[crI], float32(finished)*float32(100)/float32(totalTaskNum))
	crI = (crI + 1) % 4
}

func printStatistics(timeUsage float64) {
	fmt.Printf("total time: %.6f sec\n", timeUsage)
	fmt.Printf("qps: %.6f\n", float64(totalTaskNum)/timeUsage)
	fmt.Printf("total: %d\n", totalTaskNum)
	fmt.Printf("failed: %d\n", failed)
}
