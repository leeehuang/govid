package httpGetter

import (
    "net/http"
    "io"
    "os"
    "errors"
    "fmt"
    "sync"
    "os/exec"
    "strconv"
    "log"
    "path/filepath"
    
    "progressbar"
)

type RequestConfig struct {
    Method string
    Url string
    UserAgent string
    Referer string
    Range RequestRange
}

type RequestRange struct {
    Off int64
    Lim int64
}

type Downloader struct {
    Url string
    UserAgent string
    Referer string
    Output string
    Thread int
    Async bool
}

const (
    defaultUserAgent string = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.54 Safari/537.36"
    defaultReferer string = "https://www.google.com"
)

var (
    rangeSupportErr = errors.New("Server does not support range requests")
    invalidLengthErr = errors.New("Server sent invalid Content-Length header")
)

// Request() do request with passing Method such as GET, HEAD
func (c *RequestConfig) Request() (*http.Response, error) {
    req, err := http.NewRequest(c.Method, c.Url, nil)
    if err != nil {
        return nil, err
    }

    if c.UserAgent == "" {
        c.UserAgent = defaultUserAgent
    }
    req.Header.Set("User-Agent", c.UserAgent)
    if c.Referer == "" {
        c.Referer = defaultReferer
    }
    req.Header.Add("Referer", c.Referer)

    if c.Range.Lim != 0 {
        req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", c.Range.Off, c.Range.Lim))
    }

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    
    // if status is not ok or PartialContent(206) return non-nil error
    if resp.StatusCode != 200 && resp.StatusCode != 206 {
        err = fmt.Errorf("Receive unexpect status: %d", resp.StatusCode)
        resp.Body.Close()
        return nil, err
    }

    return resp, err
}

// Size() get the size of the response from the url
func (d *Downloader) Size() (int64, error) {
    c := &RequestConfig {
        Method: "HEAD",
        Url: d.Url,
        UserAgent: d.UserAgent,
        Referer: d.Referer,
    }
    resp, err := c.Request()
    if err != nil {
        return 0, err
    }
    // if response dont have Accept-Ranges return rangeSupportErr
    if resp.Header.Get("Accept-Ranges") != "bytes" {
        err = rangeSupportErr
    }
    // check invalid Length
    if resp.ContentLength < 0 {
        return 0, invalidLengthErr
    }
    return resp.ContentLength, err
}

// Get() get the response to a []byte
func (d *Downloader) Get() ([]byte, error) {
    c := &RequestConfig {
        Method: "GET",
        Url: d.Url,
        UserAgent: d.UserAgent,
        Referer: d.Referer,
    }
    resp, err := c.Request()
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    return io.ReadAll(resp.Body)
}

// GetFile() get the response to a file
func (d *Downloader) GetFile() (int64, error) {
    c := &RequestConfig {
        Method: "GET",
        Url: d.Url,
        UserAgent: d.UserAgent,
        Referer: d.Referer,
    }
    resp, err := c.Request()
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()

    out, err := os.OpenFile(d.Output, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        log.Println("Failed to open output file")
        return 0, err
    }
    defer out.Close()
    // Init the progress bar
    bar := progressbar.DefaultBytes(45, resp.ContentLength, d.Output)
    // Create a buffer
    buffer := make([]byte, 32000)
    n, err := io.CopyBuffer(io.MultiWriter(out, bar), resp.Body, buffer)
    return n, err
}

func (d *Downloader) MultiGet() error {
    var index int
    total, err := d.Size()
    if err != nil {
        if err == rangeSupportErr {
            _, err = d.GetFile()
            return err
        }
        log.Println("Failed to get size of response")
        return err
    }
    
    sectionLen := total / int64(d.Thread - 1)
    var wg sync.WaitGroup
    wg.Add(d.Thread)

    var bar *progressbar.ProgressBar
    // Init progress bar for async part
    if d.Async {
        bar = progressbar.Default(45, int64(d.Thread), "Downloading " + d.Output)
    } else {
    // init progress bar for partAt
        bar = progressbar.DefaultBytes(45, total, "Downloading " + d.Output)
    }

    for offset := int64(0); offset < total; offset += sectionLen {
        offset := offset
        limit := offset + sectionLen
        if limit >= total {
            limit = total
        }
        if d.Async {
            go d.AsyncPart(index, offset, limit, &wg, bar)
        } else {
            go d.PartAt(offset, limit, &wg, bar)
        }
        index++
    }

    wg.Wait()
    if d.Async {
        err = d.MergeAsyncPart(index, sectionLen)
        if err != nil {
            log.Println("Failed to merge file")
            panic(err)
        }
    }
    return err
}

