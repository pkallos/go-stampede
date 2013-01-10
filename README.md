# Stampede

Load testing tool written (probably poorly) in Go. I am still learning.

Usage:
```
$ go run stampede.go -h
usage: stampede [address]

Options:
  -b=false: Benchmark mode, fire requests without waiting.
  -c=10: The number of buffalo to spawn.
  -f="": Input file to read from.
  -h=false: Show this help.
  -i=false: Random sampling of the input file.
  -t=30: The duration of the stampede in seconds.
  -v=false: Use verbose logging.
  -w=1: The time clients wait between HTTP requests (in seconds).
```

## Input File Format
At the moment the input file format is a newline sepearted list of URLs to hit. e.g.
```
http://www.mydomain.com/index.html
http://www.mydomain.com/access/information/1
```

## Goal
The final goal for this tool is to be able to simulate load in as REAL a way as possible. This means being
able to craft and send requests with valid cookies/session data, POST real requests, etc...

Another nice-to-have feature I want to implement is the ability to aproximately match the timings in a given log file.
The use case for this being that you have a cluster of production webservers and you want to accurately replay
full requests (including headers) to another test clutser, at aproximately the same rate at which they arrive.

Unclear what the input file format will be for that, ideally I would be able to read the default log format for
NGINX, lighttpd or varnish...

## Notes:

Pretty sure that for this to run with any success you will need to alter your max# of file descriptors open.
In linuxland you can do what I did and run `sudo vim /etc/security/limits.conf`, adding an entry for your
user like.
```
username  -  nofile 128000
```
