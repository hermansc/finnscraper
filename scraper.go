package main

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/itkinside/itkconfig"
	"log"
	"math/rand"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"text/template"
	"time"
)

type Config struct {
	Url       []string
	Interval  int
	ToEmail   string
	FromEmail string
	UserAgent string
	Template  string
	Debug     bool
	FirstRun  bool
}

var config *Config
var configLocation string
var seen map[string][]string

func printHelp() {
	fmt.Fprintf(os.Stderr, "Specify config-file:\n%s scraper.conf\n", os.Args[0])
}

func loadConfig(filename string) error {
	// We start with a default config, and edit it based on config file.
	config = &Config{
		Url:       []string{},
		Interval:  30,
		ToEmail:   "user@example.com",
		FromEmail: "root@localhost",
		UserAgent: "Mozilla/5.0 (Windows NT 5.1; rv:31.0) Gecko/20100101 Firefox/31.0",
		Template:  "default.tmpl",
		Debug:     false,
		FirstRun:  true,
	}

	// Load config from file, overwriting defaults.
	err := itkconfig.LoadConfig(filename, config)
	if err != nil {
		return err
	}

	// Create map of all seen ads, so it is not nil (and we reset in case HUP)
	seen = make(map[string][]string)

	// Check that the config is OK.
	if config.ToEmail == "" || config.FromEmail == "" || config.Url == nil {
		return errors.New("Invalid configuration. You need to provide To/From-emails and Url")
	}
	if config.Interval < 1 {
		return errors.New("Interval is to small. Set 'interval' to a value larger than zero")
	}

	// Check that the template is parsable.
	_, err = parseTemplate(config.Template, nil)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Ensure that the URL looks good.
	re := regexp.MustCompile("^(http://)?(m.finn.no)(/.+/)+search.html(.*)$")
	for _, url := range config.Url {
		if !(re.Match([]byte(url))) {
			log.Fatal("Your URL '" + url + `' is in a format invalid format.
			Are you using the mobile site? Check the documentation.`)
		}
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
					log.Fatal(err)
				}
				log.Println("Loaded new config after SIGHUP")

				// Run a check, right away.
				err = checkAllUrls()
				if err != nil {
					log.Println(err)
				}
			}
		}
	}()
}

func sendMail(to, from, content string) error {
	// Check that the SMTP server is OK and connect.
	c, err := smtp.Dial("localhost:25")
	if err != nil {
		log.Println("Could not send e-mail, check your local SMTP-configuration. Got following error:")
		return err
	}

	// Set sender.
	err = c.Mail(from)
	if err != nil {
		return err
	}

	// Set recipient.
	c.Rcpt(to)
	if err != nil {
		return err
	}

	// Write the content to the mail buffer.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()
	buf := bytes.NewBufferString(content)
	_, err = buf.WriteTo(wc)
	if err != nil {
		return err
	}

	// Mail successfully sent.
	return nil
}

func stringInSlice(in string, list []string) bool {
	for _, elem := range list {
		if elem == in {
			return true
		}
	}
	return false
}

func getMailContent(url string, ads []string) (string, error) {
	// Add the strings of all ads on the dict sent to the template.
	d := make(map[string]interface{})
	d["Ads"] = strings.TrimRight(strings.Join(ads, ""), "\n\r")
	d["NumResults"] = len(ads)
	d["SearchURL"] = url

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

func checkAllUrls() error {
	// Check all URLs found in the config file.
	for _, url := range config.Url {
		err := checkFinn(url)
		if err != nil {
			log.Fatalln(err.Error())
		}

		// Sleep between 1 and 6 seconds.
		mseconds := 1000 + rand.Intn(5000)
		time.Sleep(time.Duration(mseconds) * time.Millisecond)

	}

	// Now we indicate we are only looking for new ads
	if config.FirstRun {
		config.FirstRun = false
	}
	return nil
}

func getURL(url string, useragent string) (*goquery.Document, error) {
	var doc *goquery.Document
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return doc, err
	}
	req.Header.Set("User-Agent", useragent)
	resp, err := client.Do(req)
	return goquery.NewDocumentFromResponse(resp)
}

func checkFinn(url string) error {
	if config.Debug {
		log.Println("Checking " + url)
	}

	// For saving the non-seen new ads.
	newAds := make([]string, 0)

	// Open the provided URL.
	doc, err := getURL(url, config.UserAgent)
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
		if ok := stringInSlice(finncode, seen[url]); !ok {
			// Construct a devent ad header.
			title := strings.TrimSpace(s.Find("h2[data-automation-id='titleRow']").Text())
			price := strings.TrimSpace(s.Find("span[data-automation-id='bodyRow']").Text())
			if price == "" {
				price = "0,-"
			}
			finn_url := "www.finn.no/finn/finncode/result?finncode="
			adHeader := fmt.Sprintf("%v (%v) - %v%v\n", title, price, finn_url, finncode)

			// Add the ad to our data structures, saving it.
			newAds = append(newAds, adHeader)

			// Add the finncode to seen ids.
			seen[url] = append(seen[url], finncode)

			// We assume there are no more than 5 new ads in one interval.
			if i+1 == 5 && !(config.FirstRun) {
				return false
			}
		}
		return true
	})

	// We've found new ads, send them to the designated e-mail.
	if len(newAds) > 0 && !(config.FirstRun) {
		content, err := getMailContent(url, newAds)
		if err != nil {
			log.Println(err.Error())
		}

		// Send the actual email.
		err = sendMail(config.ToEmail, config.FromEmail, content)
		if err != nil {
			return err
		}

		log.Printf("Found %d new ads! Sent e-mail to %v!\n", len(newAds), config.ToEmail)
	}
	if config.FirstRun {
		s := fmt.Sprintf("Added %d ads to my memory. ", len(seen[url]))
		s += fmt.Sprintf("Looking for new ads every %v minutes and sending them to %v.\n",
			config.Interval, config.ToEmail)
		fmt.Printf(s)
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		// Ensure we have a config file.
		printHelp()
		os.Exit(2)
	}
	// Save the location of the file, in order to reference it in a HUP-call.
	configLocation = os.Args[1]

	err := loadConfig(configLocation)
	if err != nil {
		log.Fatal(err)
	}

	// Listen for signals (SIGHUP)
	handleSignals()

	for {
		// Check and report if any new ads are found.
		err := checkAllUrls()
		if err != nil {
			log.Println(err)
		}

		// Check at given interval
		time.Sleep(time.Duration(config.Interval) * time.Minute)
	}
}
