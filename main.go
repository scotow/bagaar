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
	apiEndpoint        = "https://api.hypixel.net/skyblock/bazaar"
	maxCallPerMinute   = 120
	waitBetweenRefresh = time.Minute * 2
)

var (
	errInvalidHTTPResponseCode = errors.New("api responded with non 200 status code")
	errFailResponse            = errors.New("api responded with bad status")
)

var (
	priceLock = sync.RWMutex{}
	priceMap  = make(map[string]*ProductPrice)
)

type ProductsResponse struct {
	Success    bool     `json:"success"`
	ProductIds []string `json:"productIds"`
}

type ProductPrice struct {
	Buy  float64
	Sell float64
}

type ProductResponse struct {
	Success bool `json:"success"`
	Info    struct {
		Recap struct {
			Buy  float64 `json:"buyPrice"`
			Sell float64 `json:"sellPrice"`
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
		return nil, errors.New(fmt.Sprintf("%s: %d", errInvalidHTTPResponseCode.Error(), resp.StatusCode))
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
		return errors.New(fmt.Sprintf("%s: %d", errInvalidHTTPResponseCode.Error(), resp.StatusCode))
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

	price := new(ProductPrice)
	price.Buy = info.Info.Recap.Buy
	price.Sell = info.Info.Recap.Sell

	priceLock.Lock()
	priceMap[productId] = price
	priceLock.Unlock()

	return nil
}

func updateLoop(key string) {
	for i := 0; ; i = (i + 1) % 1000 {
		var products []string
		if i == 0 {
			resp, err := fetchProducts(key)
			if err != nil {
				log.Println(err.Error())
				i -= 1
			}
			products = resp
			log.Printf("%d products loaded\n", len(products))
		}

		log.Println("Data update started")
		for p := 0; p < len(products); p++ {
			productId := products[p]
			err := updatePrice(key, productId)
			if err != nil {
				log.Println(err.Error())
				p -= 1
			}

			time.Sleep(time.Minute / (maxCallPerMinute - 5))
		}

		log.Println("Data update completed")
		time.Sleep(waitBetweenRefresh)
	}
}

type productHandler func(*ProductPrice, http.ResponseWriter)

func priceHandler(f productHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimRight(r.URL.Path, "/"), "/")
		if len(parts) < 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid product ID"))
			return
		}

		productId := parts[len(parts)-1]
		priceLock.RLock()
		defer priceLock.RUnlock()

		pp, ok := priceMap[productId]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("invalid product ID or price not in cache"))
			return
		}

		f(pp, w)
	}
}

func buyPriceHandler(pp *ProductPrice, w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%.2f", pp.Sell)))
}

func sellPriceHandler(pp *ProductPrice, w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%.2f", pp.Buy)))
}

func csvHandler(w http.ResponseWriter, _ *http.Request) {
	priceLock.RLock()
	defer priceLock.RUnlock()

	w.WriteHeader(http.StatusOK)
	for productId, pp := range priceMap {
		_, _ = w.Write([]byte(fmt.Sprintf("%s,%.2f,%.2f\n", productId, pp.Sell, pp.Buy)))
	}
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalln("API key missing")
	}

	key := os.Args[1]
	go updateLoop(key)

	http.HandleFunc("/csv", csvHandler)
	http.HandleFunc("/buy/", priceHandler(buyPriceHandler))
	http.HandleFunc("/sell/", priceHandler(sellPriceHandler))
	log.Fatal(http.ListenAndServe(":8080", nil))
}
