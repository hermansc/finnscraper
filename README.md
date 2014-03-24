# FINNscraper

A simple [finn.no](finn.no) scraper, written in Go. Give it your email, an URL
and your given interval - and the program will check for new ads and notify you
when new one comes.

This is better than the exisiting "Saved searches", provided by Finn - as it
can check and notify you as often as every minute, but perhaps more
practically: every 30 or 60 minutes.

## Getting started

First: Use the **mobile site** provided by finn.no.

Second: A provided URL should include `search.html`, e.g.

    http://m.finn.no/bap/forsale/search.html?price_to=10000&sub_category=3215

Which is a search for all computers, less than 10 000 NOK.

Now we instruct the scraper to check every 30 minutes and notify us at
`user@gmail.com`:

    ./scraper -email="user@gmail.com" -from="me@servername.com" -interval=30 -url="http://m.finn.no/bap/forsale/search.html?price_to=10000&sub_category=3215"

## Building and installing

You need Go. Instructions for building and installing are [found
here](http://golang.org/doc/install).

Then you build the binary by:

    go build scraper.go -o scraper

Done!
