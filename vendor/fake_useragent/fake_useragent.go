package fake_useragent

import (
    "math/rand"
    "time"
    "encoding/json"
    "os"
    "log"
    "errors"
//    "path/filepath"
)

type Agent struct {
    Percent float32
    UserAgent string
    System string
    Browser string
    Version float32
    OS string
}

var (
    userAgents = make(map[string][]Agent) 
    randSource = rand.NewSource(time.Now().UnixNano())
    randGenerator = rand.New(randSource)
)

// Populate to access useragent list
func init() {
    //homedir, err := os.UserHomeDir()
    //if err != nil {
    //    log.Panic(err)
    //}
    dataFile := "./utils/browsers.json"
    data, err := os.ReadFile(dataFile)
    if err != nil {
        log.Panic(err)
    }

    err = json.Unmarshal(data, &userAgents)
    if err != nil {
        log.Fatal("Failed to unmarshal json file:", err)
    }
}

// GetUserAgent retrieves a random agent for the specified browser
func GetUserAgent(browser string) (string, error) {
    agents, ok := userAgents[browser]
    if !ok {
        err := errors.New("Browser is not existed in the list!") 
        return "", err
    }

    randomIndex := randGenerator.Intn(len(agents))
    return agents[randomIndex].UserAgent, nil
}

// RandomUserAgent retrieves a random useragen from the list
func RandomUserAgent() (string, error) {
    browsers := make([]string, 0, len(userAgents))
    for browser := range userAgents {
        browsers = append(browsers, browser)
    }

    randomIndex := randGenerator.Intn(len(browsers))
    return GetUserAgent(browsers[randomIndex])
}
