package main

import (
    "fmt"
    "bytes"
    "flag"
    "log"
    "path/filepath"
    "strconv"
    "os/exec"
    "os"
    "sync"
    "io"

    "httpGetter"
    ua "fake_useragent"
    "m3u8"
)

type mediaFile struct {
    URI string
    FilePath string
}

func main() {
    var (
        url, prefix, suffix string
    )
    uaFlag := flag.String("ua", "0", "select useragent, 0 is random, OPTIONS chrome, firefox, safari, edge")
    refererFlag := flag.String("r", "", "passing custom referer")
    asyncFlag := flag.Bool("async", false, "Download using async mode, make sure you have part binaryin the same directory of vidl")
    dThread := flag.Int("dThread", 10, "number of part to be split to download")
    output := flag.String("o", "untitle", "output file")
    thread := flag.Int("T", 10, "number of thread to use to download hls content")
    bDirect := flag.Bool("d", false, "direct download, passing T option will not have effect, using dThread for split file and download instead")
    flag.Parse()
    
    // Getting link and optional prefix, suffix to use while download hls
    fmt.Print("=> URL: ")
    fmt.Scanln(&url)
    // Only get prefix and suffix if not direct download
    if !*bDirect {
        fmt.Print("=> Prefix: ")
        fmt.Scanln(&prefix)
        fmt.Print("=> Suffix: ")
        fmt.Scanln(&suffix)
    }

    myUserAgent, err := ua.RandomUserAgent()
    if err != nil {
        log.Println(err)
    }
    if *uaFlag != "0" {
        myUserAgent, err = ua.GetUserAgent(*uaFlag)
        if err != nil {
            log.Println(err)
        }
    }

    d := &httpGetter.Downloader{
        Url: url,
        UserAgent: myUserAgent,
        Referer: *refererFlag,
        Output: *output,
        Thread: *dThread,
        Async: *asyncFlag,
    }

    if *bDirect {
        err = d.MultiGet()
        if err != nil {
            panic(err)
        }
        return
    } else {
        hlsDownloader(d, *thread, prefix, suffix)
    }
}

func hlsDownloader(d *httpGetter.Downloader, thread int, prefix, suffix string) {
    m3u8_stream, err := d.Get()
    if err != nil {
        log.Println("Failed to fetch m3u8 playlist")
        panic(err)
    }
    playlist, listType, err := m3u8.Decode(*bytes.NewBuffer(m3u8_stream), true)
    if err != nil {
        log.Println("Failed to decode m3u8 stream")
        panic(err)
    }

    switch listType {
	case m3u8.MEDIA:
		mediapl := playlist.(*m3u8.MediaPlaylist)
        downloadFromMediaPlaylist(d, thread, mediapl, prefix, suffix)
	case m3u8.MASTER:
		masterpl := playlist.(*m3u8.MasterPlaylist)
        mediapl := promptForMediaPlaylist(d, masterpl, prefix, suffix)
        downloadFromMediaPlaylist(d, thread, mediapl, prefix, suffix)
	}
}

