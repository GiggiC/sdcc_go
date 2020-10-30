#!/bin/bash
docker-compose up -d
docker exec -it docker_db_1 psql -U postgres -c "create database sdcc"
go run initDB.go
cd ../../src
go run -race *.go