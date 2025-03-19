package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"github.com/gookit/color"
	"golang.org/x/net/http/httpguts"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	totalTaskNum int
	finished     int
	failed       int

	lock   = sync.Mutex{}
	logBuf = NewMutexBuffer()

	usage = `Version: 0.0.1-SNAPSHOT
Usage: hitter [-hncmHbfsi] [-a apiUrl]

Options:
`
)

// progress animation
var (
	cr  = [4]rune{'-', '\\', '|', '/'}
	crI = 0
	timeUsages = make([]float64, 0)
)

type Headers map[string]string

func (h Headers) String() string {
	tmp := make([]string, len(h))

	i := 0
	for name, value := range h {
		tmp[i] = fmt.Sprintf("%s=%s", name, value)
		i++
	}
	return strings.Join(tmp, ",")
}

func (h Headers) Set(value string) error {
	i := strings.IndexRune(value, '=')
	if i <= 0 {
		return fmt.Errorf("invalid header")
	}
	h[value[:i]] = value[i+1:]
	return nil
}

type RequestParams struct {
	url     string
	method  string
	headers Headers
	body    []byte
	enableRequestId bool
}

func main() {
	notSaveLog, reqNum, concurrency, requestParams := parseArguments()
	totalTaskNum = reqNum

	fmt.Println("url:", requestParams.url)
	fmt.Println("method:", requestParams.method)
	fmt.Println("headers:", requestParams.headers)
	fmt.Println("body:", len(requestParams.body), "bytes")
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

	start := time.Now().UnixNano()
	for i := 0; i < reqNum; i++ {
		_ = ex.Execute(makeRequest, client, requestParams, i)
	}
	ex.Stop()
	timeUsage := float64(time.Now().UnixNano()-start) / 1e9

	// print statistics
	printStatistics(timeUsage)

	// save failed log
	if !notSaveLog {
		if err := ioutil.WriteFile("failed.log", logBuf.b.Bytes(), 0777); nil != err {
			fmt.Println(err)
		}
	}
}

func parseArguments() (bool, int, int, RequestParams) {
	help := flag.Bool("h", false, "help")
	notSaveLog := flag.Bool("s", false, "don't save log")
	enableRequestId := flag.Bool("i", false, "add request id to payload using placeholder {{}}")

	reqNum := flag.Int("n", 1, "total requests number")
	concurrency := flag.Int("c", 1, "maximum go-channel number to limit concurrency")

	url := flag.String("a", "", "api url")
	method := flag.String("m", "GET", "request method")
	headers := Headers{}
	flag.Var(&headers, "H", "request headers, format: name=value")
	bodyString := flag.String("b", "", "request body")
	file := flag.String("f", "", "read a file as request body, won't work if -b is provided")

	flag.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, usage)
		flag.PrintDefaults()
	}

	// parse
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if *url == "" {
		color.Red.Println("api url missing")
		os.Exit(1)
	}

	// check if method is valid
	if !validMethod(*method) {
		color.Red.Printf("invalid method %q\n", *method)
		os.Exit(1)
	}

	var body []byte
	if *bodyString == "" && *file != "" {
		// try to read file as body
		var err error
		body, err = ioutil.ReadFile(*file)
		if err != nil {
			color.Red.Println(err)
			os.Exit(1)
		}
	} else {
		body = []byte(*bodyString)
	}

	return *notSaveLog, *reqNum, *concurrency,
		RequestParams{
			url:     *url,
			method:  *method,
			headers: headers,
			body:    body,
			enableRequestId: *enableRequestId,
		}
}

func validMethod(method string) bool {
	/*
	     Method         = "OPTIONS"                ; Section 9.2
	                    | "GET"                    ; Section 9.3
	                    | "HEAD"                   ; Section 9.4
	                    | "POST"                   ; Section 9.5
	                    | "PUT"                    ; Section 9.6
	                    | "DELETE"                 ; Section 9.7
	                    | "TRACE"                  ; Section 9.8
	                    | "CONNECT"                ; Section 9.9
	                    | extension-method
	   extension-method = token
	     token          = 1*<any CHAR except CTLs or separators>
	*/
	return len(method) > 0 && strings.IndexFunc(method, isNotToken) == -1
}

func isNotToken(r rune) bool {
	return !httpguts.IsTokenRune(r)
}

func makeRequest(args ...interface{}) {
	client := args[0].(*http.Client)
	requestParams := args[1].(RequestParams)
	reqId := args[2].(int)

	var timeUsage float64

	var bodyReader *bytes.Reader
	if requestParams.enableRequestId {
		var bodyString = strings.Replace(string(requestParams.body[:]), "{{}}", fmt.Sprintf("%d", reqId), 1)
		bodyReader = bytes.NewReader([]byte(bodyString))
	} else {
		bodyReader = bytes.NewReader(requestParams.body)
	}

	var rep *http.Response
	req, err := http.NewRequest(requestParams.method, requestParams.url, bodyReader)
	if err == nil {
		// add headers
		for name, value := range requestParams.headers {
			req.Header.Add(name, value)
		}

		// do request
		start := time.Now().UnixNano()
		rep, err = client.Do(req)
		timeUsage = float64(time.Now().UnixNano() - start) / 1e6
	}

	// submit execution result
	if err == nil && rep.StatusCode < 300 {
		submit(timeUsage, true)
	} else {
		// log error info
		var repInfo interface{} = nil
		if rep != nil {
			repInfo = rep.StatusCode
		}
		_, _ = fmt.Fprintf(logBuf, "[%v] %v\n", repInfo, err)

		submit(timeUsage, false)
	}
}

func submit(timeUsage float64, success bool) {
	lock.Lock()
	defer lock.Unlock()

	timeUsages = append(timeUsages, timeUsage)

	// update progress
	if !success {
		failed++
	}
	finished++

	// print progress
	printProgress()
}

func printProgress() {
	color.Green.Printf("%c hitting %.2f%%\r", cr[crI], float32(finished)*float32(100)/float32(totalTaskNum))
	crI = (crI + 1) % 4
}

func printStatistics(timeUsage float64) {
	fmt.Printf("total time: %.6f sec\n", timeUsage)
	if timeUsage == 0 {
		fmt.Printf("qps: %.6f\n", float64(0))
	} else {
		fmt.Printf("qps: %.6f\n", float64(totalTaskNum)/timeUsage)
	}

	var timeUsageSum float64
	var timeUsageMax float64
	var timeUsageMin float64 = timeUsages[0]
	for i := 0; i < len(timeUsages); i++ {
		timeUsageSum += timeUsages[i]
		if timeUsageMin > timeUsages[i] {
			timeUsageMin = timeUsages[i]
		}
		if timeUsageMax < timeUsages[i] {
			timeUsageMax = timeUsages[i]
		}
	}

	fmt.Printf("resp.avg: %.6f ms\n", timeUsageSum / float64(len(timeUsages)))
	fmt.Printf("resp.max: %.6f ms\n", timeUsageMax)
	fmt.Printf("resp.min: %.6f ms\n", timeUsageMin)

	fmt.Printf("total: %d\n", totalTaskNum)
	fmt.Printf("failed: %d\n", failed)
}
