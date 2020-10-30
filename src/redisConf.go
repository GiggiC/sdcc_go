package main

import (
	"context"
	"github.com/go-redis/redis/v8"
	"log"
	"os"
)

var client *redis.Client
var ctx = context.Background()

func initRedis() {

	dsn := os.Getenv("REDIS_DSN")

	if len(dsn) == 0 {
		dsn = "localhost:6379" //localhost configuration
		//dsn = "18.211.56.181:6379"	//EC2 configuration
	}

	client = redis.NewClient(&redis.Options{
		Addr:     dsn,
		Password: "empires",
	})

	_, err := client.Ping(ctx).Result()

	if err != nil {
		log.Panic(err)
	}
}
