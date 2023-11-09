package main

import (
    "flag"

    "httpGetter"
)

func main() {
    url := flag.String("u", "", "")
    ua := flag.String("ua", "", "")
    referer := flag.String("r", "", "")
    out := flag.String("o", "", "")
    flag.Parse()

    d := &httpGetter.Downloader{
        Url: *url,
        UserAgent: *ua,
        Referer: *referer,
        Output: *out,
    }

    _, err := d.GetFile()
    if err != nil {
        panic(err)
    }
}