// Part() use by external part binary for async download
// Download on file, if already exist, it write over it
func (d *Downloader) Part(off int64, lim int64) {
    c := &RequestConfig {
        Method: "GET",
        Url: d.Url,
        UserAgent: d.UserAgent,
        Referer: d.Referer,
        Range: RequestRange{
            Off: off,
            Lim: lim,
        },
    }
    resp, err := c.Request()
    if err != nil {
        log.Printf("Failed to get %s\n", d.Url)
        panic(err)
    }
    out, err := os.OpenFile(d.Output, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        log.Printf("Failed to open %s\n", d.Output)
        panic(err)
    }
    defer out.Close()
    // Create a buffer
    buffer := make([]byte, 32000)
    // Copy to new file
    _, err = io.CopyBuffer(out, resp.Body, buffer)
    if err != nil {
        log.Printf("Failed to copy to %s\n", d.Output)
        panic(err)
    }
    defer resp.Body.Close()
}

// AsyncPart() run the external binary to download separate file
func (d *Downloader) AsyncPart(suffix int, off, lim int64, wg *sync.WaitGroup, bar *progressbar.ProgressBar) {
    defer wg.Done()
    outFile := d.Output + "_" + strconv.Itoa(suffix)
    homedir, err := os.UserHomeDir()
    if err != nil {
        log.Panic(err)
    }
    utilBin := filepath.Join(".", homedir, ".local/bin/utils/part")
    cmd := exec.Command(utilBin,
        "-url", d.Url,
        "-ua", d.UserAgent,
        "-r", d.Referer,
        "-o", outFile,
        "-off", strconv.FormatInt(off, 10),
        "-l", strconv.FormatInt(lim, 10) )
    err = cmd.Run()
    if err != nil {
        panic(err)
    }
    bar.Add(1)
}

// PartAt() copy to one file but at the offset
func (d *Downloader) PartAt(off int64, lim int64, wg *sync.WaitGroup, bar *progressbar.ProgressBar) {
    defer wg.Done()
    c := &RequestConfig {
        Method: "GET",
        Url: d.Url,
        UserAgent: d.UserAgent,
        Referer: d.Referer,
        Range: RequestRange{
            Off: off,
            Lim: lim,
        },
    }
    resp, err := c.Request()
    if err != nil {
        log.Printf("Failed to get %s\n", d.Url)
        panic(err)
    }
    out, err := os.OpenFile(d.Output, os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        log.Printf("Failed to open %s\n", d.Output)
        panic(err)
    }
    defer out.Close()
    // Create a buffer
    buffer := make([]byte, 32000)
    // Copy to file at offset
    sectionWriter := io.NewOffsetWriter(io.WriterAt(out), off)
    _, err = io.CopyBuffer(io.MultiWriter(sectionWriter, bar), resp.Body, buffer)
    if err != nil {
        log.Printf("Failed to copy to %s at %d", d.Output, off)
        panic(err)
    }
    defer resp.Body.Close()
}

func (d *Downloader) MergeAsyncPart(num int, sectionLen int64) error {
    var offset int64
    var wg sync.WaitGroup
    wg.Add(num)
    outFile, err := os.Create(d.Output)
    if err != nil {
        log.Printf("Failed to create output file: %s\n", d.Output)
        return err
    }
    defer outFile.Close()
    // Init progress bar for init process
    bar := progressbar.Default(45, int64(num), "Merging")
    // Loop through all section files and write to file
    for i := 0; i < num; i++ {
        section := d.Output + "_" + strconv.Itoa(i)
        sectionFile, err := os.Open(section)
        if err != nil {
            log.Printf("Failed to open %s\n", section)
            return err
        }
            go writeToOffset(outFile, sectionFile, offset, &wg, bar)
            offset += sectionLen
        }
    wg.Wait()
    return err
}

func writeToOffset(outFile, sectionFile *os.File, offset int64, wg *sync.WaitGroup, bar *progressbar.ProgressBar) {
    defer wg.Done()
    // Create a buffer
    buffer := make([]byte, 32000)
    // Copy to file at offset
    sectionWriter := io.NewOffsetWriter(io.WriterAt(outFile), offset)
    _, err := io.CopyBuffer(sectionWriter, sectionFile, buffer)
    if err != nil {
        log.Printf("Failed to copy to %s at %d\n", outFile.Name(), offset)
        panic(err)
    }
    sectionFile.Close()
    err = os.Remove(sectionFile.Name())
    if err != nil {
        log.Printf("Failed to remove %s\nError: %v\n", sectionFile.Name(), err)
    }
    bar.Add(1)
}