func downloadFromMediaPlaylist(d *httpGetter.Downloader, thread int, mediapl *m3u8.MediaPlaylist, prefix, suffix string) {
    // segment files index
    var index int
    var wg sync.WaitGroup
    jobs := make(chan *mediaFile, thread)
    
    // remeber final output file
    outFile := d.Output + ".mp4"
    outDir := d.Output + "_d"
    // make directory to store all segments and list to pass in ffmpeg to merge file
    err := os.MkdirAll(outDir, 0755)
    if err != nil {
        panic(err)
    }
    // Start Workers
    for i := 0; i < thread; i++ {
        wg.Add(1)
        if d.Async {
            go asyncWorker(d, jobs, &wg)
        } else {
            go worker(d, jobs, &wg)
        }
    }

    // Add jobs to the job queue and write list for ffmpeg
    // Create listFile inside working directory
    listFile := filepath.Join(outDir, "list.txt")
    lf, err := os.OpenFile(listFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
    if err != nil {
        panic(err)
    }
    defer lf.Close()

    for _, segment := range mediapl.GetAllSegments() {
        filename := strconv.Itoa(index) + ".ts"
        // write to list in order
        _, err = lf.Write([]byte("file '" + filename + "'\n"))
        if err != nil {
            lf.Close()
            panic(err)
        }
        // create job with url and filepath to download it to
        job := &mediaFile {
            URI: prefix + ((*segment).URI) + suffix,
            FilePath: filepath.Join(outDir, filename),
        }
        jobs <- job
        index++
    }

    close(jobs)
    wg.Wait()
    // Merge .ts file to mp4 output with ffmpeg
    cmd := exec.Command("ffmpeg", "-f", "concat", "-i", listFile, "-c", "copy", "-bsf:a", "aac_adtstoasc", outFile)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    err = cmd.Run()
    if err != nil {
        log.Fatal(err)
    }
    // Cleanup
    fmt.Println("Cleaning up...")
    os.RemoveAll(outDir)
}

// This function use goroutine to download but the function return before io.Copy finish
// Need more investigate
// For now, using the binary in utils to download for multiprocessing
func worker(d *httpGetter.Downloader, jobs <-chan *mediaFile, wg *sync.WaitGroup) {
    defer wg.Done()
    for m := range jobs {
        c := &httpGetter.RequestConfig {
            Method: "GET",
            Url: m.URI,
            UserAgent: d.UserAgent,
            Referer: d.Referer,
        }
        resp, err := c.Request()
        if err != nil {
            log.Println("Failed to request", m.URI)
            continue
        }

        out, err := os.OpenFile(m.FilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0655)
        if err != nil {
            log.Println("Failed to open", m.FilePath)
            continue
        }

        // Create a custom buffer
        buffer := make([]byte, 2048)
        // Copy content to output file
        _, err = io.CopyBuffer(out, resp.Body, buffer)
        defer resp.Body.Close()
        defer out.Close()
        if err != nil {
            log.Println("Failed to copy", m.FilePath, "\nError:", err)
            continue
        }
        fmt.Println("Finish", m.FilePath)
    }
}


func asyncWorker(d *httpGetter.Downloader, jobs <-chan *mediaFile, wg *sync.WaitGroup) {
    defer wg.Done()
    for m := range jobs {
        cmd := exec.Command("./utils/getFileUtil",
            "-u", m.URI,
            "-ua", d.UserAgent,
            "-r", d.Referer,
            "-o", m.FilePath)
        err := cmd.Run()
        if err != nil {
            log.Println("Failed to get", m.FilePath, "\nError:", err)
            panic(err)
        }
    }
}

func promptForMediaPlaylist(d *httpGetter.Downloader, masterpl *m3u8.MasterPlaylist, prefix, suffix string) *m3u8.MediaPlaylist {
    var i, selection int = 1, 0
    for _, v := range masterpl.Variants {
        fmt.Printf("- %d) CODECS:%s   RESOLUTION:%s\n", i, (*v).Codecs,(*v).Resolution)
        i++
    }
    for selection == 0 || selection > i{
        fmt.Print("=> Choose media to download: ")
        fmt.Scanln(&selection)
    }
    // decrease selection index by 1 to use with array
    selection--
    // Retrieve media playlist from url
    d.Url = prefix + masterpl.Variants[selection].URI + suffix
    m3u8_stream, err := d.Get()
    if err != nil {
        log.Println("Failed to fetch media playlist")
        panic(err)
    }
    p, _, err := m3u8.Decode(*bytes.NewBuffer(m3u8_stream), true)
    if err != nil {
        log.Println("Failed to decode media playlist stream")
        panic(err)
    }
    return p.(*m3u8.MediaPlaylist)
}
