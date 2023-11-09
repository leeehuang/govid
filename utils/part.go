package main

import (
    "flag"

    "httpGetter"
)

func main() {
    url := flag.String("url", "", "")
    userAgent := flag.String("ua", "", "")
    referer := flag.String("r", "", "")
    output := flag.String("o", "", "")
    off := flag.Int64("off", 0, "")
    lim := flag.Int64("l", 0, "")
    flag.Parse()

    d := &httpGetter.Downloader{
        Url: *url,
        UserAgent: *userAgent,
        Referer: *referer,
        Output: *output,
    }

    d.Part(*off, *lim)
}
