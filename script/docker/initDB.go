// main.go
package main

import (
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"io/ioutil"
	"strings"
)

const (
	host     = "172.21.0.2"
	port     = 5432
	user     = "postgres"
	password = "password"
	dbname   = "sdcc"
)

func initDB() *sql.DB {

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		panic(err)
	}

	return db
}

func main() {

	db := initDB()

	file, err := ioutil.ReadFile("/home/luigi/go/sdcc_go/script/docker/sdcc.sql")

	if err != nil {
		fmt.Println(err)
	}

	requests := strings.Split(string(file), ";")

	for _, request := range requests {

		_, err = db.Exec(request)

		if err != nil {
			fmt.Println(err)
		}
	}

	err = db.Close()

	if err != nil {
		fmt.Println(err)
	}
}
