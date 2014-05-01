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

    http://m.finn.no/bap/forsale/search.html?price_to=10000&sub_category=1.93.3215

Which is a search for all computers, less than 10 000 NOK.

Copy the `scraper.conf.sample` and add your own parameters. It should be pretty
self explanatory.

Start the script by:

    $ ./finnscraper scraper.conf

## Updating the configuration / Using SIGHUP

If you update the config-file, you don't need to stop the script. You can just
use the PID and reload it by sending the SIGHUP signal.

First find the PID:

    $ pidof finnscraper
    1692

Then use this PID to send SIGHUP:

    $ kill -HUP 1692

## Building and installing

You need Go. Instructions for building and installing are [found
here](http://golang.org/doc/install). You will need Go >= 1.1.
Please check [this
guide](http://www.extellisys.com/articles/golang-on-debian-wheezy) if your OS
does not provide this version.

Then you build the binary by first getting the dependencies:

    $ go get

And then building the project:

    $ go build scraper.go

Done!
