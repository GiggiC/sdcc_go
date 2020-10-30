package main

import "github.com/umahmood/haversine"

func remove(s []string, i int) []string {

	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

//Checking distance from two coordinates using haversine formula
func checkDistance(x1 float64, x2 float64, y1 float64, y2 float64, r1 int, r2 int) bool {

	sessionLocation := haversine.Coord{Lat: x1, Lon: y1}
	publisherLocation := haversine.Coord{Lat: x2, Lon: y2}
	_, km := haversine.Distance(sessionLocation, publisherLocation)

	if km > float64(r1+r2) {
		return false
	}

	return true
}

func stringInSlice(a string, list []string) bool {

	for _, b := range list {

		if b == a {
			return true
		}
	}

	return false
}
