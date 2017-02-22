package main

import (
	"flag"
	"fmt"

	"github.com/waltton/subcraw/crawler"
)

func main() {
	category := flag.Int("c", -1, "category")
	flag.Parse()

	if *category == -1 {
		fmt.Println("Please informe the category")
		return
	}

	crawler.Run(*category)
}
