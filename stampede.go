package main

import (
    "flag"
    "fmt"
    "os"
    "net/http"
    "strconv"
    "time"
)

// var url *string = flag.String(""

func usage() {
    fmt.Fprintf(os.Stderr, "usage: stampede [address]\n")
    flag.PrintDefaults()
    os.Exit(2)
}

func begin_attack(url string, clients int) {
    response_codes := make(chan int)
    done := false
    stop := make(chan bool)

    var codestats = map[string] int {
        "200": 0,
        "300": 0,
        "400": 0,
        "500": 0,
    }

    for i := 0; i < clients; i++ {
        go func (url string, codes chan<- int, done * bool) {
            for !(*done) {
                fmt.Printf("Requesting url %s\n", url)
                resp, err := http.Get(url)
                if err != nil {
                    fmt.Fprintf(os.Stderr, "Failed to connect to %s\n", url)
                } else {
                    codes <- resp.StatusCode
                }
            }
        }(url, response_codes, &done)
    }

    go func (codes <-chan int, codestats map[string] int, done * bool) {
        code := 0
        for !*done {
            code = <-codes

            if (code >= 200 && code < 300) {
                codestats["200"] = codestats["200"] + 1
            } else if (code >= 300 && code < 400) {
                codestats["300"] = codestats["300"] + 1
            } else if (code >= 400 && code < 500) {
                codestats["400"] = codestats["400"] + 1
            } else if (code >= 500 && code < 600) {
                codestats["500"] = codestats["500"] + 1
            }
            fmt.Println(code)
        }
    }(response_codes, codestats, &done)

    go func (done * bool, stop chan<- bool) {
        time.Sleep(5 * time.Second)
        *done = true
        stop <- true
    }(&done, stop)

    <-stop

    for k, _ := range codestats {
        fmt.Printf("%s codes: %d\n", k, codestats[k])
    }
}

func main() {
    flag.Usage = usage
    flag.Parse()

    args := flag.Args()

    if len(args) < 2 {
       fmt.Printf("Too few arugments.")
       os.Exit(1)
    }
    fmt.Printf("Address is %s\n ", args[0])
    number, _ := strconv.Atoi(args[1])

    begin_attack(args[0], number)
}



