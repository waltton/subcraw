package crawler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type pageResult struct {
	Result struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"_result"`
	Products []struct {
		ID int `json:"id"`
	} `json:"products"`
}

type productResult struct {
	Product struct {
		Result struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	} `json:"product"`
	Offer struct {
		Result struct {
			Offers []struct {
				SalesPrice float64 `json:"salesPrice"`
				ListPrice  float64 `json:"listPrice"`
			} `json:"offers"`
		} `json:"result"`
	} `json:"offer"`
}

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

var client *http.Client

func init() {
	client = &http.Client{}
	client.Timeout = time.Second * 15
}

func buildSearchURL(category, offset int) string {
	var params []string

	params = append(params, "source=omega")
	params = append(params, fmt.Sprintf(`filter={"id":"category.id","value":"%d","fixed":true}`, category))

	if offset != 0 {
		params = append(params, "offset="+strconv.Itoa(offset))
	}

	return baseSearchURL + strings.Join(params, "&")
}

func buildProdutURL(productID int) string {
	var params []string

	params = append(params, "id="+strconv.Itoa(productID))
	params = append(params, "offerLimit=1")
	params = append(params, "opn=")
	params = append(params, "storeId=nil")

	return baseProductURL + strings.Join(params, "&")

}

func fetchProduct(url string) (prod *product, err error) {
	var result productResult
	var resp *http.Response

	if resp, err = client.Get(url); err != nil {
		return nil, fmt.Errorf("could not fetch data from page, error: %v", err)
	}

	if err = json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("could not decode page content: error: %v", err)
	}
	defer resp.Body.Close()

	prod = new(product)

	prod.ID = result.Product.Result.ID
	prod.Name = result.Product.Result.Name

	if len(result.Offer.Result.Offers) > 0 {
		prod.Price = result.Offer.Result.Offers[0].SalesPrice
	}

	return
}

func fetchPage(url string) (productIDs []int, limit, total int, err error) {
	var result pageResult
	var resp *http.Response

	if resp, err = client.Get(url); err != nil {
		return nil, 0, 0, fmt.Errorf("could not fetch data from page, error: %v", err)
	}

	var data []byte

	if data, err = ioutil.ReadAll(resp.Body); err != nil {
		return nil, 0, 0, fmt.Errorf("could not decode page content: error: %v", err)
	}
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, 0, 0, fmt.Errorf("could not decode page content: error: %v", err)
	}

	defer resp.Body.Close()

	for _, product := range result.Products {
		productIDs = append(productIDs, product.ID)
	}

	limit = result.Result.Limit
	total = result.Result.Total

	return
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

func pageWorker(productQueue chan string) func(string) {
	return func(url string) {
		prodIDs, _, _, err := fetchPage(url)
		if err != nil {
			fmt.Printf("error while fetching from %s, error: %v", url, err)
			return
		}

		for _, id := range prodIDs {
			productQueue <- buildProdutURL(id)
		}
	}
}

func productWorker(results chan product) func(string) {
	return func(url string) {
		prod, err := fetchProduct(url)
		if err != nil {
			fmt.Printf("error while fetching from %s, error: %v", url, err)
			return
		}
		results <- *prod
	}
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

	maxWorkers := 25

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

	fmt.Printf("Done after %v\n", time.Since(begin))
}
