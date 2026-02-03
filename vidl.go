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
    "strings"

    "httpGetter"
    ua "fake_useragent"
    "progressbar"
    "m3u8"
)

type mediaFile struct {
    URI string
    FilePath string
}

func main() {

    var (
        output, url, prefix, suffix string
    )
    uaFlag := flag.String("ua", "0", "select useragent, 0 is random, OPTIONS chrome, firefox, safari, edge")
    refererFlag := flag.String("r", "", "passing custom referer")
    asyncFlag := flag.Bool("async", false, "Download using async mode, make sure you have part binaryin the same directory of vidl")
    dThread := flag.Int("dThread", 10, "number of part to be split to download")
    thread := flag.Int("T", 10, "number of thread to use to download hls content")
    bDirect := flag.Bool("d", false, "direct download, passing T option will not have effect, using dThread for split file and download instead")
    bCleanup := flag.Bool("c", false, "Clean up temp folder afterward")
    bConvert := flag.Bool("convert", false, "Attempt to convert .ts to .mp4, if the file download seem to be PNG, use vidl-header-convert to attempt to convert to mp4")
    flag.Parse()

    // Getting output file name
    fmt.Print("=> Output: ")
    fmt.Scanln(&output)
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
        Output: output,
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
        hlsDownloader(d, *thread, prefix, suffix, *bCleanup, *bConvert)
    }
}


func hlsDownloader(d *httpGetter.Downloader, thread int, prefix, suffix string, bCleanup, bConvert bool) {
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
        if (mediapl.Key != nil) {
            log.Println(*mediapl.Key)
        }
        downloadFromMediaPlaylist(d, thread, mediapl, prefix, suffix, bCleanup, bConvert)
	case m3u8.MASTER:
		masterpl := playlist.(*m3u8.MasterPlaylist)
        mediapl, newPrefix, newSuffix := promptForMediaPlaylist(d, masterpl, prefix, suffix)
        downloadFromMediaPlaylist(d, thread, mediapl, newPrefix, newSuffix, bCleanup, bConvert)
	}
}

func downloadFromMediaPlaylist(d *httpGetter.Downloader, thread int, mediapl *m3u8.MediaPlaylist, prefix, suffix string, bCleanup, bConvert bool) {
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

    //Get Key and IV for encrypted media
    if mediapl.Key != nil {
        utilBin := filepath.Join("./utils/getFileUtil")
        keyFile := filepath.Join(outDir, "ts.key")

        cmd := exec.Command(utilBin,
            "-u", prefix + "/" + mediapl.Key.URI,
            "-ua", d.UserAgent,
            "-r", d.Referer,
            "-o", keyFile)
        err := cmd.Run()
        if err != nil {
            log.Println("Failed to get", keyFile, "\nError:", err)
            panic(err)
        }

        ivFile :=filepath.Join(outDir, "iv")
        ivf, err := os.OpenFile(ivFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
        if err != nil {
            panic(err)
        }
        _, err = ivf.WriteString(mediapl.Key.IV)
        if err != nil {
            panic(err)
        }

    }



    // Init progress bar
    bar := progressbar.Default(45, int64(len(mediapl.GetAllSegments())), outFile)
    // Start Workers
    for i := 0; i < thread; i++ {
        wg.Add(1)
        if d.Async {
            go asyncWorker(d, jobs, &wg, bar)
        } else {
            go worker(d, jobs, &wg, bar)
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
    bar.Finish()
    // Merge .ts file to mp4 output with ffmpeg

    if bConvert {
        // decrypt if need
        if mediapl.Key != nil {
            descryptCmd:= exec.Command("./m3u8_decrypt.sh", outDir, strings.TrimPrefix(mediapl.Key.IV, "0x"))
            descryptCmd.Stdout = os.Stdout
            descryptCmd.Stderr = os.Stderr
            err = descryptCmd.Run()
            if err != nil {
                log.Fatal(err)
            }
        }

        cmd := exec.Command("ffmpeg", "-f", "concat", "-i", listFile, "-c", "copy", "-bsf:a", "aac_adtstoasc", outFile)
        cmd.Stdout = os.Stdout
        cmd.Stderr = os.Stderr
        err = cmd.Run()
        if err != nil {
            log.Fatal(err)
        }
    }
    // Cleanup
    fmt.Println("\n=> Finish: ", outFile)
    if bCleanup {
        fmt.Println("=> Cleaning up...")
        os.RemoveAll(outDir)
    }
}

// This function use goroutine to download but the function return before io.Copy finish
// Need more investigate
// For now, using the binary in utils to download for multiprocessing
func worker(d *httpGetter.Downloader, jobs <-chan *mediaFile, wg *sync.WaitGroup, bar *progressbar.ProgressBar) {
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
        //fmt.Println("Finish", m.FilePath)
        bar.Add(1)
    }
}


func asyncWorker(d *httpGetter.Downloader, jobs <-chan *mediaFile, wg *sync.WaitGroup, bar *progressbar.ProgressBar) {
    homedir, err := os.UserHomeDir()
    if err != nil {
        log.Panic(err)
    }
    utilBin := filepath.Join(".", homedir, ".local/bin/utils/getFileUtil")
    defer wg.Done()
    for m := range jobs {
        cmd := exec.Command(utilBin,
            "-u", m.URI,
            "-ua", d.UserAgent,
            "-r", d.Referer,
            "-o", m.FilePath)
        err := cmd.Run()
        if err != nil {
            log.Println("Failed to get", m.FilePath, "\nError:", err)
            panic(err)
        }
        bar.Add(1)
    }
}

func promptForMediaPlaylist(d *httpGetter.Downloader, masterpl *m3u8.MasterPlaylist, prefix, suffix string) (*m3u8.MediaPlaylist, string, string) {
    var i, selection int = 1, 0
    var newPrefix, newSuffix string
    for _, v := range masterpl.Variants {
        fmt.Printf("- %d) CODECS:%s   RESOLUTION:%s\n[ URI:%s ]\n", i, (*v).Codecs, (*v).Resolution, (*v).URI)
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

    // Getting new prefix for media file
    fmt.Println("=> Input new Prefix and Suffix for media file\n    Input 'same' to reuse prefix or suffix")
    fmt.Print("=> Prefix: ")
    fmt.Scanln(&newPrefix)
    fmt.Print("=> Suffix: ")
    fmt.Scanln(&newSuffix)
    // Check if prefix and suffix the same
    if newPrefix == "same" {
        newPrefix = prefix
    }
    if newSuffix == "same" {
        newSuffix = suffix
    }
    return p.(*m3u8.MediaPlaylist), newPrefix, newSuffix
}
