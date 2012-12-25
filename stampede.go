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
    "sync"
)

var (
    input_file       = flag.String  ("f", "", "Input file to read from.")
    clients          = flag.Int     ("c", 10, "The number of buffalo to spawn.")
    duration         = flag.Int     ("t", 30, "The duration of the stampede in seconds.")
    wait_duration    = flag.Int     ("w", 1, "The time clients wait between HTPT requests (in seconds).")
    internet_mode    = flag.Bool    ("i", false, "Random sampling of the input file.")
    verbose          = flag.Bool    ("v", false, "Use verbose logging.")
    benchmark_mode   = flag.Bool    ("b", false, "Benchmark mode, fire requests without waiting.")
    show_help        = flag.Bool    ("h", false, "Show this help.")

    // This structure is not threadsafe AFAIK so
    // it can only be modifed and read from a single
    // goroutine.
    codestats = map[string] int {
        "2xx":      0,
        "3xx":      0,
        "4xx":      0,
        "5xx":      0,
        "success":  0,
        "fail":     0,
    }

    // Some waitgroups, baby!
    producers_wg sync.WaitGroup
    consumers_wg sync.WaitGroup
)

func usage() {
    fmt.Fprintf(os.Stderr, "usage: stampede [address]\n")
    fmt.Fprintf(os.Stderr, "\nOptions:\n")
    flag.PrintDefaults()
    os.Exit(2)
}

func begin_stampede(endpoints []string) {
    codes       := make(chan int)
    timeout     := make(chan bool)
    transport   := &http.Transport {
        DisableKeepAlives: false,
    }
    client      := &http.Client { Transport: transport }

    // After timeout, fire done signals
    go func() {
        time.Sleep(time.Duration(*duration) * time.Second)
        defer close(timeout)
        for i := 0; i < *clients; i++ { timeout <- true }
    }()

    // Create the clients (producers)
    for i := 0; i < *clients; i++ {
        producers_wg.Add(1)
        go func() {
            var url string
            max     := len(endpoints)
            counter := 0

// This feels hyper naughty but I swear I read somewhere to do this
// to break out of a for-select
sadtimeslabel:
            for {
                select {
                    case <-timeout:
                        break sadtimeslabel
                    default:
                        index := 0

                        // In "Internet Mode" we cycle through random URLs from the
                        // input list
                        if *internet_mode {
                            index = rand.Int() % max
                        // In normal mode we just iterate linearly
                        } else {
                            index = counter % max
                            counter++
                        }
                        url = endpoints[index]

                        if *verbose {
                            fmt.Printf("Requesting url %s\n", url)
                        }

                        resp, err := client.Get(url)

                        if err != nil {
                            fmt.Fprintf(os.Stderr, "Failed to connect to %s, %s\n", url, err)
                            codes <- 500
                        } else {
                            codes <- resp.StatusCode
                        }

                        if (*wait_duration >= 0) {
                            time.Sleep(time.Duration(*wait_duration) * time.Second)
                        }
                        defer resp.Body.Close()
                }
            }
            producers_wg.Done()
        }()
    }

    // Build the consumer that aggregates the stats
    consumers_wg.Add(1)
    go func () {
        for code := range(codes) {
            if (*verbose) {
                fmt.Printf("Received code %d\n", code)
            }

            if (code >= 200 && code < 300) {
                codestats["2xx"] = codestats["2xx"] + 1
                codestats["success"] = codestats["success"] + 1
            } else if (code >= 300 && code < 400) {
                codestats["3xx"] = codestats["3xx"] + 1
                codestats["success"] = codestats["success"] + 1
            } else if (code >= 400 && code < 500) {
                codestats["4xx"] = codestats["4xx"] + 1
                codestats["success"] = codestats["success"] + 1
            } else if (code >= 500 && code < 600) {
                codestats["5xx"] = codestats["5xx"] + 1
                codestats["fail"] = codestats["fail"] + 1
            }
        }
        consumers_wg.Done()
    }()

    producers_wg.Wait()
    close(codes)
    consumers_wg.Wait()

    // Print out the response code histogram
    fmt.Printf("\nReponse codes:\n")
    fmt.Printf("[%s]:\t%d\n", "2xx", codestats["2xx"])
    fmt.Printf("[%s]:\t%d\n", "3xx", codestats["3xx"])
    fmt.Printf("[%s]:\t%d\n", "4xx", codestats["4xx"])
    fmt.Printf("[%s]:\t%d\n", "5xx", codestats["5xx"])

    fmt.Printf("\nTotal Requests:\t%d\n", codestats["success"] + codestats["fail"])
    fmt.Printf("Availability:\t%4.2f\n", float32(codestats["success"]) * 100.0 / float32(codestats["fail"] + codestats["success"]))
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

func init() {
    // Seed the RNG
    rand.Seed(time.Now().UTC().UnixNano())
}

func main() {
    var endpoints []string

    // Set up command line flags
    flag.Usage = usage
    flag.Parse()
    args := flag.Args()

    if *show_help {
        flag.Usage()
        os.Exit(1)
    }

    // If there's an input file, read from it
    if len(*input_file) > 0 {

        // Probably would be good to have a file format check here
        _, err := os.Stat(*input_file)
        if err != nil {
            fmt.Fprintf(os.Stderr, "Error: input file not readable\n")
            os.Exit(1)
        } else {
            endpoints, _ = read_lines(*input_file)
        }

    // Otherwise use the first command line arg as the
    // endpoint
    } else if len(args) == 1 {
        endpoints = []string { args[0] }

    // Otherwise somebody screwed up, bail
    } else {
        flag.Usage()
        os.Exit(1)
    }

    // If benchmark mode is set, force wait_duration to 0
    if (*benchmark_mode) {
        *wait_duration = 0
    }

    fmt.Printf("Starting stampede with %d buffalo...\n", *clients)
    begin_stampede(endpoints)
}
