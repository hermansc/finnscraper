package main

import (
  "fmt"
  "github.com/PuerkitoBio/goquery"
  "log"
  "strings"
  "flag"
  "time"
  "os"
  "net/smtp"
)

func printHelp() {
  fmt.Fprintf(os.Stderr, "Usage of %v:\n", os.Args[0])
  flag.PrintDefaults()
}

func main() {
  var url = flag.String("url", "", "URL to scrape for Finncodes")
  var interval = flag.Int("interval", 30, "Time between each check")
  var toemail = flag.String("email", "", "Email to send reports")
  var fromemail = flag.String("from", "", "Email to send reports")
  var debug = flag.Bool("debug", false, "Enable some extra debug messages")
  flag.Parse()

  if (*url == "" || *toemail == "" || *fromemail == "") {
    printHelp()
    os.Exit(2)
  }

  firstRun := true
  seen := make(map[string]bool)

  for {
    if (*debug) { fmt.Println("[DEBUG] Checking provided URL...") }

    // For saving the non-seen new ads.
    newAds := make([]string, 0)

    // Open the provided URL.
    doc, err := goquery.NewDocument(*url)
    if (err != nil) { log.Println(err.Error()) }

    // Find all results on the HTML page.
    results := doc.Find("div[data-automation-id='adList']")
    results.ChildrenFiltered("a").EachWithBreak(func(i int, s *goquery.Selection) bool {
      finncode, _ := s.Attr("id")
      if _, ok := seen[finncode]; !ok {
        // Construct a devent ad header.
        title := strings.TrimSpace(s.Find("div[data-automation-id='titleRow']").Text())
        price := strings.TrimSpace(s.Find("span[data-automation-id='bodyRow']").Text())
        if (price == "") { price = "0,-" }
        finn_url := "www.finn.no/finn/finncode/result?finncode="
        adHeader := fmt.Sprintf("%v (%v) - %v%v\n", title, price, finn_url, finncode)

        // Add the ad to our data structures, saving it.
        newAds = append(newAds, adHeader)
        seen[finncode] = true

        // We assume there are no more than 5 new ads in one interval.
        if (i+1 == 5 && !firstRun) {
          return false
        }
      }
      return true
    })

    // We've found new ads, send them to the designated e-mail.
    if (len(newAds) > 0 && !firstRun) {
      to := fmt.Sprintf("To: %s\r\n", *toemail)
      from := fmt.Sprintf("From: %v\r\n", *fromemail)
      subject := fmt.Sprintf("Subject: Found %d new matches on finn.no\r\n", len(newAds))
      head := to + from + subject + "\r\n"

      body := fmt.Sprintf("The search on the following URL:\n\n%v\n\nYielded %d new results:\n\n", *url, len(newAds))
      for _, ad := range newAds {
        body = body + fmt.Sprintf("%v", ad)
      }
      body = body + "\nSincerely yours,\n- The awesome FINN.no scraper <3"

      err := smtp.SendMail("localhost:25",
        nil,
        *fromemail,
        []string{*toemail},
        []byte(head+body))
      if err != nil { log.Println(err.Error()) }
      fmt.Printf("[INFO] Found %d new ads! Sent e-mail to %v!\n", len(newAds), *toemail)
    }

    // Now we indicate we are only looking for new ads
    if (firstRun) {
      fmt.Printf("[INFO] Added %d ads to my memory. Looking for new ads every %v minutes and sending them to %v.\n", 
                  len(seen), *interval, *toemail)
      firstRun = false
    }

    // Check at given interval
    time.Sleep(time.Duration(*interval) * time.Minute)
  }
}
