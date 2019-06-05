# Pilot Load

This tool connects to Pilot and opens an XDS stream.

## Install

`go get github.com/howardjohn/pilot-load`

## Usage

* The `-c` flag can be provided for the number of clients. Default is 1.
* The `-p` flag can be provided to change the Pilot address to use. By default this is `localhost:15010`.

Example: `pilot-load -c 50 -p localhost:15010`

Pushes can be manually triggered with `curl localhost:8080/debug/adsz?push=true`
