package main

import (
    "net/http"
    "log"
    "encoding/json"
    "strconv"
    "strings"
    "os"

    "github.com/PuerkitoBio/goquery"
)

type Agent struct {
    Percent float32
    UserAgent string
    System string
    Browser string
    Version float32
    OS string
}

var userAgents = make(map[string][]Agent) 

func main() {
    var url string = "https://webcache.googleusercontent.com/search?q=cache:https://techblog.willshouse.com/2012/01/03/most-common-user-agents/&sca_esv=580610545&strip=1&vwsrc=0"
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        panic(err)
    }
    
    req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.54 Safari/537.36")

    client := &http.Client{}
    res, err := client.Do(req)
    if err != nil {
        log.Fatal(err)
    }
    defer res.Body.Close()

    doc, err := goquery.NewDocumentFromReader(res.Body)
    if err != nil {
        log.Println("Failed to parse the document")
        log.Fatal(err)
    }

    doc.Find("tr").Each(func(i int, result *goquery.Selection) {
        PopulateUserAgentList(i, result)
    })

    // Output to json file
    f, err := os.OpenFile("../browsers.json", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        log.Fatal("Failed to open JSON file:\n", err)
    }
    defer f.Close()

    b, err := json.MarshalIndent(userAgents, "", "  ")
    if err != nil {
        log.Println(err)
    }

    n, err := f.Write(b)
    if err != nil {
        log.Println("Failed to write to JSON file:\n", err)
    }
    log.Printf("%dB Written\n", n)
}

func PopulateUserAgentList(i int, result *goquery.Selection) {
    // system string
    system := result.Find("td.system").First().Text()
    // [browser, version, os]
    sysinfo := strings.Fields(system)
    if len(sysinfo) > 3 || system == ""{
        return
    }

    // Remove percentage icon & convert to float
    p := result.Find("td.percent").First().Text()
    p = strings.TrimSuffix(p, "%")
    var percent float32 = ToFloat32(p)
    // Useragent string
    useragent := result.Find("td.useragent").First().Text()
    
    // Browser
    browser := strings.ToLower(sysinfo[0])
    // Version
    // If version equals Generic, just set version to 1.0
    var version float32
    if sysinfo[1] == "Generic" {
        version = 1.0
    } else {
        version = ToFloat32(sysinfo[1])
    }
    // OS
    os := strings.ToLower(sysinfo[2])
    userAgents[browser] = append(userAgents[browser], Agent{
        Percent: percent,
        UserAgent: useragent,
        System: system,
        Browser: browser,
        Version: version,
        OS: os,
    })
}

func ToFloat32(s string) float32 {
    tmp, err := strconv.ParseFloat(s, 32)
    if err != nil {
        log.Fatal("Failed to convert to float:\n", err)
    }
    return float32(tmp)
}
