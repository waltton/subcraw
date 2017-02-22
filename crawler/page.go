package crawler

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
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

func buildSearchURL(category, offset int) string {
	var params []string

	params = append(params, "source=omega")
	params = append(params, fmt.Sprintf(`filter={"id":"category.id","value":"%d","fixed":true}`, category))

	if offset != 0 {
		params = append(params, "offset="+strconv.Itoa(offset))
	}

	return baseSearchURL + strings.Join(params, "&")
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
