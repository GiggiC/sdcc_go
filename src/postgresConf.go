package main

import (
	"database/sql"
	"fmt"
	"log"
)

type server struct {
	db *sql.DB
}

//localhost configuration
const (
	host     = "172.28.1.3"
	port     = 5432
	user     = "postgres"
	password = "password"
	dbname   = "sdcc"
)

//RDS deployment configuration
/*const (
	host     = "sdcc-db.c6fwapw2bm2k.us-east-1.rds.amazonaws.com"
	port     = 5432
	user     = "postgres"
	password = "ElAmqMhwe82DonytfC1a"
	dbname   = "postgres"
)*/

func initDB() (s *server, database *sql.DB) {

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		log.Panic(err)
	}

	err = db.Ping()

	if err != nil {
		log.Panic(err)
	}

	log.Println("Successfully connected!")

	return &server{db: db}, db
}
