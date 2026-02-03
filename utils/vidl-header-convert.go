package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
    "strings"
    "errors"
)

func main() {
    dir := flag.String("d", "", "diretory created by vidl, end with _d")
    inputOffset := flag.Int64("offset", -1, "offset to \\x47\\x40 which is start of .ts header")
    noConvert := flag.Bool("no-convert", false, "don't convert to mp4 right after (for debug)")
    flagOutfile := flag.String("o", "", "convert output file location ( Default to directory with the same name )")
    bVerbose := flag.Bool("v", false, "Verbose output mode")
    flag.Parse()

    cwd, err := os.Getwd()
    if err != nil {
        log.Println(err)
    }
    cwdPath, err := filepath.Abs(cwd)
    errHandler(err)
    workPath, err := filepath.Abs(*dir)
    errHandler(err)
    if cwdPath == workPath {
        fmt.Println("\033[1;34mWARNING: Running from inside working dir!\nWARNING: Cleaning up might delete output file!\nMake sure output file will be save somewhere else with '-o'\033[0m")
    }
    f, err := os.Open(*dir)
    errHandler(err)
    defer f.Close()
    files, err := f.Readdir(0)
    errHandler(err)

    listFile := filepath.Join(*dir, "list.txt")
    defaultOutFile, _ := strings.CutSuffix(filepath.Base(*dir), "_d")
    outFile := defaultOutFile + ".mp4"
    if *flagOutfile != "" {
        outFile = *flagOutfile
    }

    foundOffset := *inputOffset
    if *inputOffset < 0 {
        fmt.Println("No offset input or offset is negative!")
        fmt.Println("Attempting to find the correct offset...")
        foundOffset = FindOffsetToTS(filepath.Join(*dir, files[0].Name()))
    } else if *inputOffset == 0 {
        fmt.Print("Offset is 0")
        if !(*noConvert) {
            fmt.Println(", converting...")
            Convert(listFile, outFile)
            CleanUp(*dir)
            return
        } else {
            fmt.Println(", exiting...")
            return
        }
    }

    counter := 0
    for _, v := range files {
        if v.Name() != "list.txt" {
            removeFakeHeader(filepath.Join(*dir, v.Name()), foundOffset)
            counter++
            if *bVerbose {
                fmt.Printf("Done: %s\n", v.Name())
            }
        }
    }

    if *bVerbose {
        fmt.Printf("File Total: %d\n", counter)
    }

    
    if !(*noConvert) {
       Convert(listFile, outFile) 
       fmt.Printf("Finished convert %s\n", outFile)
       CleanUp(*dir)
    }
}

func Convert (listFile, outFile string) {
    cmd := exec.Command("ffmpeg", "-f", "concat", "-i", listFile, "-c", "copy", "-bsf:a", "aac_adtstoasc", outFile)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    err := cmd.Run()
    if err != nil {
        log.Fatal(err)
    }
}

func CleanUp(workingDir string) {
    var answer string
    fmt.Print("Do you want to clean up work directory? (Y/n) ")
    fmt.Scanln(&answer)
    if strings.ToLower(answer) == "n" {
        return
    }
    os.RemoveAll(workingDir)
}

func FindOffsetToTS(tsFile string) int64 {
    var foundOffset int64 = 0
    f, err := os.Open(tsFile)
    errHandler(err)

    b := make([]byte, 1)
    var first bool = false
    for {
        _, err := f.Read(b)
        if err != nil && !errors.Is(err, io.EOF) {
            log.Fatal(err)
        }
        // Try to find first instance of \x47\x40
        if b[0] == 0x47 {
           first = true
           foundOffset++
           continue
        }
        if b[0] == 0x40 && first {
            fmt.Println("Possible offset for TS header found!")
            fmt.Printf("Offset: \033[1;36m%d\n\033[0m", foundOffset-1)
            break
        }
        first = false
        foundOffset++

        if err != nil {
            // end of file
            log.Fatal("Can't find offset for TS header! Exiting...")
            break
        }
    }

    return foundOffset-1
}

func removeFakeHeader(tsFile string, offset int64) {
    ts, err := os.Open(tsFile)
    errHandler(err)
    defer ts.Close()

    new_ts, err := os.Create(tsFile + ".new")
    errHandler(err)
    defer new_ts.Close()

    _, err = ts.Seek(offset, io.SeekStart)
    errHandler(err)
    
    _, err = io.Copy(new_ts, ts)

    err = os.Remove(tsFile)
    errHandler(err)
    err = os.Rename(tsFile + ".new", tsFile)
    errHandler(err)
}

func errHandler(err error) {
    if err != nil {
        log.Println(err)
        panic(err)
    }
}
