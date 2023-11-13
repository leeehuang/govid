package progressbar

import (
    "fmt"
    "strings"
    "time"
    "io"
    "os"
    "math"
)

type ProgressBar struct {
    Length  int
    CurrentPos  int
    Unit    string
    SpeedUnit   string
    CurrentVal  float64
    Total   float64
    Sensible_total float64
    Description string
    Speed   float64
    Elapsed time.Duration
    ETA     time.Duration
    Writer  io.Writer
}

const (
    SaucerPadding string = " "
    Saucer  string = "="
    SaucerHead  string = ">"
)

var (
    // start time to calculate duration
    tStart time.Time
    interval = 1 * time.Second / 5
)

func Default(length int, total int64, description string) *ProgressBar {
    // init and return default bar
    return &ProgressBar {
        Length: length,
        Unit: "it",
        SpeedUnit: "it",
        Total: float64(total),
        Description: description,
        Writer: os.Stderr,
    } 
}

func DefaultBytes(length int, total int64, description string) *ProgressBar {
    // get sensible unit and total size
    sensible_total, sensible_unit := humanizeBytes(float64(total), true)
    // init and return default byte bar
    return &ProgressBar {
        Length: length,
        Unit: sensible_unit,
        Total: float64(total),
        Sensible_total: sensible_total,
        Description: description,
    }
}

// Write implement io.Writer
func (b *ProgressBar) Write(s []byte) (n int, err error) {
    n = len(s)
    b.Add(int64(n))
    return
}

func (b *ProgressBar) Add(val int64) {
    if b.CurrentVal == 0 {
        // init start time
        tStart = time.Now()
        // Start the speed monitor
        go b.SpeedMonitor()
        fmt.Printf("\n%s:\n", b.Description)
        b.DrawBar()
    }
    b.CurrentVal += float64(val)
    b.CurrentPos = int(b.CurrentVal * float64(b.Length) / b.Total)
    //err := b.Update()
    //if err != nil {
    //    log.Panicln("Failed to update progress bar\n", err)
    //}
}

func (b *ProgressBar) Update() {
    ClearLine()
    // Get the info to update the bar
    b.GetInfo()
    // update position
    b.DrawBar()
}

func (b *ProgressBar) GetInfo() {
    // Calculate Elapsed time
    tCurrent := time.Now()
    b.Elapsed = tCurrent.Sub(tStart)
    b.ETA = time.Second * time.Duration((b.Total - b.CurrentVal) / b.Speed)
}

func (b *ProgressBar) SpeedMonitor() {
    for b.CurrentVal <= b.Total {
        lastSpeed := b.Speed
        last := b.CurrentVal
        time.Sleep(interval)
        b.Speed = (b.CurrentVal - last) / interval.Seconds()
        if b.Speed == 0 {
            b.Speed = lastSpeed
        }
        b.Update()
    }
    b.Finish()
}

func (b *ProgressBar) Finish() {
    b.CurrentPos = b.Length
    b.Update()
    fmt.Printf("\n\r")
}

// humanizeBytes() return the value and unit for the value to make it more sensible
func humanizeBytes(s float64, iec bool) (float64, string) {
	sizes := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}
	base := 1000.0

	if iec {
		sizes = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
		base = 1024.0
	}

	if s < 10 {
        return s, sizes[0]
	}
	e := math.Floor(logn(float64(s), base))
	suffix := sizes[int(e)]
	val := math.Floor(float64(s)/math.Pow(base, e)*10+0.5) / 10
	return val, suffix
}

func logn(n, b float64) float64 {
	return math.Log(n) / math.Log(b)
}

func ClearLine() {
    // Clear the line
    fmt.Printf("\033[2K\r")
    // return moves the cursor to the beginning of the line
    //fmt.Printf("\r")
}

func (b *ProgressBar) DrawBar() {
    sHead := SaucerHead
    if b.CurrentVal == b.Total {
        sHead = "="
    }
    padding := strings.Repeat(SaucerPadding, b.Length - b.CurrentPos)
    prog := strings.Repeat(Saucer, b.CurrentPos)
    if b.Unit == "it" {
        fmt.Printf("[%s%s%s] %v/%v (%v %s/s) [%v ETA=%v]", prog, sHead, padding, b.CurrentVal, b.Total, b.Speed, b.SpeedUnit, b.Elapsed.Round(time.Second), b.ETA)
    } else {
        // Convert speed to sensible value
        speed, speedUnit := humanizeBytes(b.Speed, true)
        val, valUnit := humanizeBytes(b.CurrentVal, true)
        fmt.Printf("[%s%s%s] %.1f %s/%.1f %s (%.1f %s/s) [%v ETA=%v]", prog, sHead, padding, val, valUnit, b.Sensible_total, b.Unit, speed, speedUnit, b.Elapsed.Round(time.Second), b.ETA)
    }
}
