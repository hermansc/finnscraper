package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"
)

type Config struct {
	Url       string
	Interval  int
	ToEmail   string
	FromEmail string
	UserAgent string
	Template  string
	Debug     bool
	FirstRun  bool
}

var config Config
var configLocation string
var seen map[string]bool

func printHelp() {
	fmt.Fprintf(os.Stderr, "Specify config-file:\n%s scraper.conf\n", os.Args[0])
}

func loadConfig(filename string) error {
	f, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	lines := strings.Split(string(f), "\n")

	// Some naive checks.
	if len(lines) < 2 {
		errors.New("Please check your configuration file. It looks to short.")
	}

	for _, line := range lines {
		// Ignore comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		params := strings.Split(line, " ")

		// Read the URL
		if params[0] == "url" {
			config.Url = params[1]
		}

		// Read the interval.
		if params[0] == "interval" {
			interval, err := strconv.Atoi(params[1])
			if err != nil {
				return err
			}
			config.Interval = interval
		}

		// Read the mails
		if params[0] == "tomail" {
			config.ToEmail = params[1]
		}
		if params[0] == "frommail" {
			config.FromEmail = params[1]
		}

		// Check if useragent is defined.
		if params[0] == "useragent" {
			config.UserAgent = params[1]
		}

		// Check if we have defined our own template.
		if params[0] == "template" {
			config.Template = params[1]
		}

		// Read the debug-param
		if params[0] == "debug" {
			debug, err := strconv.ParseBool(params[1])
			if err != nil {
				return err
			}
			config.Debug = debug
		}
	}

	// We just loaded a presumably new config. So we start a new run.
	config.FirstRun = true

	// Check that the config is OK.
	if config.ToEmail == "" || config.FromEmail == "" || config.Url == "" {
		return errors.New("Invalid configuration. You need to provide To/From-emails and Url")
	}
	if config.Interval < 1 {
		return errors.New("Interval is to small. Set 'interval' to a value larger than zero")
	}
	if config.UserAgent == "" {
		config.UserAgent = "Mozilla/5.0 (Windows NT 5.1; rv:31.0) Gecko/20100101 Firefox/31.0"
	}
	if config.Template == "" {
		config.Template = "default.tmpl"
	}

	// Check that the template is parsable.
	_, err = parseTemplate(config.Template, nil)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Everything is OK.
	return nil
}

func handleSignals() {
	// Make chan listening for signals, and redirect all signals to this chan.
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)

	// Anonymous go-thread listening for syscalls.
	go func() {
		for sig := range c {
			if sig == syscall.SIGHUP {
				// Reload the config.
				err := loadConfig(configLocation)
				if err != nil {
					log.Println(err.Error())
				}
				log.Println("Loaded new config after SIGHUP")
				// Run a check, right away.
				err = checkFinn()
				if err != nil {
					log.Println(err.Error())
				}
			}
		}
	}()
}

func sendMail(to, from, content string) error {
	err := smtp.SendMail("localhost:25",
		nil,
		from,
		[]string{to},
		[]byte(content))
	if err != nil {
		return err
	}
	return nil
}

func getMailContent(ads []string) (string, error) {
	// Add the strings of all ads on the dict sent to the template.
	d := make(map[string]interface{})
	d["Ads"] = strings.TrimRight(strings.Join(ads, ""), "\n\r")
	d["NumResults"] = len(ads)
	d["SearchURL"] = config.Url

	content, err := parseTemplate(config.Template, d)
	if err != nil {
		return content, err
	}

	return content, nil
}

func parseTemplate(filename string, data interface{}) (string, error) {
	var buf bytes.Buffer
	t, err := template.ParseFiles(filename)
	if err != nil {
		return buf.String(), err
	}
	err = t.Execute(&buf, data)
	if err != nil {
		return buf.String(), err
	}
	return buf.String(), nil
}

func checkFinn() error {
	if config.Debug {
		log.Println("Checking provided URL...")
	}

	// For saving the non-seen new ads.
	newAds := make([]string, 0)

	// Open the provided URL.
	client := &http.Client{}
	req, err := http.NewRequest("GET", config.Url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", config.UserAgent)
	resp, err := client.Do(req)
	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		return err
	}

	// Find all results on the HTML page.
	results := doc.Find("div[data-automation-id='adList']")
	results.ChildrenFiltered("a").EachWithBreak(func(i int, s *goquery.Selection) bool {

		// If it is a promoted ad, we ignore it.
		if s.HasClass("bg-promoted") {
			return true
		}

		finncode, _ := s.Attr("id")
		if _, ok := seen[finncode]; !ok {
			// Construct a devent ad header.
			title := strings.TrimSpace(s.Find("div[data-automation-id='titleRow']").Text())
			price := strings.TrimSpace(s.Find("span[data-automation-id='bodyRow']").Text())
			if price == "" {
				price = "0,-"
			}
			finn_url := "www.finn.no/finn/finncode/result?finncode="
			adHeader := fmt.Sprintf("%v (%v) - %v%v\n", title, price, finn_url, finncode)

			// Add the ad to our data structures, saving it.
			newAds = append(newAds, adHeader)
			seen[finncode] = true

			// We assume there are no more than 5 new ads in one interval.
			if i+1 == 5 && !(config.FirstRun) {
				return false
			}
		}
		return true
	})

	// We've found new ads, send them to the designated e-mail.
	if len(newAds) > 0 && !(config.FirstRun) {
		to := fmt.Sprintf("To: %s\r\n", config.ToEmail)
		from := fmt.Sprintf("From: %v\r\n", config.FromEmail)
		content, err := getMailContent(newAds)
		if err != nil {
			log.Println(err.Error())
		}

		// Send the actual email.
		err = sendMail(to, from, to+from+content)
		if err != nil {
			return err
		}

		log.Printf("Found %d new ads! Sent e-mail to %v!\n", len(newAds), config.ToEmail)
	}

	// Now we indicate we are only looking for new ads
	if config.FirstRun {
		fmt.Printf("Added %d ads to my memory. Looking for new ads every %v minutes and sending them to %v.\n",
			len(seen), config.Interval, config.ToEmail)
		config.FirstRun = false
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		// Ensure we have a config file.
		printHelp()
		os.Exit(2)
	}
	err := loadConfig(os.Args[1])
	if err != nil {
		log.Println(err.Error())
	}
	// Save the location of the file, in order to reference it in a HUP-call.
	configLocation = os.Args[1]

	// Listen for signals (SIGHUP)
	handleSignals()

	// Create map of all seen ads, so it is not nil.
	seen = make(map[string]bool)

	for {
		// Check and report if any new ads are found.
		err := checkFinn()
		if err != nil {
			log.Println(err.Error())
		}

		// Check at given interval
		time.Sleep(time.Duration(config.Interval) * time.Minute)
	}
}
