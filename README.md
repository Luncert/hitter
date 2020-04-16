# hitter
Performance test tool built with Golang.

## Install
### Prerequisite
- go 1.11.4 or higher version: https://golang.org/
- godep: https://github.com/tools/godep

### Step
Pull this repository and run following commands in the root directory:
```shell script
godep restore
go build
```
It should be error free and now you could see an executable file generated in the root directory.
## Usage
```shell script
Usage: hitter [-hncmHbfs] [-a apiUrl]

Options:
  -H value
        request headers, format: name=value
  -a string
        api url
  -b string
        request body
  -c int
        maximum go-channel number to limit concurrency (default 1)
  -f string
        read a file as request body, won't work if -b is provided
  -h    help
  -m string
        request method (default "GET")
  -n int
        total requests number (default 1)
  -s    don't save log
```
