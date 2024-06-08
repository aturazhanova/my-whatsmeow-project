package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

func main() {
	testQRTextAPI()
	testQRPhotoAPI()
}

func testQRTextAPI() {
	resp, err := http.Get("http://localhost:8080/qr/text")
	if err != nil {
		log.Fatalf("Failed to call QR text API: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	var result map[string]string
	err = json.Unmarshal(body, &result)
	if err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	fmt.Println("QR Code as text:", result["qr_code"])
}

func testQRPhotoAPI() {
	resp, err := http.Get("http://localhost:8080/qr/photo")
	if err != nil {
		log.Fatalf("Failed to call QR photo API: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	fmt.Println("QR Code as photo received, saving to file qr_code.png")
	err = ioutil.WriteFile("qr_code.png", body, 0644)
	if err != nil {
		log.Fatalf("Failed to save QR code image: %v", err)
	}
}
