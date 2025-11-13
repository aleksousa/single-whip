package main

import (
	"fmt"
	"net/http"
)

func main() {
	fs := http.FileServer(http.Dir("."))

	http.Handle("/", fs)

	fmt.Println("Web server started on http://localhost:8081")
	fmt.Println("Open http://localhost:8081/index.html in your browser")

	if err := http.ListenAndServe(":8081", nil); err != nil {
		panic(err)
	}
}
