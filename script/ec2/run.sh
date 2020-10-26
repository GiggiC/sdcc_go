#!/bin/bash
cd /home/ec2-user/go/src/github.com/GiggiC/sdcc_go/src
(redis-server &) && (go run *.go)
