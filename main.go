package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	apiEndpoint = "https://api.hypixel.net/skyblock/bazaar"
	maxCallPerMinute = 120
	waitBetweenRefresh = time.Minute * 30
)

var (
	errInvalidHTTPResponseCode = errors.New("api responded with non 200 status code")
	errFailResponse = errors.New("api responded with bad status")
)

var (
	priceLock = sync.RWMutex{}
	priceMap = make(map[string]float64)
)

type ProductsResponse struct {
	Success bool `json:"success"`
	ProductIds []string `json:"productIds"`
}

type ProductResponse struct {
	Success bool `json:"success"`
	Info struct {
		Recap struct {
			Price float64 `json:"buyPrice"`
		} `json:"quick_status"`
	} `json:"product_info"`
}

func fetchProducts(key string) ([]string, error) {
	url := fmt.Sprintf("%s/products?key=%s", apiEndpoint, key)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(errInvalidHTTPResponseCode.Error() + fmt.Sprintf("%d", resp.StatusCode))
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	list := new(ProductsResponse)
	err = json.Unmarshal(data, list)
	if err != nil {
		return nil, err
	}

	if !list.Success {
		return nil, errFailResponse
	}

	return list.ProductIds, nil
}

func updatePrice(key string, productId string) error {
	url := fmt.Sprintf("%s/product?key=%s&productId=%s", apiEndpoint, key, productId)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New(errInvalidHTTPResponseCode.Error() + fmt.Sprintf("%d", resp.StatusCode))
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	info := new(ProductResponse)
	err = json.Unmarshal(data, info)
	if err != nil {
		return err
	}

	if !info.Success {
		return errFailResponse
	}

	priceLock.Lock()
	priceMap[productId] = info.Info.Recap.Price
	priceLock.Unlock()

	return nil
}

func updateLoop(key string) {
	products, err := fetchProducts(key)
	if err != nil {
		log.Fatalln(err.Error())
	}
	log.Printf("%d products loaded.\n", len(products))

	for {
		log.Println("Data update started")
		for _, productId := range products {
			err := updatePrice(key, productId)
			if err != nil {
				log.Fatalln(err.Error())
			}

			time.Sleep(time.Minute / (maxCallPerMinute - 20))
		}

		log.Println("Data update completed")
		time.Sleep(waitBetweenRefresh)
	}
}

func priceHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.RequestURI, "/")
	if len(parts) < 1 {
		w.WriteHeader(http.StatusBadRequest)
		_, _  = w.Write([]byte("invalid product ID"))
		return
	}

	productId := parts[len(parts) - 1]
	priceLock.RLock()
	defer priceLock.RUnlock()

	price, ok := priceMap[productId]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _  = w.Write([]byte("invalid product ID or price not in cache"))
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%.2f", price)))
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("API key missing")
	}

	key := os.Args[1]
	go updateLoop(key)

	http.HandleFunc("/", priceHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}