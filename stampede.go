package main

import (
    "flag"
    "fmt"
    "os"
    "bufio"
    "bytes"
    "io"
    "net/http"
    "math/rand"
    "time"
)

var (
    input_file       = flag.String("f", "", "Input file to read from.")
    clients          = flag.Int("c", 10, "The number of buffalo to spawn.")
    duration         = flag.Int("t", 30, "The duration of the stampede in seconds.")
    internet_mode    = flag.Bool("i", false, "Random sampling of the input file.")
    show_help        = flag.Bool("h", false, "Show this help.")
)


func usage() {
    fmt.Fprintf(os.Stderr, "usage: stampede [address]\n")
    fmt.Fprintf(os.Stderr, "\nOptions:\n")
    flag.PrintDefaults()
    os.Exit(2)
}

func begin_attack(endpoints []string) {
    response_codes := make(chan int)
    done := false
    stop := make(chan bool)

    var codestats = map[string] int {
        "200": 0,
        "300": 0,
        "400": 0,
        "500": 0,
    }
    rand.Seed(time.Now().UTC().UnixNano())

    for i := 0; i < * clients; i++ {
        go func (endpoints []string, codes chan<- int, done * bool) {
            max := len (endpoints)
            counter := 0
            url := ""
            for !(*done) {
                index := 0
                if (* internet_mode) {
                    index = rand.Intn(max)
                } else {
                    index = counter % max
                    counter++
                }
                url = endpoints[index]

                fmt.Printf("Requesting url %s\n", url)
                resp, err := http.Get(url)
                if err != nil {
                    fmt.Fprintf(os.Stderr, "Failed to connect to %s\n", url)
                } else {
                    codes <- resp.StatusCode
                }
            }
        }(endpoints, response_codes, &done)
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

// Read a whole file into the memory and store it as array of lines
func read_lines(path string) (lines []string, err error) {
    var (
        file *os.File
        part []byte
        prefix bool
    )
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

func main() {
    var endpoints []string
    flag.Usage = usage
    flag.Parse()
    args := flag.Args()

    if *show_help {
        flag.Usage()
        os.Exit(1)
    }

    if len(*input_file) > 0 {
        _, err := os.Stat(*input_file)
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error: input file not readable\n")
            os.Exit(1)
        } else {
            endpoints, _ = read_lines(*input_file)
        }
    } else if len(args) == 1 {
        endpoints = []string{args[0]}
    } else {
        flag.Usage()
        os.Exit(1)
    }

    fmt.Printf("Starting stampede with %d clients...\n ", * clients)

    begin_attack(endpoints)
}



