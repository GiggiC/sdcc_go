#!/bin/bash
(redis-server &) && (go run /home/ec2-user/go/src/github.com/GiggiC/sdcc_go/script/run.sh)
