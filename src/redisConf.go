package main

import (
	"context"
	"github.com/go-redis/redis"
	"os"
)

var client *redis.Client
var ctx = context.Background()

func initRedis() {

	dsn := os.Getenv("REDIS_DSN")

	if len(dsn) == 0 {
		//dsn = "localhost:6379"
		dsn = "18.211.56.181:6379"
	}

	client = redis.NewClient(&redis.Options{
		Addr: dsn,
	})

	_, err := client.Ping(ctx).Result()

	if err != nil {
		panic(err)
	}
}
