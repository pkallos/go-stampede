package main

import (
    "flag"
    "fmt"
    "os"
    "os/signal"
    "bufio"
    "bytes"
    "log"
    "io"
    "net/http"
    "math/rand"
    "time"
    "sync"
    "runtime/pprof"
)

type Response struct {
    StatusCode      int
    Time            time.Duration
}

type ResponseStats struct {
    Status2xx       int
    Status3xx       int
    Status4xx       int
    Status5xx       int
    Total           int
    TotalTime       float64
    MaxTime         int
    MinTime         int
}

var (
    input_file       = flag.String  ("f", "", "Input file to read from.")
    profile          = flag.String  ("p", "", "Write out CPU profile.")
    clients          = flag.Int     ("c", 10, "The number of buffalo to spawn.")
    duration         = flag.Int     ("t", 30, "The duration of the stampede in seconds.")
    wait_duration    = flag.Int     ("w", 1, "The time clients wait between HTTP requests (in seconds).")
    internet_mode    = flag.Bool    ("i", false, "Random sampling of the input file.")
    verbose          = flag.Bool    ("v", false, "Use verbose logging.")
    benchmark_mode   = flag.Bool    ("b", false, "Benchmark mode, fire requests without waiting.")
    show_help        = flag.Bool    ("h", false, "Show this help.")

    // This structure is not threadsafe AFAIK so
    // it can only be modifed and read from a single
    // goroutine.
    response_stats = ResponseStats {
        Status2xx:   0,
        Status3xx:   0,
        Status4xx:   0,
        Status5xx:   0,
        Total:       0,
        TotalTime:   0,
        MaxTime:     0,
        MinTime:     2048000000,
    }

    // Some waitgroups, baby!
    producers_wg sync.WaitGroup
    consumers_wg sync.WaitGroup

    os_signals chan os.Signal
)

func usage() {
    fmt.Fprintf(os.Stderr, "usage: stampede [address]\n")
    fmt.Fprintf(os.Stderr, "\nOptions:\n")
    flag.PrintDefaults()
    os.Exit(2)
}

func begin_stampede(endpoints []string) {
    responses   := make(chan Response, *clients * 100)
    timeout     := make(chan bool)

    // After timeout, fire done signals
    go func() {
        time.Sleep(time.Duration(*duration) * time.Second)
        defer close(timeout)
        for i := 0; i < *clients; i++ { timeout <- true }
    }()

    // Record the start time
    start_time := time.Now()

    // Create the clients (producers)
    for i := 0; i < *clients; i++ {
        producers_wg.Add(1)
        go func() {
            var url string
            max     := len(endpoints)
            counter := 0
            transport   := &http.Transport {
                DisableKeepAlives  : false,
                DisableCompression : false,
            }
            client      := &http.Client { Transport: transport }

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

                        start := time.Now()
                        resp, err := client.Get(url)
                        elapsed := time.Since(start)
                        defer resp.Body.Close()

                        if err != nil {
                            fmt.Fprintf(os.Stderr, "Failed to connect to %s, %s\n", url, err)
                            responses <- Response { StatusCode: 500, Time: 10 * time.Second }
                        } else {
                            responses <- Response { StatusCode: resp.StatusCode, Time: elapsed }
                        }

                        if (*wait_duration >= 0) {
                            time.Sleep(time.Duration(*wait_duration) * time.Second)
                        }
                }
            }
            producers_wg.Done()
        }()
    }

    // Build the consumer that aggregates the stats
    consumers_wg.Add(1)
    go func () {
        for response := range(responses) {
            select {
                case <- os_signals:
                    fmt.Printf("Caught signal, quitting\n...")
                    for i := 0; i < *clients; i++ { timeout <- true }
                default:
                    if *verbose {
                        fmt.Printf("Received code %d time %d\n", response.StatusCode, response.Time)
                    }

                    response_stats.Total++
                    response_stats.TotalTime += float64(response.Time)

                    if int(response.Time) > response_stats.MaxTime {
                        response_stats.MaxTime = int(response.Time)
                    }
                    if int(response.Time) < response_stats.MinTime {
                        response_stats.MinTime = int(response.Time)
                    }

                    if (response.StatusCode >= 200 && response.StatusCode < 300) {
                        response_stats.Status2xx++
                    } else if (response.StatusCode >= 300 && response.StatusCode < 400) {
                        response_stats.Status3xx++
                    } else if (response.StatusCode >= 400 && response.StatusCode < 500) {
                        response_stats.Status4xx++
                    } else if (response.StatusCode >= 500 && response.StatusCode < 600) {
                        response_stats.Status5xx++
                    }
            }
        }
        consumers_wg.Done()
    }()

    producers_wg.Wait()
    close(responses)
    consumers_wg.Wait()

    // Record the elapsed time
    elapsed_time := time.Since(start_time)

    // Print out the response code histogram
    fmt.Printf("\nReponse codes:\n")
    fmt.Printf("[%s]:\t%d\n", "2xx", response_stats.Status2xx)
    fmt.Printf("[%s]:\t%d\n", "3xx", response_stats.Status3xx)
    fmt.Printf("[%s]:\t%d\n", "4xx", response_stats.Status4xx)
    fmt.Printf("[%s]:\t%d\n", "5xx", response_stats.Status5xx)

    fmt.Printf("\nTotal Requests:\t%d\n", response_stats.Total)
    fmt.Printf("Max time:\t%4.2fms\n", float64(response_stats.MaxTime) / float64(time.Millisecond))
    fmt.Printf("Min time:\t%4.2fms\n", float64(response_stats.MinTime) / float64(time.Millisecond))
    fmt.Printf("Avg time:\t%4.2fms\n", float64(response_stats.TotalTime) / float64(response_stats.Total) / float64(time.Millisecond))
    fmt.Printf("Total time:\t%4.2fs\n", float64(elapsed_time) / float64(time.Second))
    fmt.Printf("Avg Throughput:\t%4.2f req/s\n", float64(response_stats.Total) * float64(time.Second) / float64(elapsed_time))
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

    // Set up command line flags
    flag.Usage = usage
    flag.Parse()

    os_signals = make(chan os.Signal, 1)
    signal.Notify(os_signals)
}

func main() {
    var endpoints []string
    args := flag.Args()

    if *profile != "" {
        f, err := os.Create(*profile)
        if err != nil {
            log.Fatal(err)
        }
        pprof.StartCPUProfile(f)
        defer pprof.StopCPUProfile()
    }

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
