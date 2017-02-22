package crawler

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"sync"
	"time"
)

type product struct {
	ID    string
	Name  string
	Price float64
}

type job struct {
	F   func(string)
	URL string
	W   *sync.WaitGroup
}

const baseSearchURL string = "https://mystique-v1-submarino.b2w.io/mystique/search?"
const baseProductURL string = "https://pdgnamedquery-v1-submarino.b2w.io/run-pdg/product-without-promotion/revision/8?"

const maxOffset int = 480
const maxWorkers int = 25

var client *http.Client

func init() {
	client = &http.Client{}
	client.Timeout = time.Second * 15
}

func fetchFirstPage(category int, pageQueue, productQueue chan string) {
	url := buildSearchURL(category, 0)
	products, limit, total, err := fetchPage(url)
	if err != nil {
		log.Fatalf("could not fetch the first page, error %v:", err)
	}

	// Add products from first page to the product queue
	go func(products []int) {
		for _, prod := range products {
			productQueue <- buildProdutURL(prod)
		}
	}(products)

	// Add other page to the queue basead on limit and total
	go func(limit, total int) {
		possiblePages := math.Ceil(float64(total) / float64(limit))
		maxPages := math.Floor(float64(maxOffset) / float64(limit))

		pages := int(math.Min(possiblePages, maxPages))
		for i := 1; i <= pages; i++ {
			pageQueue <- buildSearchURL(category, i*limit)
		}
		close(pageQueue)
	}(limit, total)
}

// Run will fetch data from submarino products
func Run(category int) {
	begin := time.Now()

	pageQueue := make(chan string, 10)
	productQueue := make(chan string, 10)
	jobs := make(chan job, 30)
	results := make(chan product, 50)

	pageWG := &sync.WaitGroup{}
	productWG := &sync.WaitGroup{}

	// fetch from page queue and add to job queue
	go func(pageQueue, productQueue chan string) {
		for page := range pageQueue {
			jobs <- job{
				URL: page,
				F:   pageWorker(productQueue),
				W:   pageWG,
			}
		}
		go func() {
			pageWG.Wait()
			close(productQueue)
		}()
	}(pageQueue, productQueue)

	// fetch from product queue and add to job queue
	go func(productQueue chan string, results chan product) {
		for product := range productQueue {
			jobs <- job{
				URL: product,
				F:   productWorker(results),
				W:   productWG,
			}
		}
		go func() {
			// When all prodcuts workers are done, colse the jobs and results channels
			productWG.Wait()
			close(jobs)
			close(results)
		}()
	}(productQueue, results)

	// Start executing the jobs
	for i := 0; i < maxWorkers; i++ {
		go func(i int) {
			for job := range jobs {
				job.W.Add(1)
				job.F(job.URL)
				job.W.Done()
			}
		}(i)
	}

	fetchFirstPage(category, pageQueue, productQueue)

	prodCount := 0
	for prod := range results {
		prodCount++
		fmt.Printf("%.02f;%s\n", prod.Price, prod.Name)
	}

	fmt.Printf("Done with %d items after %v\n", prodCount, time.Since(begin))
}
