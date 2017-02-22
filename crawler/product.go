package crawler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

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
